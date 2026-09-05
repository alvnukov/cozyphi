package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/alvnukov/cozyphi/internal/debuglog"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/memory"
	"github.com/alvnukov/cozyphi/internal/opencode"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/tasks"
	"github.com/alvnukov/cozyphi/internal/usage"
)

// Runtime owns process resources. Workspaces borrow these resources; closing a
// session must not close a provider, language server, or MCP pool used by another.
// The plan runtime is default policy only, never a session's mutable plan.
// Close is idempotent and prevents further workspace creation.
type Runtime struct {
	mu                  sync.Mutex
	resourceMu          sync.Mutex // serializes resource loading, never admission/shutdown
	builders            sync.WaitGroup
	constructionCtx     context.Context
	cancelConstruction  context.CancelFunc
	closed              bool
	proj                *project.Project
	providers           *provider.Manager
	opencode            *opencode.Source
	planRuntime         *plangate.Runtime
	history             *usage.Store
	workspaces          map[string]*Workspace
	memories            map[string]*memory.Store
	jobs                *job.Manager
	sessions            map[*Controller]struct{}
	interactiveChildren bool
	children            []ChildSession
	childBuilders       int
	closeDone           chan struct{}
	closeErr            error // written by shutdown, read only after closeDone
}

// Workspace is the resource identity for one canonical working directory. Two
// linked worktrees can share a memory corpus but not hooks, MCP configuration or
// language-server lifetime. The runtime retains resources until process shutdown.
type Workspace struct {
	runtime       *Runtime
	cwd           string
	proj          *project.Project
	hooks         *hooks.Manager
	mcpPool       *mcp.Pool
	mcpLoadFailed bool
	lspMgr        *lsp.Manager
	memory        *memory.Store
	tasks         *tasks.Registry
}

// Root is the canonical cwd shared by this workspace's services and session UI.
// Distinct git worktrees remain distinct workspaces.
func (w *Workspace) Root() string { return w.cwd }

