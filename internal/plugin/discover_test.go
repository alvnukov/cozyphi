package plugin_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/plugin"
)

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	writeFile(t, path, string(raw))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// pluginRoot plants a plugin with one skill and a hooks.json; manifest may be nil.
func pluginRoot(t *testing.T, manifest map[string]any) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: d\n---\n")
	writeFile(t, filepath.Join(root, "hooks", "hooks.json"), `{"hooks":{}}`)
	if manifest != nil {
		writeJSON(t, filepath.Join(root, ".claude-plugin", "plugin.json"), manifest)
	}
	return root
}

type install = map[string]string

func installed(t *testing.T, claudeDir string, plugins map[string][]install) {
	t.Helper()
	writeJSON(t, filepath.Join(claudeDir, "plugins", "installed_plugins.json"),
		map[string]any{"version": 2, "plugins": plugins})
}

func enable(t *testing.T, settingsFile string, enabled map[string]any) {
	t.Helper()
	writeJSON(t, settingsFile, map[string]any{"enabledPlugins": enabled})
}

func discover(t *testing.T, cfg plugin.Config, projectRoot string) ([]plugin.Plugin, []plugin.Warning) {
	t.Helper()
	t.Setenv(plugin.EnvPlugins, "")
	cfg.Enabled = true
	return plugin.Discover(cfg, projectRoot)
}

func TestDiscoverEnabledClaudePlugin(t *testing.T) {
	claude, project := t.TempDir(), t.TempDir()
	root := pluginRoot(t, map[string]any{"name": "ignored-for-installed"})
	const id = "superpowers@claude-plugins-official"
	installed(t, claude, map[string][]install{id: {{"scope": "user", "installPath": root, "version": "6.4.1"}}})
	enable(t, filepath.Join(claude, "settings.json"), map[string]any{id: true})

	got, warns := discover(t, plugin.Config{ClaudeDir: claude}, project)
	require.Empty(t, warns)
	require.Equal(t, []plugin.Plugin{{
		ID:        id,
		Name:      "superpowers",
		Root:      root,
		DataDir:   filepath.Join(claude, "plugins", "data", "superpowers-claude-plugins-official"),
		SkillDirs: []string{filepath.Join(root, "skills")},
		HookFiles: []string{filepath.Join(root, "hooks", "hooks.json")},
	}}, got)
}

func TestDiscoverEnablesOnlyLiteralTrue(t *testing.T) {
	claude := t.TempDir()
	plugins := map[string][]install{}
	enabled := map[string]any{}
	for id, value := range map[string]any{"a@m": "true", "b@m": 1, "c@m": false, "d@m": nil} {
		plugins[id] = []install{{"installPath": pluginRoot(t, nil)}}
		enabled[id] = value
	}
	plugins["e@m"] = []install{{"installPath": pluginRoot(t, nil)}} // no key at all
	installed(t, claude, plugins)
	enable(t, filepath.Join(claude, "settings.json"), enabled)

	got, _ := discover(t, plugin.Config{ClaudeDir: claude}, t.TempDir())
	require.Empty(t, got)
}

func TestDiscoverLayersProjectSettings(t *testing.T) {
	claude, project := t.TempDir(), t.TempDir()
	installed(t, claude, map[string][]install{
		"on@m":  {{"installPath": pluginRoot(t, nil)}},
		"off@m": {{"installPath": pluginRoot(t, nil)}},
	})
	enable(t, filepath.Join(claude, "settings.json"), map[string]any{"on@m": true, "off@m": true})
	enable(t, filepath.Join(project, ".claude", "settings.json"), map[string]any{"on@m": false, "off@m": false})
	enable(t, filepath.Join(project, ".claude", "settings.local.json"), map[string]any{"on@m": true})

	got, warns := discover(t, plugin.Config{ClaudeDir: claude}, project)
	require.Empty(t, warns)
	require.Len(t, got, 1)
	require.Equal(t, "on@m", got[0].ID)
}

func TestDiscoverProjectInstallMatchesThroughSymlink(t *testing.T) {
	claude, project := t.TempDir(), t.TempDir()
	link := filepath.Join(t.TempDir(), "project-link")
	require.NoError(t, os.Symlink(project, link))
	userRoot, projectRoot, otherRoot := pluginRoot(t, nil), pluginRoot(t, nil), pluginRoot(t, nil)
	installed(t, claude, map[string][]install{"p@m": {
		{"scope": "user", "installPath": userRoot},
		{"scope": "project", "installPath": otherRoot, "projectPath": t.TempDir()},
		{"scope": "project", "installPath": projectRoot, "projectPath": link},
	}})
	enable(t, filepath.Join(claude, "settings.json"), map[string]any{"p@m": true})

	got, _ := discover(t, plugin.Config{ClaudeDir: claude}, project)
	require.Len(t, got, 1)
	require.Equal(t, projectRoot, got[0].Root, "this project's install beats the user install")

	got, _ = discover(t, plugin.Config{ClaudeDir: claude}, t.TempDir())
	require.Len(t, got, 1)
	require.Equal(t, userRoot, got[0].Root, "another project's install is skipped")
}

func TestDiscoverRejectsUnknownVersionAndGarbage(t *testing.T) {
	claude := t.TempDir()
	file := filepath.Join(claude, "plugins", "installed_plugins.json")
	writeJSON(t, file, map[string]any{"version": 3, "plugins": map[string]any{}})
	got, warns := discover(t, plugin.Config{ClaudeDir: claude}, t.TempDir())
	require.Empty(t, got)
	require.Len(t, warns, 1)
	require.Contains(t, warns[0].String(), "version 3 is not supported")

	writeFile(t, file, "{")
	got, warns = discover(t, plugin.Config{ClaudeDir: claude}, t.TempDir())
	require.Empty(t, got)
	require.Len(t, warns, 1)
	require.Contains(t, warns[0].String(), "invalid JSON")
}

