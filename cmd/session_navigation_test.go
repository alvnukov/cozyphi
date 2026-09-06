package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tui/commands"
)

func TestSessionCommandsUseTheirInjectedNavigation(t *testing.T) {
	first, second := commands.NewBuiltinRegistry(), commands.NewBuiltinRegistry()
	opened := [2]int{}
	selected := [2]int{}
	registerSessionNavigation(
		first,
		func() error { opened[0]++; return nil },
		func(n int) error { selected[0] = n; return nil },
		func() error { return nil },
	)
	registerSessionNavigation(
		second,
		func() error { opened[1]++; return nil },
		func(n int) error { selected[1] = n; return nil },
		func() error { return nil },
	)

	require.True(t, first.DispatchSlash("/new", commands.CommandContext{}))
	require.Equal(t, [2]int{1, 0}, opened)
	require.True(t, second.DispatchSlash("/switch 3", commands.CommandContext{}))
	require.Equal(t, [2]int{0, 3}, selected)
	require.True(t, first.DispatchSlash("/switch 2", commands.CommandContext{}))
	require.Equal(t, [2]int{2, 3}, selected)
	require.False(t, first.DispatchSlash("/switcher", commands.CommandContext{}))
}

func TestSessionCommandsRejectArgumentsBeforeNavigation(t *testing.T) {
	registry := commands.NewBuiltinRegistry()
	calls := 0
	registerSessionNavigation(registry, func() error { calls++; return nil }, func(int) error { calls++; return nil },
		func() error { calls++; return nil })
	for _, text := range []string{"/new extra", "/switch", "/switch 0", "/switch -1", "/switch nope", "/switch 1 2", "/close extra"} {
		require.True(t, registry.DispatchSlash(text, commands.CommandContext{}), text)
	}
	require.Zero(t, calls)
}