// NewRuntime loads process-wide sources once. The optional usage history is
// borrowed from the command's writer; it carries no per-session navigation state.
func NewRuntime(proj *project.Project, histories ...*usage.Store) (*Runtime, error) {
	if proj == nil {
		return nil, errors.New("tui: nil project")
	}
	if err := proj.LoadConfig(); err != nil {
		return nil, err
	}
	providers, err := provider.Open(provider.Options{
		CachePath: proj.Global().ProviderCatalogFile(), CredentialsPath: proj.Global().CredentialsFile(),
	})
	if err != nil {
		return nil, fmt.Errorf("tui: initialize providers: %w", err)
	}
	var source *opencode.Source
	if proj.Config().OpenCode.Enabled {
		source, err = opencode.Load(opencode.Options{Catalog: providers.Providers()})
		if err != nil {
			debuglog.Logf("opencode: load: %v", err)
		}
	}
	defaults, err := harnesssettings.LoadPlanDefaults(proj.Global().ConfigFile())
	if err != nil {
		return nil, fmt.Errorf("tui: initialize plan policy: %w", err)
	}
	policy, err := plangate.NewRuntime(defaults)
	if err != nil {
		return nil, fmt.Errorf("tui: initialize plan policy: %w", err)
	}
	r := &Runtime{
		proj: proj, providers: providers, opencode: source, planRuntime: policy,
		workspaces: make(map[string]*Workspace), memories: make(map[string]*memory.Store),
		sessions: make(map[*Controller]struct{}), closeDone: make(chan struct{}),
	}
	if len(histories) > 0 {
		r.history = histories[0]
	}
	// A process manager has no ambient parent. Engines bind their own runner;
	// an unbound spawn fails closed rather than borrowing another session.
	r.jobs, err = job.New(job.Options{
		Root: proj.JobsDir(),
		Runner: job.RunnerFunc(func(context.Context, job.RunEnv) (string, error) {
			return "", errors.New("tui: spawn requires a session-bound runner")
		}),
		OnOutcome: r.notifyOutcome,
		OnStoreError: func(op, id string, err error) {
			debuglog.Logf("jobs: %s %s: %v", op, id, err)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("tui: initialize jobs: %w", err)
	}
	r.constructionCtx, r.cancelConstruction = context.WithCancel(context.Background())
	return r, nil
}

// Workspace resolves symlinks before caching, keeping cwd/config identity rather
// than collapsing different worktrees to their common git root.
func (r *Runtime) Workspace(cwd string) (*Workspace, error) {
	if r == nil {
		return nil, errors.New("tui: nil runtime")
	}
	if cwd == "" {
		cwd = r.proj.Root()
	}
	path, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("tui: resolve workspace: %w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("tui: resolve workspace: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("tui: stat workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("tui: workspace %q is not a directory", path)
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errors.New("tui: runtime is closed")
	}
	if ws := r.workspaces[path]; ws != nil {
		r.mu.Unlock()
		return ws, nil
	}
	r.builders.Add(1)
	r.mu.Unlock()
	defer r.builders.Done()
	// A slow filesystem/config load must not keep Close from canceling
	// already-live sessions. Only loaders serialize against other loaders.
	r.resourceMu.Lock()
	defer r.resourceMu.Unlock()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errors.New("tui: runtime is closed")
	}
	ws := r.workspaces[path]
	r.mu.Unlock()
	if ws != nil {
		return ws, nil
	}
	proj := r.proj
	if proj.Root() != path {
		proj, err = project.Discover(path)
		if err != nil {
			return nil, fmt.Errorf("tui: discover workspace: %w", err)
		}
		if proj.Global() != r.proj.Global() {
			return nil, errors.New("tui: workspace belongs to a different global configuration")
		}
		if err := proj.LoadConfig(); err != nil {
			return nil, err
		}
	}
	ws = &Workspace{runtime: r, cwd: path, proj: proj, hooks: loadHooksManager(proj)}
	ws.memory = r.memories[proj.MemoryDir()]
	if ws.memory == nil {
		ws.memory, err = memory.Open(proj.MemoryDir(), usage.Memory{Store: r.history, Dir: proj.MemoryDir()})
		if err != nil {
			debuglog.Logf("memory: open: %v", err)
		} else {
			r.memories[proj.MemoryDir()] = ws.memory
		}
	}
	if ws.tasks, err = tasks.Discover(proj.RepoRoot()); err != nil {
		debuglog.Logf("tasks: discover: %v", err)
	}
	if ws.lspMgr, err = lsp.Open(context.Background(), path, lsp.DefaultConfig()); err != nil {
		debuglog.Logf("lsp: open: %v", err)
	}
	if ws.mcpPool, err = mcp.LoadPoolInDir(proj.MCPConfigFile(), path, r.opencode.MCPServers()); err != nil {
		debuglog.Logf("mcp: load: %v", err)
		ws.mcpLoadFailed = true
	}
	r.mu.Lock()
	// Retain even a late load so shutdown reaps its services after builders exit.
	r.workspaces[path] = ws
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return nil, errors.New("tui: runtime is closed")
	}
	return ws, nil
}

// NewSession constructs an isolated Controller borrowing this runtime's services.
// A Workspace from another runtime is rejected; activation and drawing belong to
// the UI, not this ownership registry.
func (r *Runtime) NewSession(bus *Bus, ws *Workspace, resumePath string) (*Controller, error) {
	if r == nil || ws == nil || ws.runtime != r {
		return nil, errors.New("tui: workspace does not belong to this runtime")
	}
	if bus == nil {
		return nil, errors.New("tui: nil bus")
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errors.New("tui: runtime is closed")
	}
	if r.interactiveChildren && len(r.sessions)+r.childBuilders >= 12 {
		r.mu.Unlock()
		return nil, errors.New("cannot open session: retained session limit (12) reached")
	}
	r.childBuilders++
	r.builders.Add(1)
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.childBuilders--
		r.mu.Unlock()
		r.builders.Done()
	}()
	c, err := newController(bus, r, ws, resumePath)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.sessions[c] = struct{}{}
	closed := r.closed
	r.mu.Unlock()
	if closed {
		c.stopSession()
		return nil, errors.New("tui: runtime is closed")
	}
	return c, nil
}

// Close stops admission immediately and bounds the caller's wait. Actual cleanup
// keeps borrowed services alive until all sessions and job runners have exited;
// a timeout is not evidence that their tools have stopped.
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		r.cancelConstruction()
		sessions := make([]*Controller, 0, len(r.sessions))
		for c := range r.sessions {
			sessions = append(sessions, c)
		}
		go r.shutdown(sessions)
	}
	done := r.closeDone
	r.mu.Unlock()
	select {
	case <-done:
		return r.closeErr
	case <-time.After(3 * time.Second):
		return errors.New("runtime shutdown is still in progress: resources remain owned until runners exit")
	}
}

func (r *Runtime) shutdown(sessions []*Controller) {
	defer close(r.closeDone)
	for _, c := range sessions {
		c.stopSession()
	}
	if err := r.jobs.Close(); err != nil {
		r.closeErr = fmt.Errorf("runtime shutdown: %w", err)
	}
	r.builders.Wait()
	// Include constructors that completed after admission closed. Existing
	// sessions were cancelled above, without waiting for those constructors.
	r.mu.Lock()
	sessions = sessions[:0]
	for c := range r.sessions {
		sessions = append(sessions, c)
	}
	r.mu.Unlock()
	for _, c := range sessions {
		c.stopSession()
		<-c.closeDone
	}
	for _, ws := range r.workspaces {
		if ws.mcpPool != nil {
			if err := ws.mcpPool.Close(); err != nil {
				debuglog.Logf("mcp: close: %v", err)
			}
		}
		if ws.lspMgr != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if err := ws.lspMgr.Close(ctx); err != nil {
				debuglog.Logf("lsp: close: %v", err)
			}
			cancel()
		}
	}
}