func TestDiscoverLocalPaths(t *testing.T) {
	named := pluginRoot(t, map[string]any{"name": "mine"})
	unnamed := pluginRoot(t, nil)
	file := filepath.Join(t.TempDir(), "file")
	writeFile(t, file, "")
	dataRoot := t.TempDir()

	got, warns := discover(t, plugin.Config{
		Paths:        []string{named, unnamed, "relative/dir", filepath.Join(dataRoot, "absent"), file},
		LocalDataDir: dataRoot,
	}, t.TempDir())
	require.Len(t, got, 2)
	require.Equal(t, "mine", got[0].ID)
	require.Equal(t, "mine", got[0].Name)
	require.Equal(t, filepath.Join(dataRoot, "mine"), got[0].DataDir)
	require.Equal(t, filepath.Base(unnamed), got[1].Name)
	require.Len(t, warns, 3)
	joined := warnText(warns)
	require.Contains(t, joined, "relative/dir: relative plugin path")
	require.Contains(t, joined, "is not a directory")
}

func TestDiscoverKeepsFirstOfTwoSameNames(t *testing.T) {
	claude := t.TempDir()
	installed(t, claude, map[string][]install{"demo@m": {{"installPath": pluginRoot(t, nil)}}})
	enable(t, filepath.Join(claude, "settings.json"), map[string]any{"demo@m": true})
	local := pluginRoot(t, map[string]any{"name": "demo"})

	got, warns := discover(t, plugin.Config{ClaudeDir: claude, Paths: []string{local}}, t.TempDir())
	require.Len(t, got, 1)
	require.Equal(t, "demo@m", got[0].ID)
	require.Len(t, warns, 1)
	require.Contains(t, warns[0].String(), `plugin name "demo" is already used by demo@m`)
}

func TestDiscoverWarnsOnUnsupportedComponentsAndManifestPaths(t *testing.T) {
	root := pluginRoot(t, map[string]any{
		"name":   "demo",
		"skills": []string{"./extra", "../escape"},
		"hooks":  map[string]any{"SessionStart": []any{}},
	})
	writeFile(t, filepath.Join(root, "extra", "x", "SKILL.md"), "---\nname: x\ndescription: x\n---\n")
	writeFile(t, filepath.Join(root, "commands", "c.md"), "")
	writeFile(t, filepath.Join(root, "agents", "a.md"), "")
	writeFile(t, filepath.Join(root, ".mcp.json"), "{}")

	got, warns := discover(t, plugin.Config{Paths: []string{root}, LocalDataDir: t.TempDir()}, t.TempDir())
	require.Len(t, got, 1)
	require.Equal(t, []string{filepath.Join(root, "skills"), filepath.Join(root, "extra")}, got[0].SkillDirs)
	joined := warnText(warns)
	for _, want := range []string{
		`skills path "../escape" leaves the plugin root`, "inline hooks in plugin.json are not supported",
		"commands/ is not supported", "agents/ is not supported", ".mcp.json is not supported",
	} {
		require.Contains(t, joined, want)
	}
}

// A warning reaches /hooks list, where the user reads it without the code:
// each one names the fix after " — ", not only the problem.
func TestDiscoverWarningsSayWhatToDo(t *testing.T) {
	claude := t.TempDir()
	writeJSON(t, filepath.Join(claude, "plugins", "installed_plugins.json"),
		map[string]any{"version": 3, "plugins": map[string]any{}})
	root := pluginRoot(t, map[string]any{"name": "demo", "skills": "../escape"})
	writeFile(t, filepath.Join(root, "commands", "c.md"), "")

	_, warns := discover(t, plugin.Config{ClaudeDir: claude, Paths: []string{root}, LocalDataDir: t.TempDir()},
		t.TempDir())
	require.Len(t, warns, 3)
	for _, w := range warns {
		require.Contains(t, w.Msg, " — ", "warning without a fix: %s", w)
	}
}

func TestDiscoverIsOffWhenDisabled(t *testing.T) {
	cfg := plugin.Config{Paths: []string{pluginRoot(t, nil)}, LocalDataDir: t.TempDir()}
	t.Setenv(plugin.EnvPlugins, "")
	got, _ := plugin.Discover(cfg, t.TempDir()) // Enabled is false
	require.Empty(t, got)

	cfg.Enabled = true
	t.Setenv(plugin.EnvPlugins, "OFF")
	require.True(t, plugin.Disabled())
	got, _ = plugin.Discover(cfg, t.TempDir())
	require.Empty(t, got)
}

func TestPluginSourcesCarryNamespaceAndVars(t *testing.T) {
	p := plugin.Plugin{
		Name: "demo", Root: "/r", DataDir: "/d",
		SkillDirs: []string{"/r/skills"}, HookFiles: []string{"/r/hooks/hooks.json"},
	}
	vars := map[string]string{
		hooks.EnvClaudePluginRoot: "/r", hooks.EnvClaudePluginData: "/d", hooks.EnvClaudeProjectDir: "/proj",
	}
	require.Equal(t, vars, p.Vars("/proj"))
	src := p.SkillSources("/proj")
	require.Len(t, src, 1)
	require.Equal(t, "demo", src[0].Namespace)
	require.Equal(t, vars, src[0].Vars)
	require.Equal(t, hooks.PluginHooks{Name: "demo", Files: p.HookFiles, Vars: vars}, p.HookSources("/proj"))
}

func warnText(warns []plugin.Warning) string {
	out := make([]string, 0, len(warns))
	for _, w := range warns {
		out = append(out, w.String())
	}
	return strings.Join(out, "\n")
}
