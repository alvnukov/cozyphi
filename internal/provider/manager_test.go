package provider_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/provider"
)

func TestManagerIncludesPinnedSubscriptionProviders(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manager, err := provider.Open(provider.Options{
		CachePath:       filepath.Join(dir, "providers.json"),
		CredentialsPath: filepath.Join(dir, "credentials.json"),
	})
	require.NoError(t, err)

	items := manager.Providers()
	zai := findProvider(t, items, "zai-coding-plan")
	require.Equal(t, provider.AuthAPIKey, zai.Auth)
	require.Equal(t, llm.ProtocolOpenAI, zai.Protocol)
	require.Equal(t, "https://api.z.ai/api/coding/paas/v4", zai.BaseURL)

	codex := findProvider(t, items, "codex")
	require.Equal(t, provider.AuthOAuthDevice, codex.Auth)
	require.Equal(t, llm.ProtocolOpenAIResponses, codex.Protocol)
	require.Equal(t, "https://chatgpt.com/backend-api/codex", codex.BaseURL)

	require.NoError(t, manager.Connect(provider.ConnectRequest{
		ProviderID: "zai-coding-plan", ExpectedBaseURL: zai.BaseURL, APIKey: "coding-plan-key",
	}))
	models := manager.Models()
	require.NotEmpty(t, models)
	require.Equal(t, llm.ProtocolOpenAI, models[0].Protocol)
	require.Equal(t, "zai-coding-plan/glm-4.5-air", models[0].Name)
}

func TestManagerRefreshKeepsLastKnownGoodCatalog(t *testing.T) {
	t.Parallel()

	var body string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)

	body = catalogJSON("acme", "Acme", "https://api.acme.example/v1", "@ai-sdk/openai-compatible", "acme-chat")
	cache := filepath.Join(t.TempDir(), "providers.json")
	manager, err := provider.Open(provider.Options{
		CatalogURL:      server.URL,
		CachePath:       cache,
		CredentialsPath: filepath.Join(t.TempDir(), "credentials.json"),
		HTTPClient:      server.Client(),
	})
	require.NoError(t, err)
	require.NoError(t, manager.Refresh(t.Context()))
	require.Equal(t, []string{"acme", "codex", "zai-coding-plan"}, providerIDs(manager.Providers()))

	body = `{"acme":{"id":"acme","name":"Acme","api":"http://127.0.0.1:9000","npm":"@ai-sdk/openai-compatible","models":{}}}`
	err = manager.Refresh(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog")
	require.Equal(
		t,
		[]string{"acme", "codex", "zai-coding-plan"},
		providerIDs(manager.Providers()),
		"failed refresh must not replace live state",
	)

	reopened, err := provider.Open(provider.Options{
		CachePath:       cache,
		CredentialsPath: filepath.Join(t.TempDir(), "credentials.json"),
	})
	require.NoError(t, err)
	require.Equal(
		t,
		[]string{"acme", "codex", "zai-coding-plan"},
		providerIDs(reopened.Providers()),
		"validated cache must survive restart",
	)
}

func TestManagerRefreshRejectsRedirectsWithoutChangingCatalog(t *testing.T) {
	t.Parallel()

	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, catalogJSON(
			"redirected", "Redirected", "https://api.redirected.example/v1", "@ai-sdk/openai-compatible", "model",
		))
	}))
	t.Cleanup(target.Close)
	redirect := httptest.NewTLSServer(http.RedirectHandler(target.URL, http.StatusFound))
	t.Cleanup(redirect.Close)

	dir := t.TempDir()
	manager, err := provider.Open(provider.Options{
		CatalogURL:      redirect.URL,
		CachePath:       filepath.Join(dir, "providers.json"),
		CredentialsPath: filepath.Join(dir, "credentials.json"),
		HTTPClient:      redirect.Client(),
	})
	require.NoError(t, err)

	err = manager.Refresh(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirect")
	assert.Equal(t, []string{"codex", "zai-coding-plan"}, providerIDs(manager.Providers()))
	assert.NoFileExists(t, filepath.Join(dir, "providers.json"))
}

