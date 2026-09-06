package commands

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type renameHost struct {
	fakeHost
	title string
	err   error
}

func (h *renameHost) RenameSession(title string) error { h.title = title; return h.err }

func TestRenameDispatch(t *testing.T) {
	r := NewBuiltinRegistry()
	h := &renameHost{}
	require.True(t, r.DispatchSlash("/rename", CommandContext{Host: h}))
	require.Contains(t, h.toastMsg, "usage: /rename <title>")
	require.Empty(t, h.title)
	require.True(t, r.DispatchSlash("/rename fix the build", CommandContext{Host: h}))
	require.Equal(t, "fix the build", h.title)
	h.err = errors.New("rename session: title must be at most 60 runes")
	require.True(t, r.DispatchSlash("/rename another", CommandContext{Host: h}))
	require.Equal(t, h.err.Error(), h.toastMsg)
	missing := &fakeHost{}
	require.True(t, r.DispatchSlash("/rename another", CommandContext{Host: missing}))
	require.Contains(t, missing.toastMsg, "no session is open")
}
