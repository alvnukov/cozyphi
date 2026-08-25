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

	// Reject unimplemented operations before any process start: diagnostics
	// and languages still fail without launching gopls.
	handler, ok := navigationHandlers[q.Op]
	if !ok {
		return Result{}, newError(ErrUnsupported, "%s is not implemented", q.Op)
	}

	root := m.goRoot(q.File)
	if root == "" {
		return Result{}, newError(ErrInvalid, "cannot determine Go root for %s", q.File)
	}
	c, err := m.clientFor(ctx, root)
	if err != nil {
		return Result{}, err
	}

	return handler(m, ctx, c, q)
}

// navigationHandlers dispatches the frozen V1 navigation operations to the
// shared client. Each handler gates itself on the server capabilities stored
// for this client generation and normalizes its own results.
var navigationHandlers = map[Operation]func(*Manager, context.Context, *client, Query) (Result, error){
	OpDefinition: (*Manager).definition,
	OpReferences: (*Manager).references,
	OpHover:      (*Manager).hover,
	OpSymbols:    (*Manager).symbols,
	OpCalls:      (*Manager).calls,
}

// Close rejects new calls, closes every live client gracefully, then releases
// the lifetime. It is idempotent and race-safe.
func (m *Manager) Close(ctx context.Context) error {
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
			c.shutdown(graceFrom(ctx))
		}
		m.cancel()
	})
	return m.closeErr
}

func graceFrom(ctx context.Context) time.Duration {
	_ = ctx
	return shutdownGrace
}