func TestManagerConnectPinsEndpointAndProtectsCredentialFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := filepath.Join(dir, "providers.json")
	credentials := filepath.Join(dir, "credentials.json")
	require.NoError(t, os.WriteFile(cache, []byte(validatedCacheJSON(
		"acme", "Acme", "https://api.acme.example/v1", "openai", "acme-chat",
	)), 0o600))

	manager, err := provider.Open(provider.Options{CachePath: cache, CredentialsPath: credentials})
	require.NoError(t, err)

	const secret = "sk-secret-must-not-leak"
	err = manager.Connect(provider.ConnectRequest{
		ProviderID:      "acme",
		ExpectedBaseURL: "https://attacker.example/v1",
		APIKey:          secret,
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)
	assert.NoFileExists(t, credentials)

	require.NoError(t, manager.Connect(provider.ConnectRequest{
		ProviderID:      "acme",
		ExpectedBaseURL: "https://api.acme.example/v1",
		APIKey:          secret,
	}))
	info, err := os.Stat(credentials)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	models := manager.Models()
	require.Len(t, models, 1)
	assert.Equal(t, "acme/acme-chat", models[0].Name)
	assert.Equal(t, "acme-chat", models[0].APIName)
	assert.Equal(t, "acme", models[0].ProviderID)
	assert.Equal(t, llm.ProtocolOpenAI, models[0].Protocol)
	assert.Equal(t, "https://api.acme.example/v1", models[0].BaseURL)
	assert.Equal(t, secret, models[0].APIKey)
}

func TestManagerRejectsOversizedOrUnsupportedCatalogWithoutLosingCache(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := filepath.Join(dir, "providers.json")
	require.NoError(t, os.WriteFile(cache, []byte(validatedCacheJSON(
		"safe", "Safe", "https://safe.example/v1", "anthropic", "claude-safe",
	)), 0o600))

	manager, err := provider.Open(provider.Options{
		CachePath:       cache,
		CredentialsPath: filepath.Join(dir, "credentials.json"),
	})
	require.NoError(t, err)
	require.Equal(t, []string{"codex", "safe", "zai-coding-plan"}, providerIDs(manager.Providers()))

	unsupported := strings.NewReader(catalogJSON(
		"bedrock", "Bedrock", "https://bedrock.example", "@ai-sdk/amazon-bedrock", "model",
	))
	err = manager.ReplaceCatalog(unsupported)
	require.Error(t, err)
	require.Equal(t, []string{"codex", "safe", "zai-coding-plan"}, providerIDs(manager.Providers()))
}

func TestManagerRefreshUpdatesPinnedProviderModelsWithoutChangingConnectionContract(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manager, err := provider.Open(provider.Options{
		CachePath:       filepath.Join(dir, "providers.json"),
		CredentialsPath: filepath.Join(dir, "credentials.json"),
	})
	require.NoError(t, err)

	err = manager.ReplaceCatalog(strings.NewReader(catalogJSON(
		"zai-coding-plan",
		"Untrusted renamed provider",
		"https://attacker.example/v1",
		"@ai-sdk/openai-compatible",
		"glm-catalog-new",
	)))
	require.NoError(t, err)

	zai := findProvider(t, manager.Providers(), "zai-coding-plan")
	require.Equal(t, "Z.AI Coding Plan", zai.Name)
	require.Equal(t, "https://api.z.ai/api/coding/paas/v4", zai.BaseURL)
	require.Equal(t, llm.ProtocolOpenAI, zai.Protocol)
	require.Equal(t, provider.AuthAPIKey, zai.Auth)
	require.Equal(t, []provider.Model{{
		ID: "glm-catalog-new", Name: "glm-catalog-new", ContextWindow: 128000, MaxOutputTokens: 8192,
	}}, zai.Models)
}

func findProvider(t *testing.T, items []provider.Info, id string) provider.Info {
	t.Helper()
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	require.FailNow(t, "provider not found", id)
	return provider.Info{}
}

func providerIDs(items []provider.Info) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func catalogJSON(id, name, endpoint, npm, model string) string {
	return fmt.Sprintf(`{
		%q: {
			"id": %q,
			"name": %q,
			"api": %q,
			"npm": %q,
			"env": ["ACME_API_KEY"],
			"models": {
				%q: {
					"id": %q,
					"name": %q,
					"tool_call": true,
					"limit": {"context": 128000, "output": 8192}
				}
			}
		}
	}`, id, id, name, endpoint, npm, model, model, model)
}

func validatedCacheJSON(id, name, endpoint, protocol, model string) string {
	return fmt.Sprintf(`{
		"version": 1,
		"providers": [{
			"id": %q,
			"name": %q,
			"base_url": %q,
			"protocol": %q,
			"models": [{
				"id": %q,
				"name": %q,
				"context_window": 128000,
				"max_output_tokens": 8192
			}]
		}]
	}`, id, name, endpoint, protocol, model, model)
}
