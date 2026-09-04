package opencode

import (
	"encoding/json"
	"fmt"
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

func TestKeySourceResolveKey(t *testing.T) {
	t.Parallel()
	keys := keySource{
		lookupEnv: func(name string) string {
			if name == "SECRET_ENV" {
				return "env-secret"
			}
			return ""
		},
		readFile: func(path string) ([]byte, error) {
			switch path {
			case "/keys/note":
				return []byte("file-secret\n"), nil
			case "/keys/blank":
				return []byte("   \n"), nil
			default:
				return nil, os.ErrNotExist
			}
		},
	}
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "plain string", raw: `"plain-secret"`, want: "plain-secret"},
		{name: "empty string", raw: `""`, want: ""},
		{name: "null", raw: `null`, want: ""},
		{name: "absent", raw: ``, want: ""},
		{name: "literal env token", raw: `"{env:SECRET_ENV}"`, want: "env-secret"},
		{name: "literal env token for unset name", raw: `"{env:UNSET_NAME}"`, want: ""},
		{name: "literal file token", raw: `"{file:/keys/note}"`, want: "file-secret"},
		{name: "literal file token with blank file", raw: `"{file:/keys/blank}"`, want: ""},
		{name: "literal file token with missing file", raw: `"{file:/keys/missing}"`, want: ""},
		{name: "embedded file token stays a plain key", raw: `"key-{file:/keys/note}"`, want: "key-{file:/keys/note}"},
		{name: "object env form", raw: `{"env":"SECRET_ENV"}`, want: "env-secret"},
		{name: "object env form for unset name", raw: `{"env":"UNSET_NAME"}`, want: ""},
		{name: "object file form", raw: `{"file":"/keys/note"}`, want: "file-secret"},
		{name: "object file form with missing file", raw: `{"file":"/keys/missing"}`, want: ""},
		{name: "object without env or file", raw: `{"apiKey":"sk-1"}`, want: ""},
		{name: "number is not a key", raw: `123`, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, keys.resolveKey(json.RawMessage(test.raw)))
		})
	}
}

