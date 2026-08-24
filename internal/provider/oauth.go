package provider

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pulseaiclub/phi/internal/llm"
)

const (
	defaultOAuthIssuer = "https://auth.openai.com"
	codexClientID      = "app_EMoamEEZ73f0CkXaXp7hrann"
	maxOAuthBodyBytes  = 64 * 1024
	oauthCallbackAddr  = "127.0.0.1:1455"
	oauthRedirectURI   = "http://localhost:1455/auth/callback"
	deviceFlowTimeout  = 15 * time.Minute
	refreshSkew        = 30 * time.Second
)

// BrowserAuthorization is a pending authorization-code flow. Secrets and the
// callback server stay inside the provider package.
type BrowserAuthorization struct {
	ProviderID       string
	AuthorizationURL string
	pending          *browserAuthorizationState
}

type browserAuthorizationState struct {
	issuer      string
	redirectURI string
	verifier    string
	state       string
	server      *http.Server
	listener    net.Listener
	result      chan browserAuthorizationResult
	finishOnce  sync.Once
	closeOnce   sync.Once
}

type browserAuthorizationResult struct {
	code string
	err  error
}

// DeviceAuthorization is a pending user-code flow. Only display-safe fields are exported.
type DeviceAuthorization struct {
	ProviderID      string
	VerificationURL string
	UserCode        string
	deviceAuthID    string
	interval        time.Duration
	issuer          string
}

type deviceCodeResponse struct {
	DeviceAuthID string `json:"device_auth_id"`
	UserCode     string `json:"user_code"`
	Interval     string `json:"interval"`
}

type deviceTokenResponse struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeVerifier      string `json:"code_verifier"`
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type oauthHTTPError struct {
	statusCode int
}

func (e *oauthHTTPError) Error() string {
	return fmt.Sprintf("OAuth endpoint returned status %d", e.statusCode)
}

type oauthClaims struct {
	ChatGPTAccountID        string `json:"chatgpt_account_id"`
	ChatGPTComputeResidency string `json:"chatgpt_compute_residency"`
	Organizations           []struct {
		ID string `json:"id"`
	} `json:"organizations"`
	OpenAIAuth struct {
		ChatGPTAccountID        string `json:"chatgpt_account_id"`
		ChatGPTComputeResidency string `json:"chatgpt_compute_residency"`
	} `json:"https://api.openai.com/auth"`
}

