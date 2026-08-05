package project

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// discoverInTempHome runs Discover("") with HOME redirected to a temp dir so
// tests never touch the real ~/.phi.
func discoverInTempHome(t *testing.T) *Project {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	p, err := Discover("")
	require.NoError(t, err)
	return p
}

func TestDiscoverCreatesGlobalDirs(t *testing.T) {
	p := discoverInTempHome(t)

	for _, dir := range []string{
		p.Global().Root(),
		p.Global().BinDir(),
		p.Global().SkillsDir(),
		p.Global().SessionBase(),
	} {
		info, err := os.Stat(dir)
		assert.NoErrorf(t, err, "expected dir %q to exist", dir)
		if err == nil {
			assert.Truef(t, info.IsDir(), "expected %q to be a directory", dir)
		}
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
primary_model:
  name: deepseek-chat
  api_key: sk-test
`), 0o644))

	require.NoError(t, p.LoadConfig())
	cfg := p.Config()

	assert.Equal(t, "deepseek-chat", cfg.PrimaryModel.Name)
	assert.Equal(t, "sk-test", cfg.PrimaryModel.APIKey)
	assert.Equal(t, "https://api.openai.com/v1", cfg.PrimaryModel.BaseURL)
	assert.Equal(t, p.Global().SkillsDir(), cfg.SkillPath)
	// Model() carries the skill path for agent.NewEngine.
	assert.Equal(t, p.Global().SkillsDir(), cfg.Model().SkillPath)
}

func TestLoadConfigEnvOverrides(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
primary_model:
  name: file-model
  api_key: file-key
skill_path: /from/file
`), 0o644))

	t.Setenv("PHI_MODEL", "env-model")
	t.Setenv("PHI_API_KEY", "env-key")
	t.Setenv("PHI_BASE_URL", "https://env.example/v1")
	t.Setenv("PHI_SKILL_PATH", "/from/env")

	require.NoError(t, p.LoadConfig())
	cfg := p.Config()
	assert.Equal(t, "env-model", cfg.PrimaryModel.Name)
	assert.Equal(t, "env-key", cfg.PrimaryModel.APIKey)
	assert.Equal(t, "https://env.example/v1", cfg.PrimaryModel.BaseURL)
	assert.Equal(t, "/from/env", cfg.SkillPath)
}

func TestLoadConfigMissingAPIKey(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte("primary_model:\n  name: x\n"), 0o644))
	t.Setenv("PHI_API_KEY", "")

	err := p.LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_key")
}

func TestLoadConfigConfigFileMissing(t *testing.T) {
	// Env-only setup: no config file, all values from environment.
	p := discoverInTempHome(t)
	t.Setenv("PHI_MODEL", "env-model")
	t.Setenv("PHI_API_KEY", "env-key")

	require.NoError(t, p.LoadConfig())
	assert.Equal(t, "env-model", p.Config().PrimaryModel.Name)
	assert.Equal(t, "interactive", string(p.Config().Permissions.Mode))
}

func TestLoadConfigPermissions(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
primary_model:
  name: m
  api_key: k
permissions:
  mode: headless-strict
  ask_timeout_sec: 30
  workspace_only_writes: true
  bash:
    default: ask
    allow:
      - "^echo\b"
    deny:
      - "\bsudo\b"
  fetch:
    default: ask
    allowed_hosts:
      - "docs.github.com"
`), 0o644))

	require.NoError(t, p.LoadConfig())
	perm := p.Config().Permissions
	assert.Equal(t, "headless-strict", string(perm.Mode))
	assert.Equal(t, 30, perm.AskTimeoutSec)
	assert.Equal(t, []string{`^echo\b`}, perm.BashAllow)
	assert.Equal(t, []string{`\bsudo\b`}, perm.BashDeny)
	assert.Equal(t, []string{"docs.github.com"}, perm.FetchAllowedHosts)
}
