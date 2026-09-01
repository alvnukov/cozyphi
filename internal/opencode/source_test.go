package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/provider"
)

func TestLoadModelsAndMCPServers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "opencode.json")
	require.NoError(t, os.WriteFile(authPath, []byte(`{
  "openai": {"type":"api", "key":"openai-secret"},
  "custom": {"type":"api", "key":"custom-secret"},
  "oauth-only": {"type":"oauth", "access":"ignored"},
  "wellknown": {"type":"wellknown", "key":"ignored", "token":"ignored"}
}`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`{
  // opencode accepts JSONC.
  "provider": {
    "custom": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {"baseURL": "{env:CUSTOM_URL}/"},
      "models": {"chat": {"limit": {"context": 32000, "output": 4096}}},
    },
  },
  "mcp": {
    "local": {"type":"local", "command":["node", "server.js"], "environment":{"TOKEN":"{env:TOKEN}"}, "timeout":2500},
    "remote": {"type":"remote", "url":"https://mcp.example", "headers":{"Authorization":"Bearer {env:TOKEN}"}, "oauth":false},
    "disabled": {"type":"local", "command":["false"], "enabled":false},
    "empty": {"type":"local", "command":[""]},
    "oauth": {"type":"remote", "url":"https://oauth.example", "oauth":{"clientId":"id"}},
  },
}`), 0o600))

	env := map[string]string{"CUSTOM_URL": "https://custom.example/v1", "TOKEN": "secret"}
	source, err := Load(Options{
		ConfigPath: configPath,
		AuthPath:   authPath,
		LookupEnv:  func(name string) string { return env[name] },
		Catalog: []provider.Info{{
			ID: "openai", BaseURL: "https://api.openai.com/v1", Protocol: llm.ProtocolOpenAI,
			Models: []provider.Model{{ID: "gpt-test", ContextWindow: 128000, MaxOutputTokens: 8192}},
		}},
	})
	require.NoError(t, err)

	models := source.Models()
	require.Len(t, models, 2)
	assert.Equal(t, "opencode/custom/chat", models[0].Name)
	assert.Equal(t, "chat", models[0].APIName)
	assert.Equal(t, "custom-secret", models[0].APIKey)
	assert.Equal(t, "https://custom.example/v1", models[0].BaseURL)
	assert.Equal(t, 32000, models[0].ContextWindow)
	assert.Equal(t, "opencode/openai/gpt-test", models[1].Name)
	assert.Equal(t, "openai-secret", models[1].APIKey)

	servers := source.MCPServers()
	require.Len(t, servers, 2)
	assert.Equal(t, []string{"node"}, servers["local"].Command)
	assert.Equal(t, []string{"server.js"}, servers["local"].Args)
	assert.Equal(t, "secret", servers["local"].Env["TOKEN"])
	assert.Equal(t, "2.5s", servers["local"].Timeout)
	assert.Equal(t, "http", servers["remote"].Transport)
	assert.Equal(t, "Bearer secret", servers["remote"].Headers["Authorization"])
	assert.NotContains(t, servers, "disabled")
	assert.NotContains(t, servers, "empty")
	assert.NotContains(t, servers, "oauth")
}

func TestLoadMissingFilesIsEmpty(t *testing.T) {
	t.Parallel()
	source, err := Load(Options{
		ConfigPath: filepath.Join(t.TempDir(), "missing-config.json"),
		AuthPath:   filepath.Join(t.TempDir(), "missing-auth.json"),
	})
	require.NoError(t, err)
	assert.Empty(t, source.Models())
	assert.Empty(t, source.MCPServers())
}

func TestPathsHonorOpenCodeAndXDGOverrides(t *testing.T) {
	t.Setenv("OPENCODE_CONFIG", "")
	t.Setenv("OPENCODE_CONFIG_DIR", "/custom/config")
	t.Setenv("XDG_CONFIG_HOME", "/xdg/config")
	t.Setenv("XDG_DATA_HOME", "/xdg/data")

	configPath, authPath, err := paths(Options{})
	require.NoError(t, err)
	assert.Equal(t, "/custom/config/opencode.json", configPath)
	assert.Equal(t, "/xdg/data/opencode/auth.json", authPath)

	t.Setenv("OPENCODE_CONFIG", "/exact/opencode.jsonc")
	configPath, _, err = paths(Options{})
	require.NoError(t, err)
	assert.Equal(t, "/exact/opencode.jsonc", configPath)
}

func TestMCPServersReturnsDetachedCopy(t *testing.T) {
	t.Parallel()
	source := &Source{servers: resolveServers(map[string]json.RawMessage{
		"local": json.RawMessage(`{"type":"local","command":["one","two"],"environment":{"A":"B"}}`),
	})}
	first := source.MCPServers()
	first["local"].Command[0] = "changed"
	first["local"].Env["A"] = "changed"
	second := source.MCPServers()
	assert.Equal(t, "one", second["local"].Command[0])
	assert.Equal(t, "B", second["local"].Env["A"])
}
