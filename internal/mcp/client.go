package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	protocolVersion = "2024-11-05"
)

// ToolDef is a subset of an MCP tools/list entry.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// Client is one MCP server session (any transport).
type Client interface {
	Initialize(ctx context.Context) error
	ListTools(ctx context.Context) ([]ToolDef, error)
	FindTool(ctx context.Context, name string) (*ToolDef, error)
	CallTool(ctx context.Context, name string, args map[string]any) (string, error)
	// Close permanently shuts down this client; create a new one to reconnect.
	Close() error
}

// NewClient builds a Client for cfg. Supported transports: stdio (default), http.
func NewClient(name string, cfg ServerConfig) (Client, error) {
	return newClient(name, cfg, "")
}

func newClient(name string, cfg ServerConfig, cwd string) (Client, error) {
	switch {
	case cfg.IsStdio():
		tr, err := newStdioTransport(name, cfg)
		if err != nil {
			return nil, err
		}
		tr.cwd = cwd
		return newSession(name, tr), nil
	case cfg.IsHTTP():
		tr, err := newHTTPTransport(name, cfg)
		if err != nil {
			return nil, err
		}
		return newSession(name, tr), nil
	default:
		t := strings.TrimSpace(cfg.Transport)
		if t == "" {
			t = "(empty)"
		}
		return nil, fmt.Errorf("server %q: unsupported transport %q (stdio or http)", name, t)
	}
}
