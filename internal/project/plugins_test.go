package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plugin"
)

// pluginProject starts a project in a fresh HOME whose Claude Code has one
// enabled plugin with a skill and a hooks.json.
func pluginProject(t *testing.T, config string) (*Project, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(plugin.EnvPlugins, "")
	t.Setenv("COZYPHI_SKILL_PATH", "")

	root := filepath.Join(home, ".claude", "plugins", "cache", "demo")
	write := func(path, content string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	write(filepath.Join(root, "skills", "s", "SKILL.md"), "---\nname: s\ndescription: d\n---\n")
	write(filepath.Join(root, "hooks", "hooks.json"), `{"hooks":{}}`)
	inst, err := json.Marshal(map[string]any{"version": 2, "plugins": map[string]any{
		"demo@m": []any{map[string]string{"installPath": root}},
	}})
	require.NoError(t, err)
	write(filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), string(inst))
	write(filepath.Join(home, ".claude", "settings.json"), `{"enabledPlugins":{"demo@m":true}}`)
	write(filepath.Join(home, ".cozyphi", "config.yaml"), config)

	p, err := Discover(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, p.LoadConfig())
	return p, root
}

func TestLoadConfigAddsEnabledPluginSources(t *testing.T) {
	p, root := pluginProject(t, "")
	cfg := p.Config()
	require.True(t, cfg.Plugins.Enabled)
	require.Equal(t, p.Global().ClaudeDir(), cfg.Plugins.ClaudeDir)
	require.Len(t, cfg.Skills, 2)
	require.Equal(t, p.Global().SkillsDir(), cfg.Skills[0].Dir)
	require.Empty(t, cfg.Skills[0].Namespace)
	require.Equal(t, filepath.Join(root, "skills"), cfg.Skills[1].Dir)
	require.Equal(t, "demo", cfg.Skills[1].Namespace)
	require.Len(t, cfg.PluginHooks, 1)
	require.Equal(t, "demo", cfg.PluginHooks[0].Name)
	require.Empty(t, cfg.PluginWarnings())
}

func TestLoadConfigHonoursPluginsDisabled(t *testing.T) {
	p, _ := pluginProject(t, "plugins:\n  enabled: false\n")
	require.Len(t, p.Config().Skills, 1)
	require.Empty(t, p.Config().PluginHooks)
}

func TestLoadConfigHonoursPluginsEnvOff(t *testing.T) {
	p, _ := pluginProject(t, "")
	require.Len(t, p.Config().Skills, 2, "the plugin loads while the environment leaves it on")
	t.Setenv(plugin.EnvPlugins, "off")
	require.NoError(t, p.LoadConfig())
	require.Len(t, p.Config().Skills, 1)
	require.Empty(t, p.Config().PluginHooks)
}

func TestLoadConfigSkillPathEnvLeadsThePluginCatalog(t *testing.T) {
	p, root := pluginProject(t, "skill_path: /from/file\n")
	t.Setenv("COZYPHI_SKILL_PATH", "/from/env")
	require.NoError(t, p.LoadConfig())
	cfg := p.Config()
	require.Len(t, cfg.Skills, 2)
	require.Equal(t, "/from/env", cfg.Skills[0].Dir)
	require.Equal(t, filepath.Join(root, "skills"), cfg.Skills[1].Dir)
}

func TestLoadConfigExpandsHomeInPluginPaths(t *testing.T) {
	p, _ := pluginProject(t, "plugins:\n  claude_dir: ~/elsewhere\n  paths:\n    - ~/src/mine\n    - relative\n")
	home := filepath.Dir(p.Global().Root())
	cfg := p.Config()
	require.Equal(t, filepath.Join(home, "elsewhere"), cfg.Plugins.ClaudeDir)
	require.Equal(t, []string{filepath.Join(home, "src", "mine"), "relative"}, cfg.Plugins.Paths)
	require.Equal(t, p.Global().PluginDataDir(), cfg.Plugins.LocalDataDir)
	require.Len(t, cfg.Skills, 1, "the claude_dir moved away, and both paths are skipped")
	require.Len(t, cfg.PluginWarnings(), 2)
}
