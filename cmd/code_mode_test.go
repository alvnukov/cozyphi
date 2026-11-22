package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
)

func TestCodeModeSelectionForBothEntryPoints(t *testing.T) {
	for _, flag := range []string{"--code-mode", "--direct-tools"} {
		tui, err := parseTUIArgs([]string{flag})
		require.NoError(t, err)
		run, err := parseRunArgs([]string{flag, "-p", "task"})
		require.NoError(t, err)
		for _, configured := range []bool{true, false} {
			cfg := &project.Config{CodeMode: project.CodeModeConfig{Enabled: configured}}
			require.Equal(t, configured, codeModeEnabled(cfg, ""))
			require.Equal(t, flag == "--code-mode", codeModeEnabled(cfg, tui.toolSurface))
			require.Equal(t, flag == "--code-mode", codeModeEnabled(cfg, run.toolSurface))
		}
	}
}
