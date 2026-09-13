package commands

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShellTaskCommandsOpenOneBrowser(t *testing.T) {
	r := NewBuiltinRegistry()
	host := &fakeHost{}
	require.True(t, r.DispatchSlash("/tasks", CommandContext{Host: host}))
	require.True(t, r.DispatchSlash("/bashes", CommandContext{Host: host}))
	require.Equal(t, 2, host.shellsOpen)
}
