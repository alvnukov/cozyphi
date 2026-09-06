package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Pool lazily connects to configured MCP servers.
type Pool struct {
	mu       sync.Mutex
	servers  map[string]ServerConfig
	clients  map[string]Client
	status   map[string]ServerStatus
	disabled map[string]bool
	cwd      string // resolved once before publication; empty preserves legacy inheritance
	closed   bool
	// origins records, per server, every configuration source that defined
	// it, in precedence order. It is written once at load and read only by
	// the harness view; a hand-built pool has none and reports none.
	origins map[string][]Origin
	// offInConfig is the set the configuration files started switched off,
	// kept apart from disabled above so a /mcp toggle stays visibly the
	// session's own choice rather than something the file asked for.
	offInConfig map[string]bool
}

// ConnectionState is the latest observed lifecycle state of one configured
// MCP server. Configured means no connection has been attempted yet.
type ConnectionState string

const (
	StateConfigured ConnectionState = "configured"
	StateConnected  ConnectionState = "connected"
	StateFailed     ConnectionState = "failed"
	// StateDisabled marks a server switched off via /mcp: it stays configured
	// but is hidden from the model until re-enabled.
	StateDisabled ConnectionState = "disabled"
)

// ServerStatus is an immutable status-panel snapshot.
type ServerStatus struct {
	Name  string
	State ConnectionState
}

// DoctorResult is one row from Doctor.
type DoctorResult struct {
	Name   string
	OK     bool
	Detail string
	Tools  int
}

// NewPool wraps a server config map. Pass nil/empty for a no-op pool.
func NewPool(servers map[string]ServerConfig) *Pool {
	if servers == nil {
		servers = map[string]ServerConfig{}
	}
	status := make(map[string]ServerStatus, len(servers))
	for name := range servers {
		status[name] = ServerStatus{Name: name, State: StateConfigured}
	}
	return &Pool{
		servers:  servers,
		clients:  map[string]Client{},
		status:   status,
		disabled: map[string]bool{},
	}
}

// LoadPool loads config for projectConfigPath (e.g. <root>/.cozyphi/mcp.json)
// over any lower-priority imported sources and returns a pool, or nil when disabled.
// Servers named in a "disabled" list of either config start switched off.
func LoadPool(projectConfigPath string, lowerPriority ...map[string]ServerConfig) (*Pool, error) {
	if Disabled() {
		return nil, nil
	}
	servers, origins, err := load(projectConfigPath, lowerPriority...)
	if err != nil {
		return nil, err
	}
	disabled, err := LoadDisabled(projectConfigPath)
	if err != nil {
		return nil, err
	}
	pool := NewPool(servers)
	pool.origins = origins
	pool.offInConfig = disabled
	for name := range disabled {
		_ = pool.SetEnabled(name, false) // unknown names are stale entries, not errors
	}
	return pool, nil
}

// LoadPoolInDir is LoadPool with an explicit working directory for stdio servers.
// Relative paths resolve against the caller's current directory; symlinks are
// canonicalized once, before lazy clients are created. Empty, missing, and
// non-directory paths fail even when MCP is disabled. Use LoadPool to retain
// legacy process-directory inheritance instead.
func LoadPoolInDir(projectConfigPath, cwd string, lowerPriority ...map[string]ServerConfig) (*Pool, error) {
	if cwd == "" {
		return nil, errors.New("mcp working directory is empty: provide an existing workspace directory")
	}
	resolved, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve mcp working directory %q: %w", cwd, err)
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve mcp working directory %q: provide an existing workspace directory: %w",
			cwd,
			err,
		)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("stat mcp working directory %q: %w", cwd, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("mcp working directory %q is not a directory: provide a workspace directory", cwd)
	}
	pool, err := LoadPool(projectConfigPath, lowerPriority...)
	if err != nil || pool == nil {
		return pool, err
	}
	pool.cwd = resolved
	return pool, nil
}

