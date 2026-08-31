package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/notify"
	"github.com/alvnukov/cozyphi/internal/permission"
)

// discoverInTempHome runs Discover("") with HOME redirected to a temp dir so
// tests never touch the real ~/.cozyphi.
func discoverInTempHome(t *testing.T) *Project {
	t.Helper()
	home := t.TempDir()
	// os.UserHomeDir uses HOME on Unix and USERPROFILE on Windows.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	p, err := Discover("")
	require.NoError(t, err)
	return p
}

func TestDiscoverSharesClaudeMemoryAcrossGitSubdirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repo := t.TempDir()
	if err := exec.CommandContext(t.Context(), "git", "init", "--quiet", repo).Run(); err != nil {
		t.Skipf("git is unavailable: %v", err)
	}
	subdir := filepath.Join(repo, "internal", "memory")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	rootProject, err := Discover(repo)
	require.NoError(t, err)
	subdirProject, err := Discover(subdir)
	require.NoError(t, err)
	assert.Equal(t, rootProject.MemoryDir(), subdirProject.MemoryDir())
}

func TestDiscoverCreatesGlobalDirs(t *testing.T) {
	p := discoverInTempHome(t)

	for _, dir := range []string{
		p.Global().Root(),
		p.Global().BinDir(),
		p.Global().SkillsDir(),
		p.Global().HooksDir(),
		p.Global().SessionBase(),
		p.Global().JobsDir(),
	} {
		info, err := os.Stat(dir)
		assert.NoErrorf(t, err, "expected dir %q to exist", dir)
		if err == nil {
			assert.Truef(t, info.IsDir(), "expected %q to be a directory", dir)
		}
	}
}

func TestHooksDirPath(t *testing.T) {
	p := discoverInTempHome(t)
	assert.Equal(t, filepath.Join(p.Global().Root(), "hooks"), p.Global().HooksDir())
}

func TestLookBinPrefersBinDir(t *testing.T) {
	p := discoverInTempHome(t)
	fake := filepath.Join(p.Global().BinDir(), "rg")
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755))

	got, err := p.Global().LookBin("rg")
	require.NoError(t, err)
	assert.Equal(t, fake, got)
}

func TestLookBinFallsBackToPATH(t *testing.T) {
	p := discoverInTempHome(t)
	got, err := p.Global().LookBin("sh")
	if err != nil {
		t.Skip(err.Error())
	}
	require.NotEmpty(t, got)
	assert.NotContains(t, got, p.Global().BinDir())
}

func TestProjectDirs(t *testing.T) {
	p := discoverInTempHome(t)
	assert.Equal(t, filepath.Join(p.Root(), ".cozyphi", "hooks"), p.HooksDir())
	assert.Equal(t, filepath.Join(p.Root(), ".cozyphi", "mcp.json"), p.MCPConfigFile())
}

func TestLoadConfigDefaults(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: deepseek-chat
    api_key: sk-test
`), 0o644))

	require.NoError(t, p.LoadConfig())
	cfg := p.Config()

	assert.Equal(t, "deepseek-chat", cfg.Model().Name)
	assert.Equal(t, "sk-test", cfg.Model().APIKey)
	assert.Equal(t, "https://api.openai.com/v1", cfg.Model().BaseURL)
	assert.Equal(t, llm.ProtocolOpenAI, cfg.Model().Protocol)
	assert.Equal(t, p.Global().SkillsDir(), cfg.SkillPath)
	// Model() carries the skill path for agent.NewEngine.
	assert.Equal(t, p.Global().SkillsDir(), cfg.Model().SkillPath)
}

func TestLoadConfigLeavesPlanDefaultsToHarnessSettings(t *testing.T) {
	// plan.defaults has one owner (internal/harnesssettings); a config
	// carrying the section must still load cleanly here, it just never
	// surfaces on project.Config.
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: k
plan:
  defaults:
    types:
      - name: inspect
        tools: [read]
`), 0o600))

	require.NoError(t, p.LoadConfig())
	assert.Equal(t, "m", p.Config().Model().Name)
}

