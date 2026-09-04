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

	// A config directory yields opencode's three global files in load order.
	configPaths, authPath, err := paths(Options{})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"/custom/config/config.json",
		"/custom/config/opencode.json",
		"/custom/config/opencode.jsonc",
	}, configPaths)
	assert.Equal(t, "/xdg/data/opencode/auth.json", authPath)

	t.Setenv("OPENCODE_CONFIG_DIR", "")
	configPaths, _, err = paths(Options{})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"/xdg/config/opencode/config.json",
		"/xdg/config/opencode/opencode.json",
		"/xdg/config/opencode/opencode.jsonc",
	}, configPaths)

	// OPENCODE_CONFIG names one more file that loads after and merges over
	// the three globals.
	t.Setenv("OPENCODE_CONFIG", "/exact/opencode.jsonc")
	configPaths, _, err = paths(Options{})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"/xdg/config/opencode/config.json",
		"/xdg/config/opencode/opencode.json",
		"/xdg/config/opencode/opencode.jsonc",
		"/exact/opencode.jsonc",
	}, configPaths)

	// The Options.ConfigPath test seam keeps single-file semantics.
	configPaths, _, err = paths(Options{ConfigPath: "/seam/opencode.json"})
	require.NoError(t, err)
	assert.Equal(t, []string{"/seam/opencode.json"}, configPaths)
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
	beeline := func(apiKey string) providerConfig {
		return providerConfig{
			NPM:     "@ai-sdk/openai",
			Options: providerOptions{BaseURL: "https://beeline.example/v1/", APIKey: new(apiKey)},
			Models: map[string]providerModel{
				"sonnet": {Limit: modelLimit{Context: new(128000), Output: new(4096)}},
			},
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
				"beeline": beeline("config-secret"),
			},
			want: []llm.ModelConfig{{
				Name: "opencode/beeline/sonnet", APIName: "sonnet", ProviderID: "beeline",
				Protocol: llm.ProtocolOpenAI, APIKey: "config-secret", BaseURL: "https://beeline.example/v1",
				ContextWindow: 128000, MaxOutputTokens: 4096,
			}},
		},
		{
			name: "config apiKey beats auth.json key",
			auth: apiAuth("auth-secret"),
			configured: map[string]providerConfig{
				"beeline": beeline("config-secret"),
			},
			want: []llm.ModelConfig{{
				Name: "opencode/beeline/sonnet", APIName: "sonnet", ProviderID: "beeline",
				Protocol: llm.ProtocolOpenAI, APIKey: "config-secret", BaseURL: "https://beeline.example/v1",
				ContextWindow: 128000, MaxOutputTokens: 4096,
			}},
		},
		{
			// An explicitly empty apiKey is a value in opencode's undefined
			// check: it stays and blocks the auth.json fallback.
			name: "explicit empty apiKey suppresses the auth.json fallback",
			auth: apiAuth("auth-secret"),
			configured: map[string]providerConfig{
				"beeline": beeline(""),
			},
			want: []llm.ModelConfig{{
				Name: "opencode/beeline/sonnet", APIName: "sonnet", ProviderID: "beeline",
				Protocol: llm.ProtocolOpenAI, BaseURL: "https://beeline.example/v1",
				ContextWindow: 128000, MaxOutputTokens: 4096,
			}},
		},
		{
			name: "config only keyless provider imports with empty key",
			configured: map[string]providerConfig{
				"gateway": {
					NPM:     "@ai-sdk/openai-compatible",
					Options: providerOptions{BaseURL: "http://localhost:8080/v1"},
					Models:  map[string]providerModel{"local-model": {Limit: modelLimit{Context: new(8192)}}},
				},
			},
			want: []llm.ModelConfig{{
				Name: "opencode/gateway/local-model", APIName: "local-model", ProviderID: "gateway",
				Protocol: llm.ProtocolOpenAI, BaseURL: "http://localhost:8080/v1", ContextWindow: 8192,
			}},
		},
		{
			// opencode defaults an unknown config provider to the
			// openai-compatible adapter, so a missing baseURL no longer drops
			// the provider: it imports with an empty endpoint.
			name: "unknown provider without npm or baseURL defaults to openai-compatible",
			configured: map[string]providerConfig{
				"beeline": {
					Options: providerOptions{APIKey: new("k")},
					Models:  map[string]providerModel{"sonnet": {}},
				},
			},
			want: []llm.ModelConfig{{
				Name: "opencode/beeline/sonnet", APIName: "sonnet", ProviderID: "beeline",
				Protocol: llm.ProtocolOpenAI, APIKey: "k",
			}},
		},
		{
			name: "config only provider without models is skipped",
			configured: map[string]providerConfig{
				"beeline": {
					NPM:     "@ai-sdk/openai",
					Options: providerOptions{BaseURL: "https://beeline.example/v1", APIKey: new("k")},
				},
			},
		},
		{
			name: "config only provider with unsupported adapter is skipped",
			configured: map[string]providerConfig{
				"beeline": {
					NPM:     "@ai-sdk/google",
					Options: providerOptions{BaseURL: "https://beeline.example/v1", APIKey: new("k")},
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
					"claude-4": {Limit: modelLimit{Context: new(500000)}},
					// model.id names the API model; the map key stays the
					// public id and seeds its limits from the catalog match.
					"fast": {ID: "claude-4"},
					// New model via the map key.
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
					Name: "opencode/anthropic/fast", APIName: "claude-4", ProviderID: "anthropic",
					Protocol: llm.ProtocolAnthropic, APIKey: "auth-secret", BaseURL: "https://api.anthropic.com",
					ContextWindow: 500000, MaxOutputTokens: 64000,
				},
				{
					Name: "opencode/anthropic/turbo", APIName: "turbo", ProviderID: "anthropic",
					Protocol: llm.ProtocolAnthropic, APIKey: "auth-secret", BaseURL: "https://api.anthropic.com",
				},
			},
		},
		{
			// A configured zero is a value in opencode's ?? chain: it wins
			// over the catalog instead of being treated as unset.
			name: "limit context zero overrides the catalog",
			auth: map[string]authEntry{"anthropic": {Type: "api", Key: "auth-secret"}},
			configured: map[string]providerConfig{
				"anthropic": {Models: map[string]providerModel{
					"claude-4": {Limit: modelLimit{Context: new(0)}},
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
					MaxOutputTokens: 64000,
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
			// options.baseURL is provider-level runtime config: it wins over
			// the provider api url for every model, like opencode's adapter
			// options check.
			name: "options baseURL beats provider api url and catalog",
			auth: map[string]authEntry{"anthropic": {Type: "api", Key: "auth-secret"}},
			configured: map[string]providerConfig{
				"anthropic": {API: "https://api.example/v1/", Options: providerOptions{BaseURL: "https://opt.example"}},
			},
			want: []llm.ModelConfig{
				{
					Name: "opencode/anthropic/claude-3", APIName: "claude-3", ProviderID: "anthropic",
					Protocol: llm.ProtocolAnthropic, APIKey: "auth-secret", BaseURL: "https://opt.example",
					ContextWindow: 200000, MaxOutputTokens: 4096,
				},
				{
					Name: "opencode/anthropic/claude-4", APIName: "claude-4", ProviderID: "anthropic",
					Protocol: llm.ProtocolAnthropic, APIKey: "auth-secret", BaseURL: "https://opt.example",
					ContextWindow: 1000000, MaxOutputTokens: 64000,
				},
			},
		},
		{
			// opencode folds the provider api url into the models listed in
			// the config only; unlisted catalog models keep the catalog url.
			name: "provider api url moves only config-listed models",
			auth: map[string]authEntry{"anthropic": {Type: "api", Key: "auth-secret"}},
			configured: map[string]providerConfig{
				"anthropic": {API: "https://api.example/v1/", Models: map[string]providerModel{
					"claude-4": {},
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
					Protocol: llm.ProtocolAnthropic, APIKey: "auth-secret", BaseURL: "https://api.example/v1",
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
			assert.Equal(t, test.want, resolveModels(test.auth, test.configured, catalog, test.disabled, nil))
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

func TestLoadExpandsMCPFileTokens(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "gateway.key")
	require.NoError(t, os.WriteFile(keyPath, []byte("  mcp-file-secret\n"), 0o600))
	quotedPath := filepath.Join(dir, "quoted.key")
	require.NoError(t, os.WriteFile(quotedPath, []byte("line one\nweird \"key\"\\path\n"), 0o600))
	configPath := filepath.Join(dir, "opencode.json")
	config := fmt.Sprintf(`{
		"mcp": {
			"local": {"type":"local", "command":["node","server.js"], "environment":{"TOKEN":"{file:%s}"}},
			"remote": {"type":"remote", "url":"https://mcp.example", "headers":{
				"Authorization":"Bearer {file:%s}",
				"X-Quoted":"{file:%s}"
			}}
		}
	}`, keyPath, keyPath, quotedPath)
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0o600))

	source, err := Load(Options{
		ConfigPath: configPath,
		AuthPath:   filepath.Join(dir, "missing-auth.json"),
	})
	require.NoError(t, err)

	servers := source.MCPServers()
	require.Len(t, servers, 2)
	assert.Equal(t, "mcp-file-secret", servers["local"].Env["TOKEN"])
	headers := servers["remote"].Headers
	assert.Equal(t, "Bearer mcp-file-secret", headers["Authorization"])
	// Content is trimmed and spliced in as an escaped JSON string body, so
	// quotes, backslashes and embedded newlines survive the parse exactly.
	assert.Equal(t, "line one\nweird \"key\"\\path", headers["X-Quoted"])
}

func TestLoadFileTokenMissingFailsLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	missing := filepath.Join(dir, "missing.key")
	config := fmt.Sprintf(`{"mcp":{"remote":{"type":"remote","url":"https://mcp.example",
		"headers":{"Authorization":"{file:%s}"}}}}`, missing)
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0o600))

	_, err := Load(Options{ConfigPath: configPath, AuthPath: filepath.Join(dir, "auth.json")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("bad file reference: %q", "{file:"+missing+"}"))
	assert.Contains(t, err.Error(), missing+" does not exist")
}

func TestLoadFileTokenOnCommentLineStaysLiteral(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	// The token sits on a //-commented line, so opencode leaves it alone: a
	// commented-out reference to a missing file must not fail the load.
	config := fmt.Sprintf(`{
		// "remote": {"type":"remote", "url":"https://mcp.example", "headers":{"Authorization":"{file:%s}"}}
		"mcp": {"local": {"type":"local", "command":["node","server.js"]}}
	}`, filepath.Join(dir, "missing.key"))
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0o600))

	source, err := Load(Options{ConfigPath: configPath, AuthPath: filepath.Join(dir, "auth.json")})
	require.NoError(t, err)
	servers := source.MCPServers()
	require.Len(t, servers, 1)
	assert.Equal(t, []string{"node"}, servers["local"].Command)
}

