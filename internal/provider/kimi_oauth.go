package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Endpoints, client id, and response shapes below are confirmed by the
// reference implementation t94j0/opencode-kimi-subscription, which mirrors the
// official kimi CLI (v0.26.0): an RFC 8628 device grant against auth.kimi.com.
const (
	defaultKimiIssuer = "https://auth.kimi.com"
	// kimiClientID is the public OAuth client the kimi CLI ships with.
	kimiClientID = "17e5f671-d194-4dfb-9706-5516cb48c098"
	// kimiDeviceGrant is the device_code grant type from RFC 8628 §3.5.
	kimiDeviceGrant = "urn:ietf:params:oauth:grant-type:device_code"
	// kimiSlowDownStep is the RFC 8628 §3.5 interval increase on slow_down.
	kimiSlowDownStep = 5 * time.Second
	// kimiDefaultInterval is the poll interval when the server names none; the
	// reference plugin uses the same default.
	kimiDefaultInterval = 5 * time.Second
	// kimiTokenLifetime is the access-token lifetime the API documents; used
	// when a token response omits expires_in.
	kimiTokenLifetime = 15 * time.Minute
	// kimiAccountID stands in for the account id Kimi's tokens do not carry.
	// It keys the account-bound model cache for "the one kimi subscription".
	kimiAccountID = "kimi"
)

type kimiDeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int64  `json:"interval"`
}

type kimiTokenErrorResponse struct {
	Error string `json:"error"`
}

// kimiGrant is Kimi's OAuth shape: one POST form token endpoint under
// /api/oauth, tokens returned directly by the poll (no code exchange), and
// plain Bearer authorization with no account headers.
type kimiGrant struct{}

func (kimiGrant) beginDevice(ctx context.Context, m *Manager) (DeviceAuthorization, error) {
	m.mu.RLock()
	issuer := m.kimiIssuer
	m.mu.RUnlock()

	req, err := kimiFormRequest(ctx, issuer+"/api/oauth/device_authorization",
		url.Values{"client_id": {kimiClientID}})
	if err != nil {
		return DeviceAuthorization{}, fmt.Errorf("provider: create device authorization: %w", err)
	}
	var response kimiDeviceCodeResponse
	if err := m.doOAuthJSON(req, &response); err != nil {
		return DeviceAuthorization{}, fmt.Errorf("provider: begin subscription sign-in: %w", err)
	}
	// RFC 8628 §3.2: verification_uri_complete saves the user the code entry,
	// so it wins over the plain URI when the server offers both.
	verification := response.VerificationURIComplete
	if verification == "" {
		verification = response.VerificationURI
	}
	if response.DeviceCode == "" || response.UserCode == "" || verification == "" {
		return DeviceAuthorization{}, errors.New("provider: subscription sign-in returned an incomplete device code")
	}
	interval := time.Duration(response.Interval) * time.Second
	if interval < time.Second {
		interval = kimiDefaultInterval
	}
	return DeviceAuthorization{
		ProviderID: kimiProviderID, VerificationURL: verification, UserCode: response.UserCode,
		deviceCode: response.DeviceCode, interval: interval, issuer: issuer,
	}, nil
}

func (kimiGrant) pollDevice(
	ctx context.Context,
	m *Manager,
	flow *DeviceAuthorization,
) (oauthTokenResponse, bool, error) {
	req, err := kimiFormRequest(ctx, flow.issuer+"/api/oauth/token", url.Values{
		"client_id":   {kimiClientID},
		"device_code": {flow.deviceCode},
		"grant_type":  {kimiDeviceGrant},
	})
	if err != nil {
		return oauthTokenResponse{}, false, fmt.Errorf("provider: create device token request: %w", err)
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return oauthTokenResponse{}, false, fmt.Errorf("provider: poll subscription sign-in: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		token, err := kimiDecodeToken(resp.Body, true)
		return token, false, err
	}
	// RFC 8628 §3.5: the error body drives the poll. Pending keeps waiting,
	// slow_down lengthens the interval, every other code is terminal.
	var failure kimiTokenErrorResponse
	if err := decodeOAuthBody(resp.Body, &failure); err != nil || failure.Error == "" {
		return oauthTokenResponse{}, false,
			fmt.Errorf("provider: subscription sign-in failed with status %d", resp.StatusCode)
	}
	switch failure.Error {
	case "authorization_pending":
		return oauthTokenResponse{}, true, nil
	case "slow_down":
		flow.interval += kimiSlowDownStep
		return oauthTokenResponse{}, true, nil
	case "access_denied":
		return oauthTokenResponse{}, false, errors.New("provider: subscription sign-in was denied")
	case "expired_token":
		return oauthTokenResponse{}, false, errors.New(
			"provider: subscription sign-in code expired; start the sign-in again",
		)
	default:
		return oauthTokenResponse{}, false,
			fmt.Errorf("provider: subscription sign-in failed: %s", truncateOAuthError(failure.Error))
	}
}

func (kimiGrant) refresh(ctx context.Context, m *Manager, current credential) (oauthTokenResponse, error) {
	m.mu.RLock()
	issuer := m.kimiIssuer
	m.mu.RUnlock()

	req, err := kimiFormRequest(ctx, issuer+"/api/oauth/token", url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {current.Refresh},
		"client_id":     {kimiClientID},
	})
	if err != nil {
		return oauthTokenResponse{}, fmt.Errorf("provider: create token request: %w", err)
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return oauthTokenResponse{}, fmt.Errorf("provider: refresh subscription credential: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return oauthTokenResponse{}, fmt.Errorf(
			"provider: refresh subscription credential: unexpected HTTP status %d", resp.StatusCode,
		)
	}
	// A refresh response may omit refresh_token; the caller keeps the stored one.
	return kimiDecodeToken(resp.Body, false)
}

func (kimiGrant) authorize(req *http.Request, current credential) error {
	if !requestWithinBaseURL(req, current.BaseURL) {
		return errors.New("provider: OAuth request target does not match the connected endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+current.Access)
	return nil
}

func kimiFormRequest(ctx context.Context, target string, form url.Values) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "cozyphi")
	return req, nil
}

// kimiDecodeToken applies the shared completeness rules to a token body: an
// access token is always required, a refresh token only on a full sign-in.
func kimiDecodeToken(body io.Reader, requireRefresh bool) (oauthTokenResponse, error) {
	var token oauthTokenResponse
	if err := decodeOAuthBody(body, &token); err != nil {
		return oauthTokenResponse{}, fmt.Errorf("provider: decode token response: %w", err)
	}
	if token.AccessToken == "" || (requireRefresh && token.RefreshToken == "") {
		return oauthTokenResponse{}, errors.New("provider: token response is incomplete")
	}
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = int64(kimiTokenLifetime.Seconds())
	}
	return token, nil
}
