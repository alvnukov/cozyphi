package controller

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/project"
)

// pluginWorkspace sets up HOME with Claude Code's records for one enabled
// plugin whose hooks.json holds hooksJSON, and returns the discovered project.
func pluginWorkspace(t *testing.T, hooksJSON string) (*project.Project, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_HOOKS", "")
	t.Setenv("COZYPHI_PLUGINS", "")
	root := filepath.Join(home, ".claude", "plugins", "cache", "demo")
	write := func(path, content string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	write(filepath.Join(root, "hooks", "hooks.json"), hooksJSON)
	write(filepath.Join(root, "commands", "c.md"), "")
	inst, err := json.Marshal(map[string]any{"version": 2, "plugins": map[string]any{
		"demo@m": []any{map[string]string{"installPath": root}},
	}})
	require.NoError(t, err)
	write(filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), string(inst))
	write(filepath.Join(home, ".claude", "settings.json"), `{"enabledPlugins":{"demo@m":true}}`)

	cwd, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())
	return proj, cwd
}

func TestListHooksShowsPluginHooksAndWarnings(t *testing.T) {
	proj, _ := pluginWorkspace(t, `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"true"}]}]}}`)
	c := &Controller{proj: proj}

	found, warns, err := c.ListHooks()
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, "plugin:demo/SessionStart#1", found[0].Manifest.Name)
	require.Equal(t, "plugin:demo", found[0].Source)
	text := make([]string, 0, len(warns))
	for _, w := range warns {
		text = append(text, w.String())
	}
	require.Contains(t, strings.Join(text, "\n"), "demo@m: commands/ is not supported")

	loaded, _, err := c.ReloadHooks()
	require.NoError(t, err)
	require.Equal(t, 1, loaded)
}

// A plugin hook that failed at runtime is reported by /hooks list: the session
// carried on without it, so the list is where the user learns it broke.
func TestListHooksReportsPluginHookRuntimeFailures(t *testing.T) {
	proj, cwd := pluginWorkspace(t,
		`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo broken >&2; exit 1"}]}]}}`)
	c := &Controller{proj: proj}
	_, _, err := c.ReloadHooks()
	require.NoError(t, err)

	_, warns, err := c.ListHooks()
	require.NoError(t, err)
	for _, w := range warns {
		require.NotContains(t, w.String(), "exited 1", "nothing has run yet")
	}

	c.Hooks().SessionStart(t.Context(), hooks.SessionEvent{SessionID: "s1", Cwd: cwd, Reason: hooks.ReasonStartup})

	_, warns, err = c.ListHooks()
	require.NoError(t, err)
	text := make([]string, 0, len(warns))
	for _, w := range warns {
		text = append(text, w.String())
	}
	require.Contains(t, strings.Join(text, "\n"), "plugin:demo/SessionStart#1: ")
	require.Contains(t, strings.Join(text, "\n"), "exited 1: broken")
}