func TestResolveModelsUnionOfAuthAndConfig(t *testing.T) {
	t.Parallel()
	catalog := []provider.Info{
		{
			ID: "anthropic", BaseURL: "https://api.anthropic.com", Protocol: llm.ProtocolAnthropic,
			Models: []provider.Model{
				{ID: "claude-3", ContextWindow: 200000, MaxOutputTokens: 4096},
				{ID: "claude-4", ContextWindow: 1000000, MaxOutputTokens: 64000},
			},
		},
		{
			ID: "zline", BaseURL: "https://zline.example/v1", Protocol: llm.ProtocolOpenAI,
			Models: []provider.Model{{ID: "z-1", ContextWindow: 64000, MaxOutputTokens: 1024}},
		},
	}
	keys := keySource{
		lookupEnv: func(name string) string {
			if name == "BEE_KEY" {
				return "env-secret"
			}
			return ""
		},
		readFile: func(string) ([]byte, error) { return nil, os.ErrNotExist },
	}
	beeline := func(apiKey string) providerConfig {
		return providerConfig{
			NPM:     "@ai-sdk/openai",
			Options: providerOptions{BaseURL: "https://beeline.example/v1/", APIKey: json.RawMessage(apiKey)},
			Models:  map[string]providerModel{"sonnet": {Limit: modelLimit{Context: 128000, Output: 4096}}},
		}
	}
	apiAuth := func(key string) map[string]authEntry {
		return map[string]authEntry{"beeline": {Type: "api", Key: key}}
	}
	tests := []struct {
		name       string
		auth       map[string]authEntry
		configured map[string]providerConfig
		disabled   map[string]bool
		want       []llm.ModelConfig
	}{
		{
			name: "config only provider imports with plain apiKey",
			configured: map[string]providerConfig{
				"beeline": beeline(`"config-secret"`),
			},
			want: []llm.ModelConfig{{
				Name: "opencode/beeline/sonnet", APIName: "sonnet", ProviderID: "beeline",
				Protocol: llm.ProtocolOpenAI, APIKey: "config-secret", BaseURL: "https://beeline.example/v1",
				ContextWindow: 128000, MaxOutputTokens: 4096,
			}},
		},
		{
			name: "config only provider resolves leftover env token apiKey",
			configured: map[string]providerConfig{
				"beeline": beeline(`"{env:BEE_KEY}"`),
			},
			want: []llm.ModelConfig{{
				Name: "opencode/beeline/sonnet", APIName: "sonnet", ProviderID: "beeline",
				Protocol: llm.ProtocolOpenAI, APIKey: "env-secret", BaseURL: "https://beeline.example/v1",
				ContextWindow: 128000, MaxOutputTokens: 4096,
			}},
		},
		{
			name: "config only keyless provider imports with empty key",
			configured: map[string]providerConfig{
				"gateway": {
					NPM:     "@ai-sdk/openai-compatible",
					Options: providerOptions{BaseURL: "http://localhost:8080/v1"},
					Models:  map[string]providerModel{"local-model": {Limit: modelLimit{Context: 8192}}},
				},
			},
			want: []llm.ModelConfig{{
				Name: "opencode/gateway/local-model", APIName: "local-model", ProviderID: "gateway",
				Protocol: llm.ProtocolOpenAI, BaseURL: "http://localhost:8080/v1", ContextWindow: 8192,
			}},
		},
		{
			name: "config only provider without baseURL is skipped",
			configured: map[string]providerConfig{
				"beeline": {
					NPM:     "@ai-sdk/openai",
					Options: providerOptions{APIKey: json.RawMessage(`"k"`)},
					Models:  map[string]providerModel{"sonnet": {}},
				},
			},
		},
		{
			name: "config only provider without models is skipped",
			configured: map[string]providerConfig{
				"beeline": {
					NPM:     "@ai-sdk/openai",
					Options: providerOptions{BaseURL: "https://beeline.example/v1", APIKey: json.RawMessage(`"k"`)},
				},
			},
		},
		{
			name: "config only provider with unsupported adapter is skipped",
			configured: map[string]providerConfig{
				"beeline": {
					NPM:     "@ai-sdk/google",
					Options: providerOptions{BaseURL: "https://beeline.example/v1", APIKey: json.RawMessage(`"k"`)},
					Models:  map[string]providerModel{"sonnet": {}},
				},
			},
		},
		{
			name: "catalog models overlay with config models",
			auth: map[string]authEntry{"anthropic": {Type: "api", Key: "auth-secret"}},
			configured: map[string]providerConfig{
				"anthropic": {Models: map[string]providerModel{
					// Overrides claude-4's context; its output falls back to the catalog.
					"claude-4": {Limit: modelLimit{Context: 500000}},
					// New model via the id field.
					"alias": {ID: "real-id"},
					// New model via the map key fallback.
					"turbo": {},
				}},
			},
			want: []llm.ModelConfig{
				{
					Name: "opencode/anthropic/claude-3", APIName: "claude-3", ProviderID: "anthropic",
					Protocol: llm.ProtocolAnthropic, APIKey: "auth-secret", BaseURL: "https://api.anthropic.com",
					ContextWindow: 200000, MaxOutputTokens: 4096,
				},
				{
					Name: "opencode/anthropic/claude-4", APIName: "claude-4", ProviderID: "anthropic",
					Protocol: llm.ProtocolAnthropic, APIKey: "auth-secret", BaseURL: "https://api.anthropic.com",
					ContextWindow: 500000, MaxOutputTokens: 64000,
				},
				{
					Name: "opencode/anthropic/real-id", APIName: "real-id", ProviderID: "anthropic",
					Protocol: llm.ProtocolAnthropic, APIKey: "auth-secret", BaseURL: "https://api.anthropic.com",
				},
				{
					Name: "opencode/anthropic/turbo", APIName: "turbo", ProviderID: "anthropic",
					Protocol: llm.ProtocolAnthropic, APIKey: "auth-secret", BaseURL: "https://api.anthropic.com",
				},
			},
		},
		{
			name: "catalog provider baseURL override keeps catalog models",
			auth: map[string]authEntry{"anthropic": {Type: "api", Key: "auth-secret"}},
			configured: map[string]providerConfig{
				"anthropic": {Options: providerOptions{BaseURL: "https://proxy.example/"}},
			},
			want: []llm.ModelConfig{
				{
					Name: "opencode/anthropic/claude-3", APIName: "claude-3", ProviderID: "anthropic",
					Protocol: llm.ProtocolAnthropic, APIKey: "auth-secret", BaseURL: "https://proxy.example",
					ContextWindow: 200000, MaxOutputTokens: 4096,
				},
				{
					Name: "opencode/anthropic/claude-4", APIName: "claude-4", ProviderID: "anthropic",
					Protocol: llm.ProtocolAnthropic, APIKey: "auth-secret", BaseURL: "https://proxy.example",
					ContextWindow: 1000000, MaxOutputTokens: 64000,
				},
			},
		},
		{
			name: "auth entry for unknown provider is skipped",
			auth: map[string]authEntry{"mystery": {Type: "api", Key: "mystery-secret"}},
		},
		{
			name: "disabled provider is skipped",
			auth: map[string]authEntry{
				"anthropic": {Type: "api", Key: "auth-secret"},
				"zline":     {Type: "api", Key: "zline-secret"},
			},
			disabled: map[string]bool{"anthropic": true},
			want: []llm.ModelConfig{{
				Name: "opencode/zline/z-1", APIName: "z-1", ProviderID: "zline",
				Protocol: llm.ProtocolOpenAI, APIKey: "zline-secret", BaseURL: "https://zline.example/v1",
				ContextWindow: 64000, MaxOutputTokens: 1024,
			}},
		},
		{
			name:       "keyless catalog provider declared in config imports with empty key",
			configured: map[string]providerConfig{"zline": {}},
			want: []llm.ModelConfig{{
				Name: "opencode/zline/z-1", APIName: "z-1", ProviderID: "zline",
				Protocol: llm.ProtocolOpenAI, BaseURL: "https://zline.example/v1",
				ContextWindow: 64000, MaxOutputTokens: 1024,
			}},
		},
		{
			name:       "auth key wins over options apiKey",
			auth:       apiAuth("auth-secret"),
			configured: map[string]providerConfig{"beeline": beeline(`"config-secret"`)},
			want: []llm.ModelConfig{{
				Name: "opencode/beeline/sonnet", APIName: "sonnet", ProviderID: "beeline",
				Protocol: llm.ProtocolOpenAI, APIKey: "auth-secret", BaseURL: "https://beeline.example/v1",
				ContextWindow: 128000, MaxOutputTokens: 4096,
			}},
		},
		{
			name: "catalog and auth providers import sorted by name",
			auth: map[string]authEntry{
				"zline":     {Type: "api", Key: "zline-secret"},
				"anthropic": {Type: "api", Key: "anthropic-secret"},
			},
			want: []llm.ModelConfig{
				{
					Name: "opencode/anthropic/claude-3", APIName: "claude-3", ProviderID: "anthropic",
					Protocol: llm.ProtocolAnthropic, APIKey: "anthropic-secret", BaseURL: "https://api.anthropic.com",
					ContextWindow: 200000, MaxOutputTokens: 4096,
				},
				{
					Name: "opencode/anthropic/claude-4", APIName: "claude-4", ProviderID: "anthropic",
					Protocol: llm.ProtocolAnthropic, APIKey: "anthropic-secret", BaseURL: "https://api.anthropic.com",
					ContextWindow: 1000000, MaxOutputTokens: 64000,
				},
				{
					Name: "opencode/zline/z-1", APIName: "z-1", ProviderID: "zline",
					Protocol: llm.ProtocolOpenAI, APIKey: "zline-secret", BaseURL: "https://zline.example/v1",
					ContextWindow: 64000, MaxOutputTokens: 1024,
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, resolveModels(test.auth, test.configured, catalog, test.disabled, keys))
		})
	}
}

