package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type statusHost struct {
	fakeHost
	opens int
}

func (h *statusHost) ShowStatus() { h.opens++ }

func TestStatusSlashAndPalette(t *testing.T) {
	r := NewBuiltinRegistry()
	h := &statusHost{}
	ctx := CommandContext{Host: h}
	require.True(t, r.DispatchSlash("/status", ctx))
	assert.Equal(t, 1, h.opens)
	found := false
	for _, command := range r.BuildPalette(ctx) {
		if command.ID == "status" {
			command.Run()
			found = true
		}
	}
	require.True(t, found)
	assert.Equal(t, 2, h.opens)
	require.True(t, r.DispatchSlash("/status", CommandContext{}))
}
