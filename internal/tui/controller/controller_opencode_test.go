package controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/opencode"
	"github.com/alvnukov/cozyphi/internal/provider"
)

func TestControllerResolvesOpenCodeModel(t *testing.T) {
	proj := writeAgentConfig(t, "")
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "opencode.json")
	require.NoError(t, os.WriteFile(authPath, []byte(`{"openai":{"type":"api","key":"imported-key"}}`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`{}`), 0o600))

	source, err := opencode.Load(opencode.Options{
		AuthPath: authPath, ConfigPath: configPath,
		Catalog: []provider.Info{{
			ID: "openai", BaseURL: "https://api.example/v1", Protocol: llm.ProtocolOpenAI,
			Models: []provider.Model{{ID: "gpt-test"}},
		}},
	})
	require.NoError(t, err)
	ctrl := &Controller{proj: proj, opencode: source}

	assert.Contains(t, ctrl.ModelNames(), "opencode/openai/gpt-test")
	model, ok := ctrl.findModel("opencode/openai/gpt-test")
	require.True(t, ok)
	assert.Equal(t, "gpt-test", model.APIName)
	assert.Equal(t, "imported-key", model.APIKey)
}
