package provider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestCodexDeviceAuthorizationPersistsAndAuthorizes(t *testing.T) {
	t.Parallel()

	var polls atomic.Int32
	var tokenForm url.Values
	accountID := "acct_test"
	access := testJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": accountID, "chatgpt_compute_residency": "eu",
		},
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			require.Equal(t, http.MethodPost, r.Method)
			_, _ = fmt.Fprint(w, `{"device_auth_id":"device-1","user_code":"ABCD-EFGH","interval":"1"}`)
		case "/api/accounts/deviceauth/token":
			polls.Add(1)
			_, _ = fmt.Fprint(w, `{"authorization_code":"auth-code","code_verifier":"verifier"}`)
		case "/oauth/token":
			require.NoError(t, r.ParseForm())
			tokenForm = r.Form
			_, _ = fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"refresh-1","expires_in":3600}`, access)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "providers.json")
	credentialsPath := filepath.Join(dir, "credentials.json")
	manager, err := Open(Options{
		CachePath:       cachePath,
		CredentialsPath: credentialsPath,
		HTTPClient:      server.Client(),
	})
	require.NoError(t, err)
	manager.oauthIssuer = server.URL

	flow, err := manager.BeginDeviceAuthorization(t.Context(), "codex")
	require.NoError(t, err)
	require.Equal(t, server.URL+"/codex/device", flow.VerificationURL)
	require.Equal(t, "ABCD-EFGH", flow.UserCode)

	require.NoError(t, manager.CompleteDeviceAuthorization(t.Context(), flow))
	require.Equal(t, int32(1), polls.Load())
	require.Equal(t, "authorization_code", tokenForm.Get("grant_type"))
	require.Equal(t, codexClientID, tokenForm.Get("client_id"))
	require.Equal(t, "auth-code", tokenForm.Get("code"))
	require.Equal(t, server.URL+"/deviceauth/callback", tokenForm.Get("redirect_uri"))

	reopened, err := Open(Options{
		CachePath: cachePath, CredentialsPath: credentialsPath, HTTPClient: server.Client(),
	})
	require.NoError(t, err)
	models := reopened.Models()
	var codex llm.ModelConfig
	for _, model := range models {
		if model.ProviderID == "codex" {
			codex = model
			break
		}
	}
	require.NotNil(t, codex.Authenticator)
	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"https://chatgpt.com/backend-api/codex/responses",
		http.NoBody,
	)
	require.NoError(t, codex.Authenticator.Authorize(t.Context(), req))
	require.Equal(t, "Bearer "+access, req.Header.Get("Authorization"))
	require.Equal(t, accountID, req.Header.Get("ChatGPT-Account-Id"))
	require.Equal(t, "eu", req.Header.Get("x-openai-internal-codex-residency"))

	untrusted := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "https://example.invalid/responses", http.NoBody,
	)
	require.ErrorContains(t, codex.Authenticator.Authorize(t.Context(), untrusted), "does not match")
	require.Empty(t, untrusted.Header.Get("Authorization"))
}

func TestCodexDeviceAuthorizationExplainsDisabledDeviceLogin(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	dir := t.TempDir()
	manager, err := Open(Options{
		CachePath:       filepath.Join(dir, "providers.json"),
		CredentialsPath: filepath.Join(dir, "credentials.json"),
		HTTPClient:      server.Client(),
	})
	require.NoError(t, err)
	manager.oauthIssuer = server.URL

	_, err = manager.BeginDeviceAuthorization(t.Context(), "codex")
	require.ErrorContains(t, err, "enable device code authorization")
}

func testJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	encode := base64.RawURLEncoding.EncodeToString
	return encode([]byte(`{"alg":"none"}`)) + "." + encode(payload) + ".signature"
}

func TestOAuthAuthenticatorRefreshesExpiredTokenOnce(t *testing.T) {
	t.Parallel()

	var refreshes atomic.Int32
	newAccess := testJWT(t, map[string]any{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/oauth/token", r.URL.Path)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		require.Equal(t, "refresh-old", r.Form.Get("refresh_token"))
		refreshes.Add(1)
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"refresh-new","expires_in":3600}`, newAccess)
	}))
	defer server.Close()

	dir := t.TempDir()
	manager, err := Open(Options{
		CachePath:       filepath.Join(dir, "providers.json"),
		CredentialsPath: filepath.Join(dir, "credentials.json"),
		HTTPClient:      server.Client(),
	})
	require.NoError(t, err)
	manager.oauthIssuer = server.URL
	manager.credentials["codex"] = credential{
		Type: "oauth", Access: "expired", Refresh: "refresh-old",
		Expires:   time.Now().Add(-time.Minute).UnixMilli(),
		AccountID: "acct_old",
		BaseURL:   "https://chatgpt.com/backend-api/codex",
		Protocol:  llm.ProtocolOpenAIResponses,
	}

	auth := &oauthAuthenticator{manager: manager, providerID: "codex"}
	for range 2 {
		req := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"https://chatgpt.com/backend-api/codex/responses",
			http.NoBody,
		)
		require.NoError(t, auth.Authorize(t.Context(), req))
		require.True(t, strings.HasPrefix(req.Header.Get("Authorization"), "Bearer "))
		require.Equal(t, "acct_old", req.Header.Get("ChatGPT-Account-Id"))
	}
	require.Equal(t, int32(1), refreshes.Load())
}
