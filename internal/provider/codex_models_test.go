package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestSubscriptionModelsRefreshLegacyCacheForAstra(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		// The subscription endpoint omits Astra for the legacy client version.
		model := "gpt-6-astra"
		if r.URL.Query().Get("client_version") == "0.145.0" {
			model = "gpt-5.5"
		}
		_, _ = fmt.Fprintf(w, `{"models":[{"slug":%q,"visibility":"list"}]}`, model)
	}))
	defer server.Close()

	dir := t.TempDir()
	options := Options{
		CachePath: filepath.Join(dir, "providers.json"), CredentialsPath: filepath.Join(dir, "credentials.json"),
		HTTPClient: loopbackClient(t, server),
	}
	require.NoError(t, writeCredentials(options.CredentialsPath, map[string]credential{
		"openai": {
			Type: "oauth", Access: "test-access", Refresh: "test-refresh", AccountID: "test-account",
			Expires: time.Now().Add(time.Hour).UnixMilli(), BaseURL: chatgptCodexBaseURL,
			Protocol:        llm.ProtocolOpenAIResponses,
			Models:          []Model{{ID: "gpt-5.5", Name: "GPT-5.5"}},
			ModelsFetchedAt: time.Now().UnixMilli(), ModelsClientVersion: "0.145.0",
		},
	}))
	manager, err := Open(options)
	require.NoError(t, err)
	require.NoError(t, manager.RefreshSubscriptionModels(t.Context()))
	require.EqualValues(t, 1, requests.Load(), "a fresh cache from the old client must be refreshed")

	reopened, err := Open(options)
	require.NoError(t, err)
	var ids []string
	for _, model := range reopened.Models() {
		if model.ProviderID == "openai" {
			ids = append(ids, model.APIName)
		}
	}
	require.Contains(t, ids, "gpt-6-astra")
	require.NoError(t, reopened.RefreshSubscriptionModels(t.Context()))
	require.EqualValues(t, 1, requests.Load(), "the refreshed cache should survive reopening")
}
