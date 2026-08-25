package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// transport is the wire protocol for one MCP connection.
// Implementations must be safe for use under session's mutex
// (one call at a time per session).
type transport interface {
	call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error)
	notify(ctx context.Context, method string, params map[string]any) error
	close() error
}

// session implements Client on top of a transport: handshake, tool cache, call.
type session struct {
	name string
	tr   transport

	mu    sync.Mutex
	tools []ToolDef
	ready bool
}

func newSession(name string, tr transport) *session {
	return &session{name: name, tr: tr}
}

func (s *session) Initialize(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.initLocked(ctx)
}

func (s *session) initLocked(ctx context.Context) error {
	if s.ready {
		return nil
	}
	if _, err := s.tr.call(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "cozyphi", "version": "0.1"},
	}); err != nil {
		_ = s.tr.close()
		return err
	}
	if err := s.tr.notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		_ = s.tr.close()
		return err
	}
	s.ready = true
	return nil
}

func (s *session) ListTools(ctx context.Context) ([]ToolDef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.initLocked(ctx); err != nil {
		return nil, err
	}
	if len(s.tools) > 0 {
		return cloneTools(s.tools), nil
	}
	raw, err := s.tr.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	tools, err := decodeToolsList(raw)
	if err != nil {
		return nil, err
	}
	s.tools = tools
	return cloneTools(tools), nil
}

func (s *session) FindTool(ctx context.Context, name string) (*ToolDef, error) {
	tools, err := s.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	for i := range tools {
		if tools[i].Name == name {
			t := tools[i]
			return &t, nil
		}
	}
	return nil, fmt.Errorf("tool %q not found on server %q", name, s.name)
}

func (s *session) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.initLocked(ctx); err != nil {
		return "", err
	}
	if args == nil {
		args = map[string]any{}
	}
	raw, err := s.tr.call(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}
	return extractToolContent(raw), nil
}

func (s *session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = false
	s.tools = nil
	return s.tr.close()
}

func cloneTools(in []ToolDef) []ToolDef {
	out := make([]ToolDef, len(in))
	copy(out, in)
	return out
}
