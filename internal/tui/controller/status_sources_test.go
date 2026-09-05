package controller

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatusConfigSourcesAllowlistedOrigins(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(map[bool]string{false: "configured", true: "environment"}[override], func(t *testing.T) {
			t.Setenv("COZYPHI_MODEL", "")
			proj, _ := newLastModelProject(t)
			const model = "secret-model-value"
			const key = "secret-api-key-value"
			const endpoint = "https://secret-user:secret-password@example.invalid/v1"
			if override {
				t.Setenv("COZYPHI_MODEL", model)
			}
			t.Setenv("COZYPHI_API_KEY", key)
			t.Setenv("COZYPHI_BASE_URL", endpoint)
			require.NoError(
				t,
				os.WriteFile(
					proj.Global().ConfigFile(),
					[]byte("models:\n  - name: "+model+"\n    api_key: "+key+"\n    base_url: "+endpoint+"\n"),
					0o600,
				),
			)
			require.NoError(
				t,
				os.WriteFile(proj.Global().LSPConfigFile(), []byte(`{"env":{"TOKEN":"secret-lsp-token"}}`), 0o600),
			)
			require.NoError(t, proj.LoadConfig())
			c := &Controller{proj: proj}
			origin := "Model: configured/session selection"
			if override {
				origin = "Model: COZYPHI_MODEL override"
			}
			// Exact equality proves that only paths and fixed labels cross this boundary,
			// not model names, environment values, credentials, or LSP config contents.
			require.Equal(t, []string{
				"Global config: " + proj.Global().ConfigFile(),
				"Project MCP: " + proj.MCPConfigFile(),
				"Global LSP: " + proj.Global().LSPConfigFile(),
				origin,
				"OpenCode: read-only import enabled",
			}, c.StatusConfigSources())
		})
	}
}