// BeginBrowserAuthorization starts a loopback OAuth authorization-code flow
// with PKCE. The listener is active before the returned URL can be opened.
func (m *Manager) BeginBrowserAuthorization(ctx context.Context, providerID string) (BrowserAuthorization, error) {
	if m == nil {
		return BrowserAuthorization{}, errors.New("provider: manager is nil")
	}
	id := strings.TrimSpace(providerID)
	m.mu.RLock()
	item, ok := m.providers[id]
	issuer := m.oauthIssuer
	m.mu.RUnlock()
	if !ok || item.Auth != AuthOAuthBrowser {
		return BrowserAuthorization{}, fmt.Errorf("provider: %q does not support browser subscription sign-in", id)
	}

	verifier, err := randomOAuthValue()
	if err != nil {
		return BrowserAuthorization{}, fmt.Errorf("provider: generate PKCE verifier: %w", err)
	}
	state, err := randomOAuthValue()
	if err != nil {
		return BrowserAuthorization{}, fmt.Errorf("provider: generate OAuth state: %w", err)
	}
	challengeDigest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeDigest[:])

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp4", oauthCallbackAddr)
	if err != nil {
		return BrowserAuthorization{}, fmt.Errorf(
			"provider: start browser sign-in callback on %s: %w; close the process using port 1455 and retry",
			oauthCallbackAddr,
			err,
		)
	}
	pending := &browserAuthorizationState{
		issuer: issuer, redirectURI: oauthRedirectURI, verifier: verifier, state: state,
		listener: listener, result: make(chan browserAuthorizationResult, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", pending.handleCallback)
	pending.server = &http.Server{
		Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	go func() {
		if serveErr := pending.server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			pending.finish(browserAuthorizationResult{err: fmt.Errorf("OAuth callback server failed: %w", serveErr)})
		}
	}()

	params := url.Values{
		"response_type":              {"code"},
		"client_id":                  {codexClientID},
		"redirect_uri":               {oauthRedirectURI},
		"scope":                      {"openid profile email offline_access"},
		"code_challenge":             {challenge},
		"code_challenge_method":      {"S256"},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
		"state":                      {state},
		"originator":                 {"cozyphi"},
	}
	return BrowserAuthorization{
		ProviderID: id, AuthorizationURL: issuer + "/oauth/authorize?" + params.Encode(), pending: pending,
	}, nil
}

// CompleteBrowserAuthorization waits for the verified loopback callback,
// exchanges its one-time code, persists the credential, and always closes the listener.
func (m *Manager) CompleteBrowserAuthorization(ctx context.Context, flow BrowserAuthorization) error {
	if m == nil {
		return errors.New("provider: manager is nil")
	}
	if flow.ProviderID == "" || flow.pending == nil || flow.pending.result == nil {
		return errors.New("provider: invalid browser authorization")
	}
	defer flow.pending.close()

	ctx, cancel := context.WithTimeout(ctx, deviceFlowTimeout)
	defer cancel()
	select {
	case result := <-flow.pending.result:
		if result.err != nil {
			return fmt.Errorf("provider: browser subscription sign-in: %w", result.err)
		}
		form := url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {result.code},
			"redirect_uri":  {flow.pending.redirectURI},
			"client_id":     {codexClientID},
			"code_verifier": {flow.pending.verifier},
		}
		token, err := m.requestToken(ctx, flow.pending.issuer, form, true)
		if err != nil {
			return err
		}
		if err := m.saveOAuthCredential(flow.ProviderID, token); err != nil {
			return err
		}
		if flow.ProviderID == "codex" {
			if err := m.refreshCodexModels(ctx, true); err != nil {
				return &ModelCatalogWarning{err: err}
			}
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("provider: browser subscription sign-in canceled or expired: %w", ctx.Err())
	}
}

func randomOAuthValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (p *browserAuthorizationState) handleCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, oauthCallbackPage("Method not allowed", false))
		return
	}

	query := r.URL.Query()
	if callbackErr := strings.TrimSpace(query.Get("error_description")); callbackErr != "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, oauthCallbackPage("Authorization failed", false))
		p.finish(browserAuthorizationResult{err: errors.New(truncateOAuthError(callbackErr))})
		return
	}
	if callbackErr := strings.TrimSpace(query.Get("error")); callbackErr != "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, oauthCallbackPage("Authorization failed", false))
		p.finish(browserAuthorizationResult{err: errors.New(truncateOAuthError(callbackErr))})
		return
	}
	code := strings.TrimSpace(query.Get("code"))
	state := query.Get("state")
	if code == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, oauthCallbackPage("Missing authorization code", false))
		p.finish(browserAuthorizationResult{err: errors.New("authorization callback did not include a code")})
		return
	}
	if subtle.ConstantTimeCompare([]byte(state), []byte(p.state)) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, oauthCallbackPage("Invalid authorization state", false))
		p.finish(browserAuthorizationResult{err: errors.New("authorization callback state mismatch")})
		return
	}
	_, _ = io.WriteString(w, oauthCallbackPage("Authorization complete. You can return to CozyPhi.", true))
	p.finish(browserAuthorizationResult{code: code})
}

func (p *browserAuthorizationState) finish(result browserAuthorizationResult) {
	p.finishOnce.Do(func() { p.result <- result })
}

func (p *browserAuthorizationState) close() {
	p.closeOnce.Do(func() { _ = p.server.Close() })
}

func truncateOAuthError(message string) string {
	const limit = 512
	if len(message) <= limit {
		return message
	}
	return message[:limit] + "…"
}

