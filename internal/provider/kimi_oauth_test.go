package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// kimiAuthServer serves the RFC 8628 flow: one authorization_pending poll,
// one slow_down poll, then tokens. It also serves the account-bound model
// listing the sign-in refreshes right after.
func kimiAuthServer(t *testing.T, polls *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/oauth/device_authorization":
			require.NoError(t, r.ParseForm())
			require.Equal(t, kimiClientID, r.Form.Get("client_id"))
			_, _ = fmt.Fprint(w, `{"device_code":"dc-1","user_code":"WDJB-MJHT",`+
				`"verification_uri_complete":"https://auth.kimi.com/device?uc=WDJB-MJHT",`+
				`"verification_uri":"https://auth.kimi.com/device","interval":1,"expires_in":300}`)
		case "/api/oauth/token":
			require.NoError(t, r.ParseForm())
			if r.Form.Get("grant_type") == "refresh_token" {
				_, _ = fmt.Fprint(w, `{"access_token":"refreshed-access","expires_in":900}`)
				return
			}
			switch n := polls.Add(1); n {
			case 1:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(w, `{"error":"authorization_pending"}`)
			case 2:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(w, `{"error":"slow_down"}`)
			default:
				require.Equal(t, kimiDeviceGrant, r.Form.Get("grant_type"))
				require.Equal(t, "dc-1", r.Form.Get("device_code"))
				_, _ = fmt.Fprint(w, `{"access_token":"kimi-access","refresh_token":"kimi-refresh","expires_in":900}`)
			}
		case "/coding/v1/models":
			require.Equal(t, "Bearer kimi-access", r.Header.Get("Authorization"))
			_, _ = fmt.Fprint(w, `{"data":[
				{"id":"kimi-for-coding","display_name":"Kimi K2.7 Coding","context_length":262144},
				{"id":"k3","display_name":"Kimi K3","context_length":1048576}
			]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func kimiManager(t *testing.T, server *httptest.Server) *Manager {
	t.Helper()
	dir := t.TempDir()
	manager, err := Open(Options{
		CachePath:       filepath.Join(dir, "providers.json"),
		CredentialsPath: filepath.Join(dir, "credentials.json"),
		HTTPClient:      loopbackClient(t, server),
	})
	require.NoError(t, err)
	manager.kimiIssuer = server.URL
	return manager
}

func TestKimiDeviceAuthorizationPersistsAndAuthorizes(t *testing.T) {
	t.Parallel()

	var polls atomic.Int32
	server := kimiAuthServer(t, &polls)
	manager := kimiManager(t, server)

	flow, err := manager.BeginDeviceAuthorization(t.Context(), kimiProviderID)
	require.NoError(t, err)
	require.Equal(t, "https://auth.kimi.com/device?uc=WDJB-MJHT", flow.VerificationURL)
	require.Equal(t, "WDJB-MJHT", flow.UserCode)

	require.NoError(t, manager.CompleteDeviceAuthorization(t.Context(), flow))
	require.Equal(t, int32(3), polls.Load())

	current := manager.credentials[kimiProviderID]
	require.Equal(t, "oauth", current.Type)
	require.Equal(
		t,
		kimiAccountID,
		current.AccountID,
		"a kimi connection keys its model cache with the constant account id",
	)
	require.Equal(t, kimiAPIBaseURL, current.BaseURL)
	require.Equal(t, llm.ProtocolOpenAI, current.Protocol)
	require.Equal(t, kimiModelsCacheTag, current.ModelsClientVersion)
	require.Len(t, current.Models, 2, "the live /models listing replaced the offline baseline")

	models := manager.Models()
	var subscription llm.ModelConfig
	for _, model := range models {
		if model.ProviderID == kimiProviderID && model.APIName == "k3" {
			subscription = model
			break
		}
	}
	require.NotNil(t, subscription.Authenticator, "an oauth credential routes requests through the authenticator")
	require.Equal(t, 1048576, subscription.ContextWindow)
	require.Equal(t, kimiMaxOutputTokens, subscription.MaxOutputTokens)

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, kimiAPIBaseURL+"/chat/completions", http.NoBody,
	)
	require.NoError(t, subscription.Authenticator.Authorize(t.Context(), req))
	require.Equal(t, "Bearer kimi-access", req.Header.Get("Authorization"))
	assert.Empty(t, req.Header.Get("ChatGPT-Account-Id"), "kimi requests carry no OpenAI account header")

	untrusted := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "https://example.invalid/chat/completions", http.NoBody,
	)
	require.ErrorContains(t, subscription.Authenticator.Authorize(t.Context(), untrusted), "does not match")
}

func TestKimiDeviceAuthorizationDenied(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/oauth/device_authorization":
			_, _ = fmt.Fprint(
				w,
				`{"device_code":"dc-1","user_code":"WDJB-MJHT","verification_uri":"https://auth.kimi.com/device","interval":1}`,
			)
		case "/api/oauth/token":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":"access_denied"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	manager := kimiManager(t, server)

	flow, err := manager.BeginDeviceAuthorization(t.Context(), kimiProviderID)
	require.NoError(t, err)
	err = manager.CompleteDeviceAuthorization(t.Context(), flow)
	require.ErrorContains(t, err, "denied")
	assert.Empty(t, manager.credentials[kimiProviderID].Access)
}

func TestKimiSlowDownLengthensThePollInterval(t *testing.T) {
	t.Parallel()

	var polls atomic.Int32
	server := kimiAuthServer(t, &polls)
	manager := kimiManager(t, server)

	flow, err := manager.BeginDeviceAuthorization(t.Context(), kimiProviderID)
	require.NoError(t, err)
	require.Equal(t, time.Second, flow.interval)

	token, pending, err := (kimiGrant{}).pollDevice(t.Context(), manager, &flow)
	require.NoError(t, err)
	require.True(t, pending)
	require.Empty(t, token.AccessToken)

	_, pending, err = (kimiGrant{}).pollDevice(t.Context(), manager, &flow)
	require.NoError(t, err)
	require.True(t, pending)
	require.Equal(t, time.Second+kimiSlowDownStep, flow.interval)
}

func TestKimiRefreshExchangesStoredTokenOnExpiry(t *testing.T) {
	t.Parallel()

	var polls atomic.Int32
	server := kimiAuthServer(t, &polls)
	manager := kimiManager(t, server)

	// A stored credential past its expiry: the next authorized request must
	// exchange the refresh token before it goes out.
	manager.mu.Lock()
	manager.credentials[kimiProviderID] = credential{
		Type: "oauth", Access: "stale-access", Refresh: "kimi-refresh",
		Expires: time.Now().Add(-time.Minute).UnixMilli(), AccountID: kimiAccountID,
		BaseURL: kimiAPIBaseURL, Protocol: llm.ProtocolOpenAI,
	}
	manager.mu.Unlock()

	refreshed, err := manager.validOAuthCredential(t.Context(), kimiProviderID)
	require.NoError(t, err)
	require.Equal(t, "refreshed-access", refreshed.Access)
	require.Equal(
		t,
		"kimi-refresh",
		refreshed.Refresh,
		"a refresh response without a new refresh token keeps the stored one",
	)
	require.Positive(t, refreshed.Expires)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, kimiAPIBaseURL+"/models", http.NoBody)
	require.NoError(t, (kimiGrant{}).authorize(req, refreshed))
	require.Equal(t, "Bearer refreshed-access", req.Header.Get("Authorization"))
}

func TestKimiProviderPinsSubscriptionOnlySignIn(t *testing.T) {
	t.Parallel()

	manager := kimiManager(t, httptest.NewServer(http.NotFoundHandler()))

	item, ok := manager.providers[kimiProviderID]
	require.True(t, ok)
	methods := item.AuthMethods()
	require.Len(t, methods, 1)
	require.Equal(t, AuthOAuthDevice, methods[0].Kind)
	require.Equal(t, kimiAPIBaseURL, methods[0].BaseURL)
	require.Equal(t, llm.ProtocolOpenAI, methods[0].Protocol)
	require.Equal(
		t,
		kimiModels(),
		connectedModels(item, credential{Type: "oauth", BaseURL: kimiAPIBaseURL, Protocol: llm.ProtocolOpenAI}),
	)

	err := manager.Connect(ConnectRequest{
		ProviderID: kimiProviderID, ExpectedBaseURL: kimiAPIBaseURL, APIKey: "sk-kimi",
	})
	require.ErrorContains(t, err, "requires subscription sign-in")
}

func TestDecodeKimiModels(t *testing.T) {
	t.Parallel()

	t.Run("happy path sorts and pins the output limit", func(t *testing.T) {
		models, err := decodeKimiModels(strings.NewReader(`{"data":[
			{"id":"kimi-for-coding","context_length":262144},
			{"id":"k3","display_name":"Kimi K3","context_length":1048576}
		]}`))
		require.NoError(t, err)
		require.Equal(t, []Model{
			{ID: "k3", Name: "Kimi K3", ContextWindow: 1048576, MaxOutputTokens: kimiMaxOutputTokens},
			{
				ID:              "kimi-for-coding",
				Name:            "kimi-for-coding",
				ContextWindow:   262144,
				MaxOutputTokens: kimiMaxOutputTokens,
			},
		}, models)
	})

	t.Run("rejects empty, duplicate, and malformed listings", func(t *testing.T) {
		_, err := decodeKimiModels(strings.NewReader(`{"data":[]}`))
		require.ErrorContains(t, err, "model count")

		_, err = decodeKimiModels(strings.NewReader(`{"data":[{"id":"k3"},{"id":"k3"}]}`))
		require.ErrorContains(t, err, "duplicate")

		_, err = decodeKimiModels(strings.NewReader(`{"data":[{"id":"Bad ID"},{"id":"k3"}]}`))
		require.ErrorContains(t, err, "invalid listed model id")

		_, err = decodeKimiModels(strings.NewReader(`{"data":[{"id":"k3","context_length":-1}]}`))
		require.ErrorContains(t, err, "invalid metadata")
	})
}