func TestLoadFileTokenResolvesRelativeAndHomePaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "home.key"), []byte("home-secret\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "keys"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keys", "note.key"), []byte("relative-secret\n"), 0o600))
	authPath := filepath.Join(dir, "auth.json")

	configPath := filepath.Join(dir, "opencode.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{
		"mcp": {"remote": {"type":"remote", "url":"https://mcp.example", "headers":{
			"X-Relative":"{file:keys/note.key}",
			"X-Home":"{file:~/home.key}"
		}}}
	}`), 0o600))
	source, err := Load(Options{ConfigPath: configPath, AuthPath: authPath})
	require.NoError(t, err)
	headers := source.MCPServers()["remote"].Headers
	// Relative references resolve against the config file's directory.
	assert.Equal(t, "relative-secret", headers["X-Relative"])
	assert.Equal(t, "home-secret", headers["X-Home"])

	// A "~" reference to a missing file names the home-resolved path.
	require.NoError(t, os.WriteFile(configPath, []byte(`{
		"mcp": {"remote": {"type":"remote", "url":"https://mcp.example", "headers":{"X-Missing":"{file:~/missing.key}"}}}
	}`), 0o600))
	_, err = Load(Options{ConfigPath: configPath, AuthPath: authPath})
	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Join(dir, "missing.key")+" does not exist")
}

func TestLoadEnvValueWithFileTokenExpands(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "gateway.key")
	require.NoError(t, os.WriteFile(keyPath, []byte("nested-secret\n"), 0o600))
	configPath := filepath.Join(dir, "opencode.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{
		"mcp": {"local": {"type":"local", "command":["node"], "environment":{"TOKEN":"{env:TOKEN}"}}}
	}`), 0o600))

	// The env pass runs before the file pass over the same text, so a file
	// token carried by an environment value still expands.
	env := map[string]string{"TOKEN": "{file:" + keyPath + "}"}
	source, err := Load(Options{
		ConfigPath: configPath,
		AuthPath:   filepath.Join(dir, "auth.json"),
		LookupEnv:  func(name string) string { return env[name] },
	})
	require.NoError(t, err)
	assert.Equal(t, "nested-secret", source.MCPServers()["local"].Env["TOKEN"])
}

