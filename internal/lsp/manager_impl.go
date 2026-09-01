package lsp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// validateQuery enforces the input matrix before any process start. The
// navigational matrix is deliberately tolerant: a symbol, a full position, or
// any combination of the two is a valid target, and a position refines an
// ambiguous symbol instead of conflicting with it.
func validateQuery(q Query) error {
	switch q.Op {
	case OpDefinition, OpReferences, OpImplementations, OpTypeDefinition, OpHover, OpCalls:
		if q.Character > 0 && q.Line <= 0 {
			return newError(ErrInvalid, "%s: character requires line", q.Op)
		}
		if q.Symbol == "" {
			if q.File == "" {
				return newError(ErrInvalid, "%s requires symbol or file with line+character", q.Op)
			}
			if q.Line <= 0 {
				return newError(ErrInvalid, "%s requires symbol or line+character", q.Op)
			}
			if q.Character <= 0 {
				return newError(ErrInvalid, "%s with line alone needs character or symbol", q.Op)
			}
		}
		if q.File == "" && q.Line > 0 {
			return newError(ErrInvalid, "%s: line requires file", q.Op)
		}
		if q.Op == OpCalls && q.Direction != DirectionIncoming && q.Direction != DirectionOutgoing {
			return newError(ErrInvalid, "calls requires direction incoming|outgoing")
		}
		return nil
	case OpSymbols:
		if q.File == "" && q.Query == "" {
			return newError(ErrInvalid, "symbols requires file or query")
		}
		return nil
	case OpDiagnostics:
		if q.File == "" {
			return newError(ErrInvalid, "diagnostics requires file")
		}
		return nil
	case OpLanguages:
		if q.File != "" || q.Symbol != "" || q.Query != "" || q.Line > 0 || q.Character > 0 || q.Direction != "" {
			return newError(ErrInvalid, "languages takes no target fields")
		}
		return nil
	default:
		return newError(ErrInvalid, "unknown operation %q", q.Op)
	}
}

// timeoutFor selects the frozen per-query deadline: workspace-wide symbol
// search gets the long budget, every other server operation the ordinary one.
func timeoutFor(q Query) time.Duration {
	if q.Op == OpSymbols && q.Query != "" {
		return workspaceSymbolTimeout
	}
	return queryTimeout
}

// goRoot selects the canonical Go root for a file: nearest go.work, then
// nearest go.mod, then the workspace root. It never leaves the workspace.
func (m *Manager) goRoot(file string) string {
	if file == "" {
		return m.workspace
	}
	start := filepath.Dir(file)
	if root := nearestMarker(start, m.workspace, "go.work"); root != "" {
		return root
	}
	if root := nearestMarker(start, m.workspace, "go.mod"); root != "" {
		return root
	}
	return m.workspace
}

// nearestMarker walks up from start (inclusive) to workspace (inclusive) and
// returns the first directory containing marker, or "".
func nearestMarker(start, workspace, marker string) string {
	workspace = filepath.Clean(workspace)
	for dir := start; ; dir = filepath.Dir(dir) {
		if st, err := os.Stat(filepath.Join(dir, marker)); err == nil && !st.IsDir() {
			return dir
		}
		if dir == workspace {
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		if !strings.HasPrefix(dir, workspace) && dir != workspace {
			return ""
		}
	}
}

// clientFor returns the shared client for root, coalescing concurrent startup
// on the Manager lifetime rather than the first caller's query context. A
// live client is reused across queries — document sync and diagnostics caches
// are per client generation — and a dead generation is replaced by a restart.
func (m *Manager) clientFor(ctx context.Context, root string) (*client, error) {
	m.mu.Lock()
	if c, ok := m.clients[root]; ok {
		if c.alive() {
			m.mu.Unlock()
			return c, nil
		}
		// Dead generation: drop it so the restart below replaces it.
		delete(m.clients, root)
	}
	if task, ok := m.starts[root]; ok {
		m.mu.Unlock()
		select {
		case <-task.done:
			return task.client, task.err
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-m.lifetime.Done():
			return nil, newError(ErrClosed, "manager is closed")
		}
	}
	task := &startTask{done: make(chan struct{})}
	m.starts[root] = task
	m.mu.Unlock()

	// Resolve the executable before spawning: a bare name must go through
	// ~/.cozyphi/bin and then PATH, never the process working directory, and
	// a miss fails closed without launching anything.
	exe, ok := resolveGopls(m.config.Gopls.Command)
	if !ok {
		err := newError(
			ErrUnavailable,
			"gopls executable not found; install gopls or set gopls.command in ~/.cozyphi/lsp.json",
		)
		task.err = err
		m.mu.Lock()
		m.lastStartErr, _ = boundText(err.Error())
		delete(m.starts, root)
		m.mu.Unlock()
		close(task.done)
		return nil, err
	}

	// The breaker records only real spawn attempts: config validation and
	// binary lookup above never consume quota, a refusal starts nothing.
	if retryAfter, ok := m.breaker.allow(root); !ok {
		err := newError(
			ErrUnavailable,
			"gopls start refused: %d attempts in the last %s; retry_after_seconds=%d",
			maxStartAttempts, startBreakerWindow, int(retryAfter.Seconds()),
		)
		task.err = err
		m.mu.Lock()
		m.lastStartErr, _ = boundText(err.Error())
		delete(m.starts, root)
		m.mu.Unlock()
		close(task.done)
		return nil, err
	}
	m.breaker.record(root)

	task.client, task.err = startAndInitialize(m.lifetime, root, m.workspace, exe, m.config)
	close(task.done)

	m.mu.Lock()
	delete(m.starts, root)
	if task.err == nil {
		m.clients[root] = task.client
		m.lastStartErr = ""
	} else {
		m.lastStartErr, _ = boundText(task.err.Error())
	}
	m.mu.Unlock()
	return task.client, task.err
}

// startAndInitialize spawns the resolved executable and completes
// initialize/initialized under the frozen handshake deadline. The process
// lifetime stays the Manager lifetime: only the handshake is bounded.
func startAndInitialize(ctx context.Context, root, workspace, exe string, config Config) (*client, error) {
	argv := append([]string{exe}, config.Gopls.Command[1:]...)
	c, err := startClient(ctx, root, workspace, argv, config)
	if err != nil {
		return nil, err
	}
	initCtx, cancel := context.WithTimeout(ctx, initializeTimeout)
	defer cancel()
	if err := c.initialize(initCtx, root, config); err != nil {
		_ = c.proc.Close(0)
		if initCtx.Err() != nil {
			return nil, newError(ErrUnavailable, "gopls did not initialize within %s", initializeTimeout)
		}
		return nil, err
	}
	return c, nil
}

// alive reports whether the client connection is still usable.
func (c *client) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// readSnapshot reads a bounded disk snapshot for document sync and positions.
func readSnapshot(file string) ([]byte, error) {
	st, err := os.Stat(file)
	if err != nil {
		return nil, fmt.Errorf("lsp: read snapshot: %w", err)
	}
	if st.Size() > MaxFileBytes {
		return nil, newError(ErrInvalid, "%s exceeds %d bytes", file, MaxFileBytes)
	}
	return os.ReadFile(file)
}