// ServerNames returns sorted names of servers the model can reach — every
// configured server that is not disabled. It feeds the system-prompt catalog.
func (p *Pool) ServerNames() []string {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	names := make([]string, 0, len(p.servers))
	for name := range p.servers {
		if p.disabled[name] {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DisabledNames returns the sorted names currently switched off.
func (p *Pool) DisabledNames() []string {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	names := make([]string, 0, len(p.disabled))
	for name := range p.disabled {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SetEnabled switches one configured server on or off. Disabling closes a
// live client at once so its tools vanish from the model immediately;
// enabling only clears the flag — the connection stays lazy. Unknown names
// are an error so typos do not silently persist a no-op.
func (p *Pool) SetEnabled(name string, enabled bool) error {
	if p == nil {
		return errors.New("mcp pool is nil")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.servers[name]; !ok {
		return fmt.Errorf("unknown mcp server %q", name)
	}
	if enabled {
		delete(p.disabled, name)
		p.status[name] = ServerStatus{Name: name, State: StateConfigured}
		return nil
	}
	p.disabled[name] = true
	// A failing close does not undo the disable: the server is off for the
	// model either way, and the subprocess is dead or dying regardless.
	if c, ok := p.clients[name]; ok {
		_ = c.Close()
		delete(p.clients, name)
	}
	p.status[name] = ServerStatus{Name: name, State: StateDisabled}
	return nil
}

// ServerStatuses returns sorted copies of the latest observed server states,
// including disabled entries marked with StateDisabled.
func (p *Pool) ServerStatuses() []ServerStatus {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ServerStatus, 0, len(p.status))
	for name, status := range p.status {
		if p.disabled[name] {
			status = ServerStatus{Name: name, State: StateDisabled}
		}
		out = append(out, status)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// HasServers reports whether any servers are configured.
func (p *Pool) HasServers() bool {
	return p != nil && len(p.ServerNames()) > 0
}

// ListTools lists tools for a server (lazy connect).
func (p *Pool) ListTools(ctx context.Context, server string) ([]ToolDef, error) {
	c, err := p.client(server)
	if err != nil {
		return nil, err
	}
	result, err := c.ListTools(ctx)
	p.observe(server, err)
	return result, err
}

// Inspect returns one tool definition.
func (p *Pool) Inspect(ctx context.Context, server, tool string) (*ToolDef, error) {
	c, err := p.client(server)
	if err != nil {
		return nil, err
	}
	result, err := c.FindTool(ctx, tool)
	p.observe(server, err)
	return result, err
}

// Call invokes a tool on a server.
func (p *Pool) Call(ctx context.Context, server, tool string, args map[string]any) (string, error) {
	c, err := p.client(server)
	if err != nil {
		return "", err
	}
	result, err := c.CallTool(ctx, tool, args)
	p.observe(server, err)
	return result, err
}

// Doctor checks config and connectivity for each configured server,
// disabled ones included — they report as switched off, not missing.
func (p *Pool) Doctor(ctx context.Context) []DoctorResult {
	if p == nil {
		return []DoctorResult{{Name: "(none)", OK: false, Detail: "mcp disabled or not loaded"}}
	}
	statuses := p.ServerStatuses()
	if len(statuses) == 0 {
		return []DoctorResult{{Name: "(none)", OK: false, Detail: "no servers in mcp.json"}}
	}
	out := make([]DoctorResult, 0, len(statuses))
	for _, status := range statuses {
		if status.State == StateDisabled {
			out = append(out, DoctorResult{Name: status.Name, OK: false, Detail: "disabled (enable with /mcp)"})
			continue
		}
		out = append(out, p.doctorOne(ctx, status.Name))
	}
	return out
}

func (p *Pool) doctorOne(ctx context.Context, name string) DoctorResult {
	p.mu.Lock()
	cfg := p.servers[name]
	p.mu.Unlock()

	if err := validateServerConfig(cfg); err != nil {
		return DoctorResult{Name: name, OK: false, Detail: err.Error()}
	}
	tools, err := p.ListTools(ctx, name)
	if err != nil {
		return DoctorResult{Name: name, OK: false, Detail: err.Error()}
	}
	return DoctorResult{
		Name:   name,
		OK:     true,
		Detail: fmt.Sprintf("%d tools", len(tools)),
		Tools:  len(tools),
	}
}

func validateServerConfig(cfg ServerConfig) error {
	switch {
	case cfg.IsStdio():
		_, err := cfg.CmdLine()
		return err
	case cfg.IsHTTP():
		if strings.TrimSpace(cfg.URL) == "" {
			return errors.New("http transport requires url")
		}
		return nil
	default:
		return fmt.Errorf("unsupported transport %q", cfg.Transport)
	}
}

// Close permanently shuts down the pool and all live clients. Repeated calls are safe.
func (p *Pool) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	var first error
	for name, c := range p.clients {
		if err := c.Close(); err != nil {
			p.status[name] = ServerStatus{Name: name, State: StateFailed}
			if first == nil {
				first = err
			}
		} else {
			p.status[name] = ServerStatus{Name: name, State: StateConfigured}
		}
		delete(p.clients, name)
	}
	return first
}

func (p *Pool) client(server string) (Client, error) {
	if p == nil {
		return nil, errors.New("mcp pool is nil")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, errors.New("mcp pool is closed: load a new pool to connect")
	}
	if p.disabled[server] {
		return nil, fmt.Errorf("mcp server %q is disabled", server)
	}
	if c, ok := p.clients[server]; ok {
		return c, nil
	}
	cfg, ok := p.servers[server]
	if !ok {
		return nil, fmt.Errorf("unknown mcp server %q", server)
	}
	c, err := newClient(server, cfg, p.cwd)
	if err != nil {
		p.status[server] = ServerStatus{Name: server, State: StateFailed}
		return nil, err
	}
	p.clients[server] = c
	return c, nil
}

func (p *Pool) observe(server string, err error) {
	if p == nil {
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		p.status[server] = ServerStatus{Name: server, State: StateFailed}
		return
	}
	p.status[server] = ServerStatus{Name: server, State: StateConnected}
}
