package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// errTransportDead marks a call error that left the transport unsynchronized
// with the server: a timeout or cancellation abandoned the request on the
// wire, or the pipe broke. Transports wrap it; the session reacts by dropping
// its handshake state so the next call re-initializes over a fresh connection.
var errTransportDead = errors.New("transport desynchronized")

// transport is the wire protocol for one MCP connection.
// Implementations must be safe for use under session's mutex
// (one call at a time per session). A call error wrapping errTransportDead
// means the transport closed itself; the session resets accordingly.
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
	if _, err := s.callTransport(ctx, "initialize", map[string]any{
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

// callTransport routes a call through the transport and folds wire death into
// session state: when the transport reports errTransportDead, the handshake
// and tool cache are dropped so the next call re-initializes instead of
// reading a stale pipe.
func (s *session) callTransport(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	raw, err := s.tr.call(ctx, method, params)
	if err != nil && errors.Is(err, errTransportDead) {
		s.ready = false
		s.tools = nil
		_ = s.tr.close()
	}
	return raw, err
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
	raw, err := s.callTransport(ctx, "tools/list", map[string]any{})
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
	raw, err := s.callTransport(ctx, "tools/call", map[string]any{
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
