package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type statusClient struct{ err error }

func (c statusClient) Initialize(context.Context) error { return c.err }
func (c statusClient) ListTools(context.Context) ([]ToolDef, error) {
	return []ToolDef{{Name: "read"}}, c.err
}

func (c statusClient) FindTool(context.Context, string) (*ToolDef, error) {
	return &ToolDef{Name: "read"}, c.err
}

func (c statusClient) CallTool(context.Context, string, map[string]any) (string, error) {
	return "ok", c.err
}
func (c statusClient) Close() error { return c.err }

func TestPoolServerStatusesTrackObservedConnectivity(t *testing.T) {
	p := NewPool(map[string]ServerConfig{"zeta": {}, "alpha": {}, "cancelled": {}})
	p.clients["alpha"] = statusClient{}
	p.clients["zeta"] = statusClient{err: errors.New("connection refused")}
	p.clients["cancelled"] = statusClient{err: context.Canceled}

	initial := p.ServerStatuses()
	require.Len(t, initial, 3)
	assert.Equal(t, "alpha", initial[0].Name)
	assert.Equal(t, StateConfigured, initial[0].State)

	_, err := p.ListTools(t.Context(), "alpha")
	require.NoError(t, err)
	_, err = p.ListTools(t.Context(), "zeta")
	require.Error(t, err)
	_, err = p.ListTools(t.Context(), "cancelled")
	require.ErrorIs(t, err, context.Canceled)

	got := p.ServerStatuses()
	assert.Equal(t, StateConnected, got[0].State)
	assert.Equal(t, StateConfigured, got[1].State, "user cancellation must not mark a healthy server failed")
	assert.Equal(t, StateFailed, got[2].State)
}