func TestLoadConfigResolvesLegacyAnthropicProtocolAtConfigBoundary(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: gateway-model
    api_key: sk-test
    base_url: https://api.anthropic.com
`), 0o600))

	require.NoError(t, p.LoadConfig())
	assert.Equal(t, llm.ProtocolAnthropic, p.Config().Model().Protocol)
}

func TestLoadConfigEnvOverrides(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: file-model
    api_key: file-key
skill_path: /from/file
`), 0o644))

	t.Setenv("COZYPHI_MODEL", "env-model")
	t.Setenv("COZYPHI_API_KEY", "env-key")
	t.Setenv("COZYPHI_BASE_URL", "https://env.example/v1")
	t.Setenv("COZYPHI_SKILL_PATH", "/from/env")

	require.NoError(t, p.LoadConfig())
	cfg := p.Config()
	assert.Equal(t, "env-model", cfg.Model().Name)
	assert.Equal(t, "env-key", cfg.Model().APIKey)
	assert.Equal(t, "https://env.example/v1", cfg.Model().BaseURL)
	assert.Equal(t, "/from/env", cfg.SkillPath)
	assert.True(t, cfg.ModelEnvOverride(), "COZYPHI_MODEL must outrank remembered UI state")
}

func TestLoadConfigNoModelEnvOverride(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(
		t,
		os.WriteFile(p.Global().ConfigFile(), []byte("models:\n  - name: file-model\n    api_key: file-key\n"), 0o644),
	)
	t.Setenv("COZYPHI_MODEL", "")

	require.NoError(t, p.LoadConfig())
	assert.False(t, p.Config().ModelEnvOverride())
}

func TestLoadConfigMissingAPIKey(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte("models:\n  - name: x\n"), 0o644))
	t.Setenv("COZYPHI_API_KEY", "")

	err := p.LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_key")
}

func TestLoadConfigConfigFileMissing(t *testing.T) {
	// Env-only setup: no config file, all values from environment.
	p := discoverInTempHome(t)
	t.Setenv("COZYPHI_MODEL", "env-model")
	t.Setenv("COZYPHI_API_KEY", "env-key")

	require.NoError(t, p.LoadConfig())
	assert.Equal(t, "env-model", p.Config().Model().Name)
	assert.Equal(t, "interactive", string(p.Config().Permissions.Mode))
}

func TestLoadConfigMaxOutputTokens(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: reasoner
    api_key: k
    max_output_tokens: 32768
    default: true
  - name: plain
    api_key: k
`), 0o644))

	require.NoError(t, p.LoadConfig())
	assert.Equal(t, 32768, p.Config().Model().MaxOutputTokens)
	plain, ok := p.Config().FindModel("plain")
	require.True(t, ok)
	assert.Equal(t, 0, plain.MaxOutputTokens)
}

func TestLoadConfigReasoningEffort(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: codex-high
    api_name: gpt-5.5
    protocol: openai-responses
    api_key: k
    reasoning_effort: high
`), 0o644))

	require.NoError(t, p.LoadConfig())
	cfg := p.Config().Model()
	assert.Equal(t, llm.ProtocolOpenAIResponses, cfg.Protocol)
	assert.Equal(t, "gpt-5.5", cfg.RequestModel())
	assert.Equal(t, llm.ReasoningEffortHigh, cfg.ReasoningEffort)
}

func TestLoadConfigRejectsUnknownReasoningEffort(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: codex-weird
    protocol: openai-responses
    api_key: k
    reasoning_effort: banana
`), 0o644))

	err := p.LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported reasoning_effort")
}

func TestLoadConfigPermissions(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: k
permissions:
  mode: headless-strict
  ask_timeout_sec: 30
  workspace_only_writes: true
  bash:
    default: ask
    allow:
      - '^echo\b'
    deny:
      - '\bsudo\b'
`), 0o644))

	require.NoError(t, p.LoadConfig())
	perm := p.Config().Permissions
	assert.Equal(t, "headless-strict", string(perm.Mode))
	assert.Equal(t, 30, perm.AskTimeoutSec)
	assert.Equal(t, []string{`^echo\b`}, perm.BashAllow)
	assert.Equal(t, []string{`\bsudo\b`}, perm.BashDeny)
	assert.True(t, p.Config().Agents.Enabled) // default on when agents: absent
}

func TestLoadConfigAgentsEnabled(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: k
agents:
  enabled: true
`), 0o644))

	require.NoError(t, p.LoadConfig())
	assert.True(t, p.Config().Agents.Enabled)
}

func TestLoadConfigAgentsDisabled(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: k
agents:
  enabled: false
`), 0o644))

	require.NoError(t, p.LoadConfig())
	assert.False(t, p.Config().Agents.Enabled)
}

