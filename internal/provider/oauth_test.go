package provider

import (
	"context"
	"crypto/sha256"
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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestOpenAIDeviceAuthorizationPersistsAndAuthorizes(t *testing.T) {
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
		case "/backend-api/codex/models":
			require.Equal(t, codexModelsClientVersion, r.URL.Query().Get("client_version"))
			_, _ = fmt.Fprint(w, `{"models":[
				{"slug":"gpt-5.5","display_name":"GPT-5.5","visibility":"list","context_window":400000}
			]}`)
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
		HTTPClient:      loopbackClient(t, server),
	})
	require.NoError(t, err)
	manager.oauthIssuer = server.URL

	flow, err := manager.BeginDeviceAuthorization(t.Context(), "openai")
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
	require.Equal(
		t,
		"https://chatgpt.com/backend-api/codex",
		reopened.credentials["openai"].BaseURL,
		"the headless flow pins the same subscription endpoint as the browser one",
	)
	models := reopened.Models()
	var subscription llm.ModelConfig
	for _, model := range models {
		if model.ProviderID == "openai" {
			subscription = model
			break
		}
	}
	require.NotNil(t, subscription.Authenticator)
	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"https://chatgpt.com/backend-api/codex/responses",
		http.NoBody,
	)
	require.NoError(t, subscription.Authenticator.Authorize(t.Context(), req))
	require.Equal(t, "Bearer "+access, req.Header.Get("Authorization"))
	require.Equal(t, accountID, req.Header.Get("ChatGPT-Account-Id"))
	require.Equal(t, "eu", req.Header.Get("x-openai-internal-codex-residency"))

	untrusted := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "https://example.invalid/responses", http.NoBody,
	)
	require.ErrorContains(t, subscription.Authenticator.Authorize(t.Context(), untrusted), "does not match")
	require.Empty(t, untrusted.Header.Get("Authorization"))
}

func TestOpenAIDeviceAuthorizationExplainsDisabledDeviceLogin(t *testing.T) {
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

	_, err = manager.BeginDeviceAuthorization(t.Context(), "openai")
	require.ErrorContains(t, err, "enable device code authorization")
}

// loopbackClient sends every request to the test server, whatever host it names.
// A flow keeps the production endpoints it pins, and nothing leaves the machine.
func loopbackClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := server.Client()
	client.Transport = loopbackTransport{target: target, base: client.Transport}
	return client
}

type loopbackTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (l loopbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = l.target.Scheme
	clone.URL.Host = l.target.Host
	clone.Host = ""
	return l.base.RoundTrip(clone)
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
	manager.credentials["openai"] = credential{
		Type: "oauth", Access: "expired", Refresh: "refresh-old",
		Expires:   time.Now().Add(-time.Minute).UnixMilli(),
		AccountID: "acct_old",
		BaseURL:   "https://chatgpt.com/backend-api/codex",
		Protocol:  llm.ProtocolOpenAIResponses,
	}

	auth := &oauthAuthenticator{manager: manager, providerID: "openai"}
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

// browserTestManager wires a manager to a stub issuer with an ephemeral callback
// port. OpenAI pins port 1455 in production; a test must not fight a real Codex
// login for it.
func browserTestManager(t *testing.T, server *httptest.Server) *Manager {
	t.Helper()
	dir := t.TempDir()
	manager, err := Open(Options{
		CachePath:       filepath.Join(dir, "providers.json"),
		CredentialsPath: filepath.Join(dir, "credentials.json"),
		HTTPClient:      loopbackClient(t, server),
	})
	require.NoError(t, err)
	manager.oauthIssuer = server.URL
	manager.callbackAddr = "127.0.0.1:0"
	return manager
}

func fireCallback(t *testing.T, flow BrowserAuthorization, query url.Values) int {
	t.Helper()
	req, err := http.NewRequestWithContext(
		t.Context(), http.MethodGet, flow.pending.redirectURI+"?"+query.Encode(), http.NoBody,
	)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode
}

func authorizationQuery(t *testing.T, flow BrowserAuthorization) url.Values {
	t.Helper()
	parsed, err := url.Parse(flow.AuthorizationURL)
	require.NoError(t, err)
	require.Equal(t, "/oauth/authorize", parsed.Path)
	return parsed.Query()
}

func TestBrowserAuthorizationURLCarriesPKCEChallengeAndState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	manager := browserTestManager(t, server)

	flow, err := manager.BeginBrowserAuthorization(t.Context(), "openai")
	require.NoError(t, err)
	t.Cleanup(flow.pending.close)

	query := authorizationQuery(t, flow)
	assert.Equal(t, "code", query.Get("response_type"))
	assert.Equal(t, codexClientID, query.Get("client_id"))
	assert.Equal(t, "openid profile email offline_access", query.Get("scope"))
	assert.Equal(t, "S256", query.Get("code_challenge_method"))
	assert.Equal(t, "cozyphi", query.Get("originator"))
	assert.NotEmpty(t, query.Get("state"))
	assert.Equal(t, flow.pending.redirectURI, query.Get("redirect_uri"))
	assert.True(t, strings.HasSuffix(query.Get("redirect_uri"), "/auth/callback"))

	challenge, err := base64.RawURLEncoding.DecodeString(query.Get("code_challenge"))
	require.NoError(t, err)
	assert.Len(t, challenge, sha256.Size, "the challenge is a SHA-256 digest, never the verifier itself")
	assert.NotContains(t, flow.AuthorizationURL, flow.pending.verifier)
}

