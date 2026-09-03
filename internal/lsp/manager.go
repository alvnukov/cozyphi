package lsp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Manager owns the gopls lifecycle for one workspace process. Only Open and
// Close create or tear down subprocesses; Query borrows the shared runtime.
type Manager struct {
	lifetime context.Context
	cancel   context.CancelFunc

	workspace string
	config    Config

	mu      sync.Mutex
	closed  bool
	clients map[string]*client
	starts  map[string]*startTask
	// breaker refuses repeated gopls starts for one root after crashes.
	breaker *startBreaker
	// lastStartErr is the bounded sanitized reason of the most recent failed
	// client start; languages reports it.
	lastStartErr string

	closeOnce sync.Once
	closeErr  error
}

// startTask coalesces concurrent startup for one canonical root.
type startTask struct {
	done   chan struct{}
	client *client
	err    error
}

// Open validates the workspace and config and starts nothing. Disabled config
// returns (nil, nil) so assembly registers no tool. lifetime owns subprocesses;
// canceling it kills the whole tree promptly.
func Open(lifetime context.Context, workspace string, config Config) (*Manager, error) {
	if !config.Enabled {
		return nil, nil
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, errors.New("lsp: empty workspace")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("lsp: resolve workspace: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("lsp: workspace: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("lsp: workspace is not a directory: %s", abs)
	}
	if len(config.Gopls.Command) == 0 {
		return nil, errors.New("lsp: gopls command must not be empty")
	}
	ctx, cancel := context.WithCancel(lifetime)
	return &Manager{
		lifetime:  ctx,
		cancel:    cancel,
		workspace: abs,
		config:    config,
		clients:   make(map[string]*client),
		starts:    make(map[string]*startTask),
		breaker:   newStartBreaker(startBreakerWindow),
	}, nil
}

// Query validates the operation and dispatches it to the shared client for the
// canonical Go root. Query ctx cancels only this request; the Manager lifetime
// owns the process.
func (m *Manager) Query(ctx context.Context, q Query) (Result, error) {
	if m == nil {
		return Result{}, newError(ErrClosed, "manager is nil")
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Result{}, newError(ErrClosed, "manager is closed")
	}
	m.mu.Unlock()

	if err := validateQuery(q); err != nil {
		return Result{}, err
	}
	if q.Limit <= 0 {
		q.Limit = DefaultItemLimit
	}
	if q.Limit > MaxItemLimit {
		q.Limit = MaxItemLimit
	}

	// languages is the one operation that never touches a server: it reports
	// the frozen V1 inventory without resolving roots or starting gopls.
	if q.Op == OpLanguages {
		return m.languagesStatus(), nil
	}

	// Reject unimplemented operations before any process start.
	handler, ok := opHandlers[q.Op]
	if !ok {
		return Result{}, newError(ErrUnsupported, "%s is not implemented", q.Op)
	}

	// A navigational symbol without a file resolves workspace-wide first, so
	// the model can ask "where is Foo" without knowing the file. The step runs
	// on the workspace-root client; the resolved file then picks its own root.
	if navigational(q.Op) && q.File == "" {
		resolved, done, res, err := m.resolveWorkspaceSymbol(ctx, q)
		if err != nil {
			return Result{}, err
		}
		if done {
			return res, nil
		}
		q = resolved
	}

	root := m.goRoot(q.File)
	if root == "" {
		return Result{}, newError(ErrInvalid, "cannot determine Go root for %s", q.File)
	}
	// Every server-touching query runs under its frozen deadline; ordinary
	// operations get 15s, workspace-wide symbol search 30s.
	ctx, cancel := context.WithTimeout(ctx, timeoutFor(q))
	defer cancel()

	c, err := m.clientFor(ctx, root)
	if err != nil {
		return Result{}, err
	}

	return handler(m, ctx, c, q)
}

// opHandlers dispatches the frozen V1 operations to the shared client. Each
// handler synchronizes its documents first, gates itself on the server
// capabilities stored for this client generation, and normalizes its results.
var opHandlers = map[Operation]func(*Manager, context.Context, *client, Query) (Result, error){
	OpDefinition:      (*Manager).definition,
	OpReferences:      (*Manager).references,
	OpImplementations: (*Manager).implementations,
	OpTypeDefinition:  (*Manager).typeDefinition,
	OpHover:           (*Manager).hover,
	OpSymbols:         (*Manager).symbols,
	OpCalls:           (*Manager).calls,
	OpDiagnostics:     (*Manager).diagnostics,
}

// Close rejects new calls, closes every live client gracefully, then releases
// the lifetime. It is idempotent and race-safe. The context is kept for
// callers' cancellation chains; shutdown runs on a fixed grace period.
func (m *Manager) Close(_ context.Context) error {
	if m == nil {
		return nil
	}
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		clients := make([]*client, 0, len(m.clients))
		for _, c := range m.clients {
			clients = append(clients, c)
		}
		m.clients = make(map[string]*client)
		m.starts = make(map[string]*startTask)
		m.mu.Unlock()

		for _, c := range clients {
			c.shutdown(graceFrom())
		}
		m.cancel()
	})
	return m.closeErr
}

func graceFrom() time.Duration {
	return shutdownGrace
}
