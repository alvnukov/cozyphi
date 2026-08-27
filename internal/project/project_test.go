package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/plangate"
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

func TestLoadConfigPlanDefaults(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: k
plan:
  defaults:
    types:
      - name: inspect
        tools: [read, lsp]
      - name: execute
        tools: [bash]
    additional_exemptions: [grep]
`), 0o600))

	require.NoError(t, p.LoadConfig())
	assert.Equal(t, plangate.Defaults{
		Types: []plangate.TypeDefaults{
			{Name: "inspect", Tools: []string{"read", "lsp"}},
			{Name: "execute", Tools: []string{"bash"}},
		},
		AdditionalExemptions: []string{"grep"},
	}, p.Config().PlanDefaults)
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

// The allow-all dialog persists permissions.dangerously_allow_all by editing
// config.yaml line-by-line; these tests pin that every shape the editor can
// meet stays loadable afterwards.
func TestSetDangerouslyAllowAllInlineEmptySection(t *testing.T) {
	// Regression: `cozyphi config` saves an untouched permissions section as
	// `permissions: {}`; appending an indented child under the inline
	// mapping produced YAML no later start could parse.
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\npermissions: {}\n")

	require.NoError(t, SetDangerouslyAllowAll(p.Global(), true))

	got, err := os.ReadFile(p.Global().ConfigFile())
	require.NoError(t, err)
	assert.Equal(t,
		"models:\n  - name: m\n    api_key: k\npermissions:\n  dangerously_allow_all: true\n",
		string(got))
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

	got, err := os.ReadFile(p.Global().ConfigFile())
	require.NoError(t, err)
	assert.Equal(t,
		"models:\n  - name: m\n    api_key: k\npermissions:\n  mode: ask\n  dangerously_allow_all: false\n",
		string(got))
	require.NoError(t, p.LoadConfig())
	assert.False(t, p.Config().Permissions.DangerouslyAllowAll)
}

func TestSetDangerouslyAllowAllAppendsMissingSection(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\n")

	require.NoError(t, SetDangerouslyAllowAll(p.Global(), true))

	got, err := os.ReadFile(p.Global().ConfigFile())
	require.NoError(t, err)
	assert.Equal(t,
		"models:\n  - name: m\n    api_key: k\n\npermissions:\n  dangerously_allow_all: true\n",
		string(got))
	require.NoError(t, p.LoadConfig())
	assert.True(t, p.Config().Permissions.DangerouslyAllowAll)
}

func TestSetDangerouslyAllowAllRefusesInlineMapping(t *testing.T) {
	// A non-empty inline section cannot be edited line-by-line without
	// losing its keys; the setter must fail closed and leave the file.
	p := discoverInTempHome(t)
	before := "models:\n  - name: m\n    api_key: k\npermissions: {mode: ask}\n"
	writeTestConfigBody(t, p, before)

	err := SetDangerouslyAllowAll(p.Global(), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "block")
	got, err := os.ReadFile(p.Global().ConfigFile())
	require.NoError(t, err)
	assert.Equal(t, before, string(got))
}