func TestLoadConfigAgentsModels(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: k
  - name: cheap
    api_key: k
  - name: strong
    api_key: k
agents:
  models:
    explore: cheap
    worker: strong
`), 0o644))

	require.NoError(t, p.LoadConfig())
	assert.Equal(t, map[string]string{
		"explore": "cheap",
		"worker":  "strong",
	}, p.Config().Agents.Models)
}

func TestLoadConfigAgentsModelsUnknownRoleFails(t *testing.T) {
	// An unknown role key can never take effect, so it is structural: load
	// fails loudly instead of silently ignoring the entry.
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: k
agents:
  models:
    explorer: m
`), 0o644))

	err := p.LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "explorer")
}

func TestLoadConfigAgentsModelsUnknownNameLoads(t *testing.T) {
	// A model NAME the config cannot resolve is data pointing outside the
	// file; per the agreed degradation it must not block startup.
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: k
agents:
  models:
    explore: no-such-model
`), 0o644))

	require.NoError(t, p.LoadConfig())
	assert.Equal(t, map[string]string{"explore": "no-such-model"}, p.Config().Agents.Models)
}

func TestAgentModelFor(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: k
  - name: cheap
    api_key: k
agents:
  models:
    explore: cheap
    worker: no-such-model
`), 0o644))
	require.NoError(t, p.LoadConfig())
	cfg := p.Config()

	mc, ok := cfg.AgentModelFor(job.RoleExplore)
	require.True(t, ok)
	assert.Equal(t, "cheap", mc.Name)

	_, ok = cfg.AgentModelFor(job.RoleWorker)
	assert.False(t, ok, "a name that no longer resolves must inherit, not error")

	_, ok = cfg.AgentModelFor(job.RoleReview)
	assert.False(t, ok, "an unset role inherits")

	assert.Equal(t, []string{"worker=no-such-model"}, cfg.StaleAgentModels(),
		"stale pins are reported for warning, live and unset are not")
	var nilCfg *Config
	assert.Empty(t, nilCfg.StaleAgentModels(), "nil config has nothing to warn about")
}

func TestLoadConfigScalarOrInlineListForms(t *testing.T) {
	// The old line scanner only understood block lists (and treated an inline
	// sequence as one literal string); real YAML handles scalar and flow forms.
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: k
permissions:
  bash:
    allow: "go test ./..."
    deny: ['rm -rf', 'sudo']
`), 0o644))

	require.NoError(t, p.LoadConfig())
	perm := p.Config().Permissions
	assert.Equal(t, []string{"go test ./..."}, perm.BashAllow)
	assert.Equal(t, []string{"rm -rf", "sudo"}, perm.BashDeny)
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	// A malformed config must fail loudly instead of silently degrading to
	// defaults (the old line scanner ignored malformed lines).
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(
		"models:\n  - name: m\n    api_key: k\npermissions: [unclosed\n",
	), 0o644))

	require.Error(t, p.LoadConfig())
}

func TestLoadConfigModelsFlat(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: p
    api_key: pk
    base_url: https://primary.example/v1
    context_window: 1000
    default: true
  - name: a1
    api_key: ak1
    base_url: https://a1.example/v1
  - name: a2
    api_key: ak2
    base_url: https://a2.example/v1
    context_window: 2000
`), 0o644))

	require.NoError(t, p.LoadConfig())
	cfg := p.Config()
	require.Len(t, cfg.Models, 3)
	assert.Equal(t, "p", cfg.Models[0].Name)
	assert.Equal(t, "ak1", cfg.Models[1].APIKey)
	assert.Equal(t, 2000, cfg.Models[2].ContextWindow)
	assert.Equal(t, "p", cfg.DefaultModel)

	// Model() returns the entry marked default, with the skill path applied.
	m := cfg.Model()
	assert.Equal(t, "p", m.Name)
	assert.Equal(t, p.Global().SkillsDir(), m.SkillPath)

	// Models() lists every entry with the skill path applied.
	models := cfg.AllModels()
	require.Len(t, models, 3)
	for _, mm := range models {
		assert.Equal(t, p.Global().SkillsDir(), mm.SkillPath)
	}

	// FindModel returns the full per-model connection config.
	a2, ok := cfg.FindModel("a2")
	require.True(t, ok)
	assert.Equal(t, "https://a2.example/v1", a2.BaseURL)
	_, ok = cfg.FindModel("nope")
	assert.False(t, ok)
}