func TestLoadImportsConfigOnlyProviderWithFileKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "beeline.key")
	require.NoError(t, os.WriteFile(keyPath, []byte("beeline-file-secret\n"), 0o600))
	authPath := filepath.Join(dir, "auth.json")
	require.NoError(t, os.WriteFile(authPath, []byte(`{"openai":{"type":"api","key":"openai-secret"}}`), 0o600))
	configPath := filepath.Join(dir, "opencode.json")
	config := fmt.Sprintf(`{
		"provider": {
			"beeline": {
				"npm": "@ai-sdk/openai",
				"options": {"baseURL": "https://beeline.example/v1", "apiKey": "{file:%s}"},
				"models": {"sonnet": {"limit": {"context": 200000, "output": 8192}}}
			}
		}
	}`, keyPath)
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0o600))

	source, err := Load(Options{
		ConfigPath: configPath,
		AuthPath:   authPath,
		Catalog: []provider.Info{{
			ID: "openai", BaseURL: "https://api.openai.com/v1", Protocol: llm.ProtocolOpenAI,
			Models: []provider.Model{{ID: "gpt-test"}},
		}},
	})
	require.NoError(t, err)

	models := source.Models()
	require.Len(t, models, 2)
	assert.Equal(t, "opencode/beeline/sonnet", models[0].Name)
	assert.Equal(t, "beeline-file-secret", models[0].APIKey)
	assert.Equal(t, "https://beeline.example/v1", models[0].BaseURL)
	assert.Equal(t, 200000, models[0].ContextWindow)
	assert.Equal(t, "opencode/openai/gpt-test", models[1].Name)
	assert.Equal(t, "openai-secret", models[1].APIKey)
}

func TestLoadDisabledProvidersAreSkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "opencode.json")
	require.NoError(t, os.WriteFile(authPath, []byte(`{
	"openai": {"type":"api", "key":"openai-secret"},
	"zline":  {"type":"api", "key":"zline-secret"}
}`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`{"disabled_providers":["openai"]}`), 0o600))
	source, err := Load(Options{
		ConfigPath: configPath,
		AuthPath:   authPath,
		Catalog: []provider.Info{
			{
				ID: "openai", BaseURL: "https://api.openai.com/v1", Protocol: llm.ProtocolOpenAI,
				Models: []provider.Model{{ID: "gpt-test"}},
			},
			{
				ID: "zline", BaseURL: "https://zline.example/v1", Protocol: llm.ProtocolOpenAI,
				Models: []provider.Model{{ID: "z-1"}},
			},
		},
	})
	require.NoError(t, err)

	models := source.Models()
	require.Len(t, models, 1)
	assert.Equal(t, "opencode/zline/z-1", models[0].Name)
	assert.Equal(t, "zline-secret", models[0].APIKey)
}