func oauthCallbackPage(message string, success bool) string {
	color := "#d25f5f"
	if success {
		color = "#58b978"
	}
	return "<!doctype html><meta charset=utf-8><title>CozyPhi authorization</title>" +
		"<body style='font:16px system-ui;background:#181818;color:#ddd;padding:3rem'>" +
		"<h1 style='color:" + color + "'>CozyPhi</h1><p>" + message + "</p></body>"
}

// BeginDeviceAuthorization starts subscription sign-in without blocking for user interaction.
func (m *Manager) BeginDeviceAuthorization(ctx context.Context, providerID string) (DeviceAuthorization, error) {
	if m == nil {
		return DeviceAuthorization{}, errors.New("provider: manager is nil")
	}
	id := strings.TrimSpace(providerID)
	m.mu.RLock()
	item, ok := m.providers[id]
	issuer := m.oauthIssuer
	m.mu.RUnlock()
	if !ok || (item.Auth != AuthOAuthDevice && item.Auth != AuthOAuthBrowser) {
		return DeviceAuthorization{}, fmt.Errorf("provider: %q does not support subscription sign-in", id)
	}

	payload, err := json.Marshal(map[string]string{"client_id": codexClientID})
	if err != nil {
		return DeviceAuthorization{}, fmt.Errorf("provider: encode device authorization: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		issuer+"/api/accounts/deviceauth/usercode", bytes.NewReader(payload))
	if err != nil {
		return DeviceAuthorization{}, fmt.Errorf("provider: create device authorization: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "cozyphi")

	var response deviceCodeResponse
	if err := m.doOAuthJSON(req, &response); err != nil {
		var statusErr *oauthHTTPError
		if errors.As(err, &statusErr) && statusErr.statusCode == http.StatusNotFound {
			return DeviceAuthorization{}, errors.New(
				"provider: begin subscription sign-in: enable device code authorization in ChatGPT security settings",
			)
		}
		return DeviceAuthorization{}, fmt.Errorf("provider: begin subscription sign-in: %w", err)
	}
	interval, err := time.ParseDuration(response.Interval + "s")
	if err != nil || interval < time.Second {
		interval = 5 * time.Second
	}
	if response.DeviceAuthID == "" || response.UserCode == "" {
		return DeviceAuthorization{}, errors.New("provider: subscription sign-in returned an incomplete device code")
	}
	return DeviceAuthorization{
		ProviderID: id, VerificationURL: issuer + "/codex/device", UserCode: response.UserCode,
		deviceAuthID: response.DeviceAuthID, interval: interval, issuer: issuer,
	}, nil
}

// CompleteDeviceAuthorization polls until authorization, cancellation, or timeout.
func (m *Manager) CompleteDeviceAuthorization(ctx context.Context, flow DeviceAuthorization) error {
	if m == nil {
		return errors.New("provider: manager is nil")
	}
	if flow.ProviderID == "" || flow.deviceAuthID == "" || flow.UserCode == "" || flow.issuer == "" {
		return errors.New("provider: invalid device authorization")
	}
	ctx, cancel := context.WithTimeout(ctx, deviceFlowTimeout)
	defer cancel()
	for {
		token, pending, err := m.pollDeviceAuthorization(ctx, flow)
		if err != nil {
			return err
		}
		if !pending {
			return m.saveOAuthCredential(flow.ProviderID, token)
		}
		timer := time.NewTimer(flow.interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("provider: subscription sign-in canceled or expired: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func (m *Manager) pollDeviceAuthorization(
	ctx context.Context,
	flow DeviceAuthorization,
) (oauthTokenResponse, bool, error) {
	payload, err := json.Marshal(map[string]string{
		"device_auth_id": flow.deviceAuthID,
		"user_code":      flow.UserCode,
	})
	if err != nil {
		return oauthTokenResponse{}, false, fmt.Errorf("provider: encode device token request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		flow.issuer+"/api/accounts/deviceauth/token", bytes.NewReader(payload))
	if err != nil {
		return oauthTokenResponse{}, false, fmt.Errorf("provider: create device token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "cozyphi")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return oauthTokenResponse{}, false, fmt.Errorf("provider: poll subscription sign-in: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return oauthTokenResponse{}, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return oauthTokenResponse{}, false,
			fmt.Errorf("provider: subscription sign-in failed with status %d", resp.StatusCode)
	}
	var code deviceTokenResponse
	if err := decodeOAuthBody(resp.Body, &code); err != nil {
		return oauthTokenResponse{}, false, fmt.Errorf("provider: decode device token: %w", err)
	}
	if code.AuthorizationCode == "" || code.CodeVerifier == "" {
		return oauthTokenResponse{}, false, errors.New("provider: device token response is incomplete")
	}
	token, err := m.exchangeAuthorizationCode(ctx, flow.issuer, code)
	return token, false, err
}

func (m *Manager) exchangeAuthorizationCode(
	ctx context.Context,
	issuer string,
	code deviceTokenResponse,
) (oauthTokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code.AuthorizationCode},
		"redirect_uri":  {issuer + "/deviceauth/callback"},
		"client_id":     {codexClientID},
		"code_verifier": {code.CodeVerifier},
	}
	return m.requestToken(ctx, issuer, form, true)
}

func (m *Manager) requestToken(
	ctx context.Context,
	issuer string,
	form url.Values,
	requireRefresh bool,
) (oauthTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		issuer+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokenResponse{}, fmt.Errorf("provider: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var token oauthTokenResponse
	if err := m.doOAuthJSON(req, &token); err != nil {
		return oauthTokenResponse{}, fmt.Errorf("provider: exchange subscription token: %w", err)
	}
	if token.AccessToken == "" || (requireRefresh && token.RefreshToken == "") {
		return oauthTokenResponse{}, errors.New("provider: token response is incomplete")
	}
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = 3600
	}
	return token, nil
}

func (m *Manager) doOAuthJSON(req *http.Request, target any) error {
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return &oauthHTTPError{statusCode: resp.StatusCode}
	}
	return decodeOAuthBody(resp.Body, target)
}

func decodeOAuthBody(body io.Reader, target any) error {
	data, err := io.ReadAll(io.LimitReader(body, maxOAuthBodyBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxOAuthBodyBytes {
		return errors.New("OAuth response is too large")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("invalid OAuth JSON: %w", err)
	}
	return nil
}

func (m *Manager) saveOAuthCredential(providerID string, token oauthTokenResponse) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.providers[providerID]
	if !ok || (item.Auth != AuthOAuthDevice && item.Auth != AuthOAuthBrowser) {
		return fmt.Errorf("provider: OAuth provider %q is unavailable", providerID)
	}
	previous := m.credentials[providerID]
	next := cloneCredentials(m.credentials)
	accountID := extractAccountID(token)
	if accountID == "" {
		accountID = previous.AccountID
	}
	updated := credential{
		Type: "oauth", Access: token.AccessToken, Refresh: token.RefreshToken,
		Expires:   time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UnixMilli(),
		AccountID: accountID, BaseURL: item.BaseURL, Protocol: item.Protocol,
	}
	if accountID != "" && accountID == previous.AccountID {
		updated.Models = append([]Model(nil), previous.Models...)
		updated.ModelsFetchedAt = previous.ModelsFetchedAt
		updated.ModelsClientVersion = previous.ModelsClientVersion
	}
	next[providerID] = updated
	if err := writeCredentials(m.credsPath, next); err != nil {
		return fmt.Errorf("provider: save subscription credential for %q: %w", providerID, err)
	}
	m.credentials = next
	if providerID == "codex" && accountID != previous.AccountID {
		fallback := builtinProviders()["codex"]
		item.Models = append([]Model(nil), fallback.Models...)
		m.providers[providerID] = item
	}
	return nil
}

type oauthAuthenticator struct {
	manager    *Manager
	providerID string
}

func (a *oauthAuthenticator) Authorize(ctx context.Context, req *http.Request) error {
	if a == nil || a.manager == nil {
		return errors.New("provider: OAuth authenticator is unavailable")
	}
	credential, err := a.manager.validOAuthCredential(ctx, a.providerID)
	if err != nil {
		return err
	}
	return authorizeOAuthRequest(req, credential)
}

func authorizeOAuthRequest(req *http.Request, credential credential) error {
	if !requestWithinBaseURL(req, credential.BaseURL) {
		return errors.New("provider: OAuth request target does not match the connected endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+credential.Access)
	if credential.AccountID != "" {
		req.Header.Set("ChatGPT-Account-Id", credential.AccountID)
	}
	if residency := extractResidency(credential.Access); residency != "" {
		req.Header.Set("x-openai-internal-codex-residency", residency)
	}
	req.Header.Set("originator", "cozyphi")
	return nil
}

func requestWithinBaseURL(req *http.Request, baseURL string) bool {
	if req == nil || req.URL == nil || req.URL.User != nil {
		return false
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" || base.User != nil {
		return false
	}
	if !strings.EqualFold(req.URL.Scheme, base.Scheme) || !strings.EqualFold(req.URL.Host, base.Host) {
		return false
	}
	basePath := strings.TrimRight(base.EscapedPath(), "/")
	requestPath := strings.TrimRight(req.URL.EscapedPath(), "/")
	return basePath == "" || requestPath == basePath || strings.HasPrefix(requestPath, basePath+"/")
}

func (m *Manager) validOAuthCredential(ctx context.Context, providerID string) (credential, error) {
	m.authMu.Lock()
	defer m.authMu.Unlock()
	m.mu.RLock()
	current, ok := m.credentials[providerID]
	item, providerExists := m.providers[providerID]
	issuer := m.oauthIssuer
	m.mu.RUnlock()
	if !ok || current.Type != "oauth" {
		return credential{}, fmt.Errorf("provider: %q is not signed in", providerID)
	}
	if !providerExists || current.BaseURL != item.BaseURL || current.Protocol != item.Protocol {
		return credential{}, fmt.Errorf(
			"provider: stored connection contract for %q is invalid; reconnect the provider",
			providerID,
		)
	}
	if time.Now().Add(refreshSkew).UnixMilli() < current.Expires {
		return current, nil
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {current.Refresh},
		"client_id":     {codexClientID},
	}
	token, err := m.requestToken(ctx, issuer, form, false)
	if err != nil {
		return credential{}, fmt.Errorf("provider: refresh subscription credential: %w", err)
	}
	if token.RefreshToken == "" {
		token.RefreshToken = current.Refresh
	}
	if err := m.saveOAuthCredential(providerID, token); err != nil {
		return credential{}, err
	}
	m.mu.RLock()
	refreshed := m.credentials[providerID]
	m.mu.RUnlock()
	return refreshed, nil
}

func extractAccountID(token oauthTokenResponse) string {
	for _, raw := range []string{token.IDToken, token.AccessToken} {
		claims, ok := parseJWTClaims(raw)
		if !ok {
			continue
		}
		if claims.ChatGPTAccountID != "" {
			return claims.ChatGPTAccountID
		}
		if claims.OpenAIAuth.ChatGPTAccountID != "" {
			return claims.OpenAIAuth.ChatGPTAccountID
		}
		if len(claims.Organizations) > 0 {
			return claims.Organizations[0].ID
		}
	}
	return ""
}

func extractResidency(token string) string {
	claims, ok := parseJWTClaims(token)
	if !ok {
		return ""
	}
	residency := claims.OpenAIAuth.ChatGPTComputeResidency
	if residency == "" {
		residency = claims.ChatGPTComputeResidency
	}
	if residency == "no_constraint" {
		return ""
	}
	return residency
}

func parseJWTClaims(token string) (oauthClaims, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(parts[1]) > maxOAuthBodyBytes {
		return oauthClaims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(payload) > maxOAuthBodyBytes {
		return oauthClaims{}, false
	}
	var claims oauthClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return oauthClaims{}, false
	}
	return claims, true
}

var _ llm.RequestAuthenticator = (*oauthAuthenticator)(nil)