func TestLoadConfigDefaultFallsBackToFirst(t *testing.T) {
	// No entry marked default → the first entry wins.
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: first
    api_key: k1
  - name: second
    api_key: k2
`), 0o644))

	require.NoError(t, p.LoadConfig())
	cfg := p.Config()
	assert.Empty(t, cfg.DefaultModel)
	assert.Equal(t, "first", cfg.Model().Name)
}

func TestWriteOwnerOnlyTightensExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))

	require.NoError(t, WriteOwnerOnly(path, []byte("new")))

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(written))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "a 0644 predecessor must be tightened")
}

// writeTestConfigBody installs body as the project's config.yaml.
func writeTestConfigBody(t *testing.T, p *Project, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(body), 0o644))
}

// The allow-all dialog persists permissions.dangerously_allow_all through the
// config.yaml single owner; these tests pin that every shape the file can meet
// ends up with the key set and the file reloadable, with unrelated content
// intact.
func TestSetDangerouslyAllowAllInlineEmptySection(t *testing.T) {
	// Regression shape: `cozyphi config` saves an untouched permissions
	// section as `permissions: {}`; the key must land inside it as YAML the
	// next start still parses.
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\npermissions: {}\n")

	require.NoError(t, SetDangerouslyAllowAll(p.Global(), true))

	require.NoError(t, p.LoadConfig())
	assert.True(t, p.Config().Permissions.DangerouslyAllowAll)
}

func TestSetDangerouslyAllowAllReplacesExistingKey(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(
		t,
		p,
		"models:\n  - name: m\n    api_key: k\npermissions:\n  mode: ask\n  dangerously_allow_all: true\n",
	)

	require.NoError(t, SetDangerouslyAllowAll(p.Global(), false))

	require.NoError(t, p.LoadConfig())
	assert.False(t, p.Config().Permissions.DangerouslyAllowAll)
	assert.Equal(t, permission.Mode("ask"), p.Config().Permissions.Mode, "sibling keys survive the edit")
}

func TestSetDangerouslyAllowAllAppendsMissingSection(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\n")

	require.NoError(t, SetDangerouslyAllowAll(p.Global(), true))

	require.NoError(t, p.LoadConfig())
	assert.True(t, p.Config().Permissions.DangerouslyAllowAll)
}

func TestSetDangerouslyAllowAllEditsInlineMapping(t *testing.T) {
	// A non-empty inline section used to be refused: line-by-line editing
	// could not touch it without losing keys. The single owner edits the node
	// tree, so the mapping gains the key in place.
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\npermissions: {mode: ask}\n")

	require.NoError(t, SetDangerouslyAllowAll(p.Global(), true))

	require.NoError(t, p.LoadConfig())
	assert.True(t, p.Config().Permissions.DangerouslyAllowAll)
	assert.Equal(t, permission.Mode("ask"), p.Config().Permissions.Mode)
}

func TestSetDangerouslyAllowAllFailsClosedOnUnparseableConfig(t *testing.T) {
	p := discoverInTempHome(t)
	before := "models: [oops\n"
	writeTestConfigBody(t, p, before)

	require.Error(t, SetDangerouslyAllowAll(p.Global(), true))

	got, err := os.ReadFile(p.Global().ConfigFile())
	require.NoError(t, err)
	assert.Equal(t, before, string(got), "a config that cannot be parsed is never rewritten")
}

func TestLoadConfigNotificationsMode(t *testing.T) {
	t.Run("absent section defaults to unfocused", func(t *testing.T) {
		p := discoverInTempHome(t)
		writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\n")

		require.NoError(t, p.LoadConfig())
		assert.Equal(t, notify.ModeUnfocused, p.Config().Notifications.Mode)
	})
	t.Run("empty section defaults to unfocused", func(t *testing.T) {
		p := discoverInTempHome(t)
		writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\nnotifications: {}\n")

		require.NoError(t, p.LoadConfig())
		assert.Equal(t, notify.ModeUnfocused, p.Config().Notifications.Mode)
	})
	t.Run("explicit mode is honored", func(t *testing.T) {
		p := discoverInTempHome(t)
		writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\nnotifications:\n  mode: always\n")

		require.NoError(t, p.LoadConfig())
		assert.Equal(t, notify.ModeAlways, p.Config().Notifications.Mode,
			"an explicit mode must win over the default")
	})
	t.Run("invalid mode fails config load", func(t *testing.T) {
		p := discoverInTempHome(t)
		writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\nnotifications:\n  mode: sometimes\n")

		err := p.LoadConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "notifications.mode")
	})
}