func TestBrowserAuthorizationRejectsAMismatchedCallbackState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a forged callback must not reach the issuer: %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer server.Close()
	manager := browserTestManager(t, server)

	flow, err := manager.BeginBrowserAuthorization(t.Context(), "openai")
	require.NoError(t, err)

	status := fireCallback(t, flow, url.Values{"code": {"stolen"}, "state": {"forged"}})
	assert.Equal(t, http.StatusBadRequest, status)

	err = manager.CompleteBrowserAuthorization(t.Context(), flow)
	require.ErrorContains(t, err, "state mismatch")
	assert.Empty(t, manager.credentials)
	assert.NoFileExists(t, manager.credsPath)
}

func TestBrowserAuthorizationExchangesTheCodeAndPinsTheSubscription(t *testing.T) {
	t.Parallel()

	access := testJWT(t, map[string]any{"chatgpt_account_id": "acct_browser"})
	var tokenForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			require.NoError(t, r.ParseForm())
			tokenForm = r.Form
			_, _ = fmt.Fprintf(w,
				`{"access_token":%q,"id_token":%q,"refresh_token":"refresh-1","expires_in":3600}`, access, access)
		case "/backend-api/codex/models":
			_, _ = fmt.Fprint(w, `{"models":[
				{"slug":"gpt-5.5","display_name":"GPT-5.5","visibility":"list","context_window":400000}
			]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	manager := browserTestManager(t, server)

	flow, err := manager.BeginBrowserAuthorization(t.Context(), "openai")
	require.NoError(t, err)
	query := authorizationQuery(t, flow)

	status := fireCallback(t, flow, url.Values{"code": {"auth-code"}, "state": {query.Get("state")}})
	assert.Equal(t, http.StatusOK, status)
	require.NoError(t, manager.CompleteBrowserAuthorization(t.Context(), flow))

	assert.Equal(t, "authorization_code", tokenForm.Get("grant_type"))
	assert.Equal(t, "auth-code", tokenForm.Get("code"))
	assert.Equal(t, codexClientID, tokenForm.Get("client_id"))
	assert.Equal(t, flow.pending.redirectURI, tokenForm.Get("redirect_uri"))
	digest := sha256.Sum256([]byte(tokenForm.Get("code_verifier")))
	assert.Equal(t, query.Get("code_challenge"), base64.RawURLEncoding.EncodeToString(digest[:]),
		"the exchange must prove the verifier behind the challenge")

	stored := manager.credentials["openai"]
	assert.Equal(t, "oauth", stored.Type)
	assert.Equal(t, "https://chatgpt.com/backend-api/codex", stored.BaseURL)
	assert.Equal(t, llm.ProtocolOpenAIResponses, stored.Protocol)
	assert.Equal(t, "acct_browser", stored.AccountID)
	assert.Equal(t, []Model{{ID: "gpt-5.5", Name: "GPT-5.5", ContextWindow: 400000}}, stored.Models)
}

func TestBrowserAuthorizationCancelsAndReleasesTheCallbackPort(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	manager := browserTestManager(t, server)

	flow, err := manager.BeginBrowserAuthorization(t.Context(), "openai")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err = manager.CompleteBrowserAuthorization(ctx, flow)
	require.ErrorContains(t, err, "canceled")
	assert.Empty(t, manager.credentials)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, flow.pending.redirectURI, http.NoBody)
	require.NoError(t, err)
	//nolint:bodyclose // the request must fail; there is no body to close.
	_, err = http.DefaultClient.Do(req)
	require.Error(t, err, "a canceled sign-in must not leave the callback listener running")
}
