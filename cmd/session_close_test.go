package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tui/commands"
)

func TestSessionNavigationOffersClose(t *testing.T) {
	registry := commands.NewBuiltinRegistry()
	closed := 0
	registerSessionNavigation(registry, func() error { return nil }, func(int) error { return nil },
		func() error { closed++; return nil })
	require.True(t, registry.DispatchSlash("/close", commands.CommandContext{}),
		"retained agent sessions need a TUI action that closes their tab and releases its slot")
	require.Equal(t, 1, closed)
	for _, row := range registry.BuildPalette(commands.CommandContext{}) {
		if row.ID == "session-close" {
			row.Run()
			require.Equal(t, 2, closed)
			return
		}
	}
	t.Fatal("close action missing from palette")
}
