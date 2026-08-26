package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeConfigFile writes one config file with an exact mode so security
// checks can be exercised deterministically.
func writeConfigFile(t *testing.T, dir, name, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
	require.NoError(t, os.Chmod(path, mode))
	return path
}

func TestLoadConfigMissingFileUsesDefaults(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "lsp.json"))
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, []string{"gopls"}, cfg.Gopls.Command)
	assert.Empty(t, cfg.Gopls.Env)
	assert.Empty(t, cfg.Gopls.InitializationOptions)
	assert.Empty(t, cfg.Gopls.Settings)
}

func TestLoadConfigFullValidFile(t *testing.T) {
	path := writeConfigFile(t, t.TempDir(), "lsp.json", `{
		"enabled": true,
		"gopls": {
			"command": ["/opt/gopls/bin/gopls", "-rpc.trace"],
			"env": {"B": "2", "A": "1"},
			"initialization_options": {"buildFlags": ["-tags=e2e"]},
			"settings": {"gopls": {"verboseOutput": true}}
		}
	}`, 0o600)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, []string{"/opt/gopls/bin/gopls", "-rpc.trace"}, cfg.Gopls.Command)
	assert.Equal(t, []string{"A=1", "B=2"}, cfg.Gopls.Env)
	assert.Equal(t, map[string]any{"buildFlags": []any{"-tags=e2e"}}, cfg.Gopls.InitializationOptions)
	assert.Equal(t, map[string]any{"gopls": map[string]any{"verboseOutput": true}}, cfg.Gopls.Settings)
}

func TestLoadConfigDisabled(t *testing.T) {
	path := writeConfigFile(t, t.TempDir(), "lsp.json", `{"enabled": false}`, 0o600)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.False(t, cfg.Enabled)
	// Missing gopls while still parseable keeps the default command so the
	// frozen Config is always complete for callers.
	assert.Equal(t, []string{"gopls"}, cfg.Gopls.Command)
}

func TestLoadConfigMalformedFailsClosedWithoutSecrets(t *testing.T) {
	path := writeConfigFile(t, t.TempDir(), "lsp.json",
		`{"gopls":{"env":{"TOKEN":"SECRETV"}},`, 0o600)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRETV")
	assert.Contains(t, err.Error(), "lsp")
}

func TestLoadConfigRejectsInsecureFiles(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		dir := t.TempDir()
		real := writeConfigFile(t, dir, "real.json", `{"enabled":true}`, 0o600)
		link := filepath.Join(dir, "lsp.json")
		require.NoError(t, os.Symlink(real, link))
		_, err := LoadConfig(link)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "symlink")
	})
	t.Run("group-writable", func(t *testing.T) {
		path := writeConfigFile(t, t.TempDir(), "lsp.json", `{"enabled":true}`, 0o660)
		_, err := LoadConfig(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "writable")
	})
	t.Run("world-writable", func(t *testing.T) {
		path := writeConfigFile(t, t.TempDir(), "lsp.json", `{"enabled":true}`, 0o666)
		_, err := LoadConfig(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "writable")
	})
	t.Run("directory", func(t *testing.T) {
		dir := t.TempDir()
		_, err := LoadConfig(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "regular")
	})
}

func TestLoadConfigRejectsOversized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lsp.json")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat(" ", MaxConfigBytes+1)), 0o600))
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestLoadConfigUnknownKeysFailClosed(t *testing.T) {
	for name, content := range map[string]string{
		"top-level":    `{"enabled":true,"rust-analyzer":{"command":["ra"]}}`,
		"server-field": `{"gopls":{"logfile":"/tmp/x"}}`,
		"wrong-type":   `{"gopls":{"command":"gopls"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := writeConfigFile(t, t.TempDir(), "lsp.json", content, 0o600)
			_, err := LoadConfig(path)
			require.Error(t, err)
		})
	}
}

func TestLoadConfigCommandValidation(t *testing.T) {
	for name, first := range map[string]string{
		"dot-relative":    "./gopls",
		"parent-relative": "../gopls",
		"subdir":          "dir/gopls",
		"backslash":       "\\\\gopls",
		"volume-relative": "C:gopls",
	} {
		t.Run("reject "+name, func(t *testing.T) {
			path := writeConfigFile(t, t.TempDir(), "lsp.json",
				`{"gopls":{"command":["`+first+`"]}}`, 0o600)
			_, err := LoadConfig(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "command")
		})
	}
	for name, cmd := range map[string][]string{
		"bare":      {"gopls"},
		"absolute":  {"/usr/local/bin/gopls"},
		"with-args": {"/usr/local/bin/gopls", "-rpc.trace"},
	} {
		t.Run("accept "+name, func(t *testing.T) {
			path := writeConfigFile(t, t.TempDir(), "lsp.json",
				`{"gopls":{"command":["`+strings.Join(cmd, `","`)+`"]}}`, 0o600)
			cfg, err := LoadConfig(path)
			require.NoError(t, err)
			assert.Equal(t, cmd, cfg.Gopls.Command)
		})
	}
	t.Run("reject empty argv element", func(t *testing.T) {
		path := writeConfigFile(t, t.TempDir(), "lsp.json", `{"gopls":{"command":["gopls",""]}}`, 0o600)
		_, err := LoadConfig(path)
		require.Error(t, err)
	})
	t.Run("reject empty command", func(t *testing.T) {
		path := writeConfigFile(t, t.TempDir(), "lsp.json", `{"gopls":{"command":[]}}`, 0o600)
		_, err := LoadConfig(path)
		require.Error(t, err)
	})
}

func TestLoadConfigEnvValidation(t *testing.T) {
	for name, content := range map[string]string{
		"empty-key":  `{"gopls":{"env":{"":"v"}}}`,
		"key-with-e": `{"gopls":{"env":{"K=V":"v"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := writeConfigFile(t, t.TempDir(), "lsp.json", content, 0o600)
			_, err := LoadConfig(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "env")
		})
	}
}

// TestLoadConfigIgnoresProjectLocal pins the contract that only the passed
// global path is ever read: a poisoned project-local .cozyphi/lsp.json must
// have no effect.
func TestLoadConfigIgnoresProjectLocal(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, ".cozyphi"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(work, ".cozyphi", "lsp.json"), []byte("garbage"), 0o600))
	t.Chdir(work)
	global := filepath.Join(t.TempDir(), "lsp.json")
	cfg, err := LoadConfig(global)
	require.NoError(t, err)
	assert.Equal(t, DefaultConfig(), cfg)
}
