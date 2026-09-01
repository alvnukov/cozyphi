package mcp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Pool lazily connects to configured MCP servers.
type Pool struct {
	mu      sync.Mutex
	servers map[string]ServerConfig
	clients map[string]Client
	status  map[string]ServerStatus
}

// ConnectionState is the latest observed lifecycle state of one configured
// MCP server. Configured means no connection has been attempted yet.
type ConnectionState string

const (
	StateConfigured ConnectionState = "configured"
	StateConnected  ConnectionState = "connected"
	StateFailed     ConnectionState = "failed"
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
		servers: servers,
		clients: map[string]Client{},
		status:  status,
	}
}

// LoadPool loads config for projectConfigPath (e.g. <root>/.cozyphi/mcp.json)
// over any lower-priority imported sources and returns a pool, or nil when disabled.
func LoadPool(projectConfigPath string, lowerPriority ...map[string]ServerConfig) (*Pool, error) {
	if Disabled() {
		return nil, nil
	}
	servers, err := Load(projectConfigPath, lowerPriority...)
	if err != nil {
		return nil, err
	}
	return NewPool(servers), nil
}

// ServerNames returns sorted configured server names.
func (p *Pool) ServerNames() []string {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	names := make([]string, 0, len(p.servers))
	for name := range p.servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ServerStatuses returns sorted copies of the latest observed server states.
func (p *Pool) ServerStatuses() []ServerStatus {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ServerStatus, 0, len(p.status))
	for _, status := range p.status {
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

// Doctor checks config and connectivity for each server.
func (p *Pool) Doctor(ctx context.Context) []DoctorResult {
	if p == nil {
		return []DoctorResult{{Name: "(none)", OK: false, Detail: "mcp disabled or not loaded"}}
	}
	names := p.ServerNames()
	if len(names) == 0 {
		return []DoctorResult{{Name: "(none)", OK: false, Detail: "no servers in mcp.json"}}
	}
	out := make([]DoctorResult, 0, len(names))
	for _, name := range names {
		out = append(out, p.doctorOne(ctx, name))
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

// Close shuts down all live clients.
func (p *Pool) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
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
	if c, ok := p.clients[server]; ok {
		return c, nil
	}
	cfg, ok := p.servers[server]
	if !ok {
		return nil, fmt.Errorf("unknown mcp server %q", server)
	}
	c, err := NewClient(server, cfg)
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