func TestLoadFileContentWithTokensIsNotReExpanded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "note.txt")
	require.NoError(t, os.WriteFile(keyPath, []byte("value {file:/definitely/missing.key}\n"), 0o600))
	configPath := filepath.Join(dir, "opencode.json")
	config := fmt.Sprintf(`{
		"mcp": {"remote": {"type":"remote", "url":"https://mcp.example", "headers":{"X-Note":"{file:%s}"}}}
	}`, keyPath)
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0o600))

	// The file pass runs once: tokens inside spliced content stay literal.
	source, err := Load(Options{ConfigPath: configPath, AuthPath: filepath.Join(dir, "auth.json")})
	require.NoError(t, err)
	assert.Equal(t, "value {file:/definitely/missing.key}", source.MCPServers()["remote"].Headers["X-Note"])
}

func TestLoadMergesGlobalConfigFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OPENCODE_CONFIG", "")
	t.Setenv("OPENCODE_CONFIG_DIR", dir)
	authPath := filepath.Join(dir, "auth.json")
	require.NoError(t, os.WriteFile(authPath, []byte(`{"anthropic":{"type":"api","key":"auth-secret"}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{
		"provider": {
			"gateway": {
				"npm": "@ai-sdk/openai-compatible",
				"options": {"baseURL": "https://one.example/v1", "apiKey": "one-secret"},
				"models": {"a": {"limit": {"context": 1000}}}
			}
		}
	}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{
		"provider": {
			"gateway": {"models": {"b": {"limit": {"context": 2000}}}},
			"second": {"npm": "@ai-sdk/openai-compatible", "options": {"baseURL": "https://two.example/v1"}, "models": {"s": {}}}
		}
	}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode.jsonc"), []byte(`{
		// jsonc comments load, and the last file wins on scalars.
		"provider": {"gateway": {"options": {"apiKey": "three-secret"}}}
	}`), 0o600))

	source, err := Load(Options{AuthPath: authPath})
	require.NoError(t, err)

	models := source.Models()
	require.Len(t, models, 3)
	assert.Equal(t, "opencode/gateway/a", models[0].Name)
	assert.Equal(t, "three-secret", models[0].APIKey)
	assert.Equal(t, "https://one.example/v1", models[0].BaseURL)
	assert.Equal(t, 1000, models[0].ContextWindow)
	assert.Equal(t, "opencode/gateway/b", models[1].Name)
	assert.Equal(t, "three-secret", models[1].APIKey)
	assert.Equal(t, 2000, models[1].ContextWindow)
	assert.Equal(t, "opencode/second/s", models[2].Name)
	assert.Equal(t, "https://two.example/v1", models[2].BaseURL)
}

func TestLoadOpenCodeConfigMergesOverGlobals(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OPENCODE_CONFIG", filepath.Join(dir, "extra.json"))
	t.Setenv("OPENCODE_CONFIG_DIR", dir)
	authPath := filepath.Join(dir, "auth.json")
	require.NoError(t, os.WriteFile(authPath, []byte(`{"anthropic":{"type":"api","key":"auth-secret"}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{
		"provider": {
			"gateway": {
				"npm": "@ai-sdk/openai-compatible",
				"options": {"baseURL": "https://one.example/v1", "apiKey": "one-secret"},
				"models": {"a": {"limit": {"context": 1000}}}
			}
		}
	}`), 0o600))
	// The OPENCODE_CONFIG file loads after the globals and wins field by field.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.json"), []byte(`{
		"provider": {"gateway": {"options": {"apiKey": "extra-secret"}}}
	}`), 0o600))

	source, err := Load(Options{AuthPath: authPath})
	require.NoError(t, err)

	models := source.Models()
	require.Len(t, models, 1)
	assert.Equal(t, "opencode/gateway/a", models[0].Name)
	assert.Equal(t, "extra-secret", models[0].APIKey)
	assert.Equal(t, "https://one.example/v1", models[0].BaseURL)
	assert.Equal(t, 1000, models[0].ContextWindow)
}

func TestLoadGlobalConfigFilesAllMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OPENCODE_CONFIG", "")
	t.Setenv("OPENCODE_CONFIG_DIR", dir)

	source, err := Load(Options{AuthPath: filepath.Join(dir, "auth.json")})
	require.NoError(t, err)
	assert.Empty(t, source.Models())
	assert.Empty(t, source.MCPServers())
}

func TestLoadEnabledProvidersAllowlist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "opencode.json")
	require.NoError(t, os.WriteFile(authPath, []byte(`{
	"openai": {"type":"api", "key":"openai-secret"},
	"zline":  {"type":"api", "key":"zline-secret"}
}`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`{
	"enabled_providers": ["openai"],
	"provider": {"gateway": {"npm": "@ai-sdk/openai-compatible", "options": {"baseURL": "https://gw.example/v1"}, "models": {"g": {}}}}
}`), 0o600))
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
	assert.Equal(t, "opencode/openai/gpt-test", models[0].Name)
	assert.Equal(t, "openai-secret", models[0].APIKey)
}

func TestLoadEnabledProvidersEmptyAllowsNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "opencode.json")
	require.NoError(t, os.WriteFile(authPath, []byte(`{"openai":{"type":"api","key":"openai-secret"}}`), 0o600))
	// An empty enabled_providers is a present allowlist: like opencode's
	// truthy [], it allows nothing.
	require.NoError(t, os.WriteFile(configPath, []byte(`{"enabled_providers": []}`), 0o600))
	source, err := Load(Options{
		ConfigPath: configPath,
		AuthPath:   authPath,
		Catalog: []provider.Info{{
			ID: "openai", BaseURL: "https://api.openai.com/v1", Protocol: llm.ProtocolOpenAI,
			Models: []provider.Model{{ID: "gpt-test"}},
		}},
	})
	require.NoError(t, err)
	assert.Empty(t, source.Models())
}

func TestLoadEmptyAPIKeySuppressesAuthFallback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "opencode.json")
	require.NoError(t, os.WriteFile(authPath, []byte(`{"anthropic":{"type":"api","key":"auth-secret"}}`), 0o600))
	// An explicit empty apiKey is a value in opencode's undefined check: it
	// stays and blocks the auth.json fallback.
	require.NoError(t, os.WriteFile(configPath, []byte(`{
		"provider": {"anthropic": {"options": {"apiKey": ""}, "models": {"sonnet": {}}}}
	}`), 0o600))
	source, err := Load(Options{
		ConfigPath: configPath,
		AuthPath:   authPath,
		Catalog: []provider.Info{{
			ID: "anthropic", BaseURL: "https://api.anthropic.com", Protocol: llm.ProtocolAnthropic,
			Models: []provider.Model{{ID: "claude-3", ContextWindow: 200000, MaxOutputTokens: 4096}},
		}},
	})
	require.NoError(t, err)

	// Both the listed sonnet and the unlisted catalog model carry the empty
	// key: options.apiKey is provider-level runtime config.
	models := source.Models()
	require.Len(t, models, 2)
	assert.Equal(t, "opencode/anthropic/claude-3", models[0].Name)
	assert.Empty(t, models[0].APIKey)
	assert.Equal(t, "https://api.anthropic.com", models[0].BaseURL)
	assert.Equal(t, "opencode/anthropic/sonnet", models[1].Name)
	assert.Empty(t, models[1].APIKey)
}

func TestLoadDisabledProvidersBeatEnabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "opencode.json")
	require.NoError(t, os.WriteFile(authPath, []byte(`{"openai":{"type":"api","key":"openai-secret"}}`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`{
	"enabled_providers": ["openai"],
	"disabled_providers": ["openai"]
}`), 0o600))
	source, err := Load(Options{
		ConfigPath: configPath,
		AuthPath:   authPath,
		Catalog: []provider.Info{{
			ID: "openai", BaseURL: "https://api.openai.com/v1", Protocol: llm.ProtocolOpenAI,
			Models: []provider.Model{{ID: "gpt-test"}},
		}},
	})
	require.NoError(t, err)
	assert.Empty(t, source.Models())
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
