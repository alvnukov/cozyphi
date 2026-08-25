package lsp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// validateQuery enforces the frozen V1 input matrix before any process start.
func validateQuery(q Query) error {
	switch q.Op {
	case OpDefinition, OpReferences, OpHover, OpCalls:
		if q.File == "" {
			return newError(ErrInvalid, "%s requires file", q.Op)
		}
		if q.Symbol == "" && (q.Line <= 0 || q.Character <= 0) {
			return newError(ErrInvalid, "%s requires symbol or line+character", q.Op)
		}
		if q.Symbol != "" && (q.Line > 0 || q.Character > 0) {
			return newError(ErrInvalid, "%s accepts symbol or line+character, not both", q.Op)
		}
		if q.Op == OpCalls && q.Direction != DirectionIncoming && q.Direction != DirectionOutgoing {
			return newError(ErrInvalid, "calls requires direction incoming|outgoing")
		}
		return nil
	case OpSymbols:
		if q.File == "" && q.Query == "" {
			return newError(ErrInvalid, "symbols requires file or query")
		}
		if q.File != "" && q.Query != "" {
			return newError(ErrInvalid, "symbols accepts file or query, not both")
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
// on the Manager lifetime rather than the first caller's query context.
func (m *Manager) clientFor(ctx context.Context, root string) (*client, error) {
	m.mu.Lock()
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

	task.client, task.err = startAndInitialize(m.lifetime, root, m.config)
	close(task.done)

	m.mu.Lock()
	delete(m.starts, root)
	if task.err == nil {
		m.clients[root] = task.client
	}
	m.mu.Unlock()
	return task.client, task.err
}

// startAndInitialize spawns gopls and completes initialize/initialized.
func startAndInitialize(ctx context.Context, root string, config Config) (*client, error) {
	c, err := startClient(ctx, root, config)
	if err != nil {
		return nil, err
	}
	if err := c.initialize(ctx, root, config); err != nil {
		_ = c.proc.Close(0)
		return nil, err
	}
	return c, nil
}

// syncDocument sends didOpen once per client generation for the file's current
// disk snapshot and returns that exact text for position conversion.
func (c *client) syncDocument(ctx context.Context, file string) (string, error) {
	raw, err := readSnapshot(file)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(raw) {
		return "", newError(ErrInvalid, "%s is not valid UTF-8", file)
	}
	uri := uriFromPath(file)
	c.openedMu.Lock()
	opened := c.opened[uri]
	c.openedMu.Unlock()
	if !opened {
		if err := c.notify(ctx, "textDocument/didOpen", map[string]any{
			"textDocument": map[string]any{
				"uri":        uri,
				"languageId": "go",
				"version":    1,
				"text":       string(raw),
			},
		}); err != nil {
			return "", err
		}
		c.openedMu.Lock()
		c.opened[uri] = true
		c.openedMu.Unlock()
	}
	return string(raw), nil
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
