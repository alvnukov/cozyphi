package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func resetTestManager(t *testing.T, consume http.HandlerFunc) *Manager {
	t.Helper()
	m := newOpenAIQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case openAICodexUsagePath:
			_, _ = io.WriteString(w, `{"rate_limit_reset_credits":{"available_count":2}}`)
		case openAICodexProfilePath:
			_, _ = io.WriteString(w, `{}`)
		default:
			consume(w, r)
		}
	}))
	m.credsPath = filepath.Join(t.TempDir(), "credentials.json")
	m.oauthIssuer = strings.TrimSuffix(m.credentials[openaiProviderID].BaseURL, "/backend-api/codex")
	return m
}

func resetTarget(t *testing.T, m *Manager) *QuotaResetTarget {
	t.Helper()
	snapshot, err := m.QuotaSnapshot(t.Context(), openaiProviderID)
	require.NoError(t, err)
	require.NotNil(t, snapshot.ResetTarget)
	require.True(t, m.QuotaResetTargetValid(snapshot.ResetTarget))
	return snapshot.ResetTarget
}

func TestQuotaResetWireContract(t *testing.T) {
	var requests []map[string]json.RawMessage
	m := resetTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.RequestURI() != openAICodexResetPath ||
			r.Header.Get("Authorization") != "Bearer access-token" ||
			r.Header.Get("ChatGPT-Account-Id") != "acct_123" ||
			r.Header.Get("Content-Type") != "application/json" ||
			r.Header.Get("Accept") != "application/json" || r.Header.Get("originator") != "cozyphi" {
			t.Error("consume wire contract mismatch")
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("consume body is not JSON")
		}
		requests = append(requests, body)
		_, _ = io.WriteString(w, `{"code":"reset","windows_reset":2,"credit":{"id":"ignored-secret"}}`)
	})
	for range 2 {
		target := resetTarget(t, m)
		result, err := m.ResetQuota(t.Context(), target)
		require.NoError(t, err)
		require.Equal(t, QuotaResetResult{Code: QuotaResetDone, WindowsReset: 2}, result)
		require.False(t, m.QuotaResetTargetValid(target))
		_, err = m.ResetQuota(t.Context(), target)
		require.ErrorIs(t, err, ErrQuotaResetStale)
	}
	require.Len(t, requests, 2)
	var nonces []string
	for _, body := range requests {
		require.Len(t, body, 1, "credit_id must be omitted")
		var nonce string
		require.NoError(t, json.Unmarshal(body["redeem_request_id"], &nonce))
		require.NotEmpty(t, nonce)
		nonces = append(nonces, nonce)
	}
	require.NotEqual(t, nonces[0], nonces[1], "new confirmation gets a fresh redemption ID")
}

func TestQuotaResetResponseCodes(t *testing.T) {
	for _, code := range []QuotaResetCode{QuotaResetNothing, QuotaResetNoCredit, QuotaResetAlreadyRedeemed} {
		t.Run(string(code), func(t *testing.T) {
			m := resetTestManager(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprintf(w, `{"code":%q}`, code)
			})
			result, err := m.ResetQuota(t.Context(), resetTarget(t, m))
			require.NoError(t, err)
			require.Equal(t, QuotaResetResult{Code: code}, result)
		})
	}
}

func TestQuotaResetUnknownOutcomesAreOneShotAndSafe(t *testing.T) {
	for name, body := range map[string]string{
		"malformed":    `{"code":"reset",`,
		"unknown code": `{"code":"access-token acct_123 refresh-token"}`,
		"missing code": `{}`,
		"null":         `null`,
		"null windows": `{"code":"reset","windows_reset":null}`,
		"negative":     `{"code":"reset","windows_reset":-1}`,
		"wrong type":   `{"code":"reset","windows_reset":"access-token"}`,
		"fraction":     `{"code":"reset","windows_reset":1.5}`,
		"overflow":     `{"code":"reset","windows_reset":9223372036854775808}`,
		"trailing":     `{"code":"reset"}{}`,
		"oversize":     `{"code":"reset","unused":"` + strings.Repeat("x", maxQuotaBytes) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var sends atomic.Int32
			m := resetTestManager(t, func(w http.ResponseWriter, _ *http.Request) {
				sends.Add(1)
				_, _ = io.WriteString(w, body)
			})
			target := resetTarget(t, m)
			result, err := m.ResetQuota(t.Context(), target)
			require.ErrorIs(t, err, ErrQuotaResetUnknown)
			require.Empty(t, result)
			assertResetSafe(t, err, target)
			_, err = m.ResetQuota(t.Context(), target)
			require.ErrorIs(t, err, ErrQuotaResetStale)
			require.EqualValues(t, 1, sends.Load())
		})
	}
}

func assertResetSafe(t *testing.T, err error, target *QuotaResetTarget) {
	t.Helper()
	//nolint:staticcheck // SA9005: this test pins the absence of serializable capability state.
	encoded, marshalErr := json.Marshal(target)
	require.NoError(t, marshalErr)
	printed := fmt.Sprintf("%v %+v %#v %s %s", target, target, target, encoded, err)
	for _, secret := range []string{"access-token", "refresh-token", "acct_123", "ignored-secret"} {
		require.NotContains(t, printed, secret)
	}
}

func TestQuotaResetRedirectAndHTTPFailures(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308, 400, 401, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var reached atomic.Int32
			destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				reached.Add(1)
			}))
			defer destination.Close()
			var sends atomic.Int32
			m := resetTestManager(t, func(w http.ResponseWriter, _ *http.Request) {
				sends.Add(1)
				w.Header().Set("Location", destination.URL+"/access-token/acct_123")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "access-token refresh-token acct_123")
			})
			target := resetTarget(t, m)
			_, err := m.ResetQuota(t.Context(), target)
			require.ErrorIs(t, err, ErrQuotaResetUnknown)
			assertResetSafe(t, err, target)
			require.Zero(t, reached.Load())
			require.EqualValues(t, 1, sends.Load())
		})
	}
}

func TestQuotaResetAvailability(t *testing.T) {
	for _, summary := range []string{
		`{}`, `null`, `{"available_count":null}`, `{"available_count":0}`,
		`{"available_count":-1}`, `{"available_count":"2"}`,
	} {
		t.Run(summary, func(t *testing.T) {
			m := codexFixtureManager(t, `{"rate_limit_reset_credits":`+summary+`}`, `{}`, http.StatusOK)
			snapshot, err := m.QuotaSnapshot(t.Context(), openaiProviderID)
			require.NoError(t, err)
			require.Nil(t, snapshot.ResetTarget)
			require.False(t, m.QuotaResetTargetValid(snapshot.ResetTarget))
		})
	}
	m := resetTestManager(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected reset") })
	cred := m.credentials[openaiProviderID]
	cred.AccountID = ""
	m.credentials[openaiProviderID] = cred
	snapshot, err := m.QuotaSnapshot(t.Context(), openaiProviderID)
	require.NoError(t, err)
	require.Nil(t, snapshot.ResetTarget)
}

func TestQuotaResetDuplicatePanesAndInflightSnapshots(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	m := resetTestManager(t, func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-finish
		_, _ = io.WriteString(w, `{"code":"reset"}`)
	})
	first, second := resetTarget(t, m), resetTarget(t, m)
	result := make(chan error, 1)
	go func() { _, err := m.ResetQuota(t.Context(), first); result <- err }()
	<-started
	require.False(t, m.QuotaResetTargetValid(second))
	_, err := m.ResetQuota(t.Context(), second)
	require.ErrorIs(t, err, ErrQuotaResetStale)
	snapshot, err := m.QuotaSnapshot(t.Context(), openaiProviderID)
	require.NoError(t, err)
	require.Nil(t, snapshot.ResetTarget)
	close(finish)
	require.NoError(t, <-result)
	_, err = m.ResetQuota(t.Context(), second)
	require.ErrorIs(t, err, ErrQuotaResetStale)
}

func TestQuotaResetCancellation(t *testing.T) {
	t.Run("before send", func(t *testing.T) {
		m := resetTestManager(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
		target := resetTarget(t, m)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := m.ResetQuota(ctx, target)
		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, ErrQuotaResetUnknown)
		require.False(t, m.QuotaResetTargetValid(target))
	})
	t.Run("after send", func(t *testing.T) {
		started := make(chan struct{})
		m := resetTestManager(t, func(_ http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			close(started)
			<-r.Context().Done()
		})
		target := resetTarget(t, m)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		result := make(chan error, 1)
		go func() { _, err := m.ResetQuota(ctx, target); result <- err }()
		<-started
		cancel()
		require.ErrorIs(t, <-result, ErrQuotaResetUnknown)
	})
	t.Run("client timeout", func(t *testing.T) {
		m := resetTestManager(t, func(_ http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			<-r.Context().Done()
		})
		target := resetTarget(t, m)
		m.httpClient.Timeout = 20 * time.Millisecond
		_, err := m.ResetQuota(t.Context(), target)
		require.ErrorIs(t, err, ErrQuotaResetUnknown)
	})
	t.Run("waiting for refresh", func(t *testing.T) {
		m := resetTestManager(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
		target := resetTarget(t, m)
		m.authGate <- struct{}{}
		defer func() { <-m.authGate }()
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		_, err := m.ResetQuota(ctx, target)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.NotErrorIs(t, err, ErrQuotaResetUnknown)
	})
}

func TestQuotaResetStaleAccountsAndManager(t *testing.T) {
	m := resetTestManager(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
	other := resetTestManager(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected send") })
	target := resetTarget(t, m)
	for _, invalid := range []*QuotaResetTarget{nil, {}, resetTarget(t, other)} {
		require.False(t, m.QuotaResetTargetValid(invalid))
		_, err := m.ResetQuota(t.Context(), invalid)
		require.ErrorIs(t, err, ErrQuotaResetStale)
	}
	for _, account := range []string{"account-B", "acct_123"} {
		token := oauthTokenResponse{
			AccessToken: "access-token", RefreshToken: "refresh-token", ExpiresIn: 3600,
			IDToken: testJWT(t, map[string]any{"chatgpt_account_id": account}),
		}
		require.NoError(t, m.saveOAuthCredential(openaiProviderID, AuthOAuthBrowser, token))
	}
	require.False(t, m.QuotaResetTargetValid(target), "A → B → A cannot resurrect an intent")
	_, err := m.ResetQuota(t.Context(), target)
	require.ErrorIs(t, err, ErrQuotaResetStale)
}

func TestQuotaResetRefreshPinsConfirmedAccount(t *testing.T) {
	for _, account := range []string{"acct_123", "account-B"} {
		t.Run(account, func(t *testing.T) {
			var sends atomic.Int32
			m := resetTestManager(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth/token" {
					_ = json.NewEncoder(w).Encode(oauthTokenResponse{
						AccessToken: "fresh-access", RefreshToken: "fresh-refresh", ExpiresIn: 3600,
						IDToken: testJWT(t, map[string]any{"chatgpt_account_id": account}),
					})
					return
				}
				sends.Add(1)
				if r.Header.Get("Authorization") != "Bearer fresh-access" ||
					r.Header.Get("ChatGPT-Account-Id") != "acct_123" {
					t.Error("reset did not pin confirmed account with refreshed token")
				}
				_, _ = io.WriteString(w, `{"code":"reset"}`)
			})
			target := resetTarget(t, m)
			cred := m.credentials[openaiProviderID]
			cred.Expires = 1
			m.credentials[openaiProviderID] = cred
			_, err := m.ResetQuota(t.Context(), target)
			if account == "acct_123" {
				require.NoError(t, err)
				require.EqualValues(t, 1, sends.Load())
			} else {
				require.ErrorIs(t, err, ErrQuotaResetStale)
				require.Zero(t, sends.Load())
			}
		})
	}
}

func TestQuotaResetExactAuthorizationBoundary(t *testing.T) {
	cred := credential{Type: "oauth", Access: "access-token", AccountID: "acct_123", BaseURL: chatgptCodexBaseURL}
	const endpoint = "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits/consume"
	for _, target := range []string{
		endpoint + "?credit_id=secret", endpoint + "?", endpoint + "#fragment", endpoint + "/",
		strings.Replace(endpoint, "consume", "%63onsume", 1),
		strings.Replace(endpoint, "https:", "http:", 1),
		strings.Replace(endpoint, "chatgpt.com", "other.test", 1),
		"https://chatgpt.com/backend-api/wham/usage",
	} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, target, http.NoBody)
		require.NoError(t, err)
		require.ErrorIs(t, authorizeOpenAICodexResetRequest(req, cred), ErrQuotaResetStale)
		require.Empty(t, req.Header.Get("Authorization"))
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPost} {
		req, err := http.NewRequestWithContext(t.Context(), method, endpoint, http.NoBody)
		require.NoError(t, err)
		err = authorizeOpenAICodexResetRequest(req, cred)
		if method == http.MethodPost {
			require.NoError(t, err)
			require.Equal(t, "Bearer access-token", req.Header.Get("Authorization"))
		} else {
			require.ErrorIs(t, err, ErrQuotaResetStale)
			require.Empty(t, req.Header.Get("Authorization"))
		}
	}
}

func TestQuotaResetRefreshFailuresAreSafeAndNeverConsume(t *testing.T) {
	for name, response := range map[string]string{
		"malformed": `{"access_token":"access-token","expires_in":"acct_123"}`,
		"oversize":  strings.Repeat("access-token", maxOAuthBodyBytes),
		"redirect":  "",
	} {
		t.Run(name, func(t *testing.T) {
			var reached, refreshes atomic.Int32
			destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				reached.Add(1)
			}))
			defer destination.Close()
			m := resetTestManager(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/oauth/token" {
					t.Error("refresh failure must not send a reset")
					return
				}
				refreshes.Add(1)
				if name == "redirect" {
					http.Redirect(w, r, destination.URL+"/access-token", http.StatusTemporaryRedirect)
					return
				}
				_, _ = io.WriteString(w, response)
			})
			target := resetTarget(t, m)
			cred := m.credentials[openaiProviderID]
			cred.Expires = 1
			m.credentials[openaiProviderID] = cred
			_, err := m.ResetQuota(t.Context(), target)
			require.Error(t, err)
			require.NotErrorIs(t, err, ErrQuotaResetUnknown)
			assertResetSafe(t, err, target)
			_, err = m.ResetQuota(t.Context(), target)
			require.ErrorIs(t, err, ErrQuotaResetStale)
			require.EqualValues(t, 1, refreshes.Load())
			require.Zero(t, reached.Load())
		})
	}
}

func TestQuotaResetConcurrentSignInCannotBeOverwrittenByRefresh(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	m := resetTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Error("stale refresh must not send a reset")
			return
		}
		close(started)
		<-finish
		_, _ = io.WriteString(w, `{"access_token":"late-token","expires_in":3600}`)
	})
	target := resetTarget(t, m)
	cred := m.credentials[openaiProviderID]
	cred.Expires = 1
	m.credentials[openaiProviderID] = cred
	result := make(chan error, 1)
	go func() { _, err := m.ResetQuota(t.Context(), target); result <- err }()
	<-started
	for _, account := range []string{"account-B", "acct_123"} {
		require.NoError(t, m.saveOAuthCredential(openaiProviderID, AuthOAuthBrowser, oauthTokenResponse{
			AccessToken: "new-signin-token", RefreshToken: "new-refresh", ExpiresIn: 3600,
			IDToken: testJWT(t, map[string]any{"chatgpt_account_id": account}),
		}))
	}
	close(finish)
	require.ErrorIs(t, <-result, ErrQuotaResetStale)
	require.Equal(t, "new-signin-token", m.credentials[openaiProviderID].Access)
}

func TestQuotaResetFetchStraddlingAttemptCannotIssueTarget(t *testing.T) {
	for _, during := range []bool{false, true} {
		t.Run(fmt.Sprint(during), func(t *testing.T) {
			var block atomic.Bool
			fetchStarted, finishFetch := make(chan struct{}), make(chan struct{})
			resetStarted, finishReset := make(chan struct{}), make(chan struct{})
			m := newOpenAIQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case openAICodexUsagePath:
					if block.Load() {
						close(fetchStarted)
						<-finishFetch
					}
					_, _ = io.WriteString(w, `{"rate_limit_reset_credits":{"available_count":2}}`)
				case openAICodexResetPath:
					close(resetStarted)
					<-finishReset
					_, _ = io.WriteString(w, `{"code":"reset"}`)
				default:
					_, _ = io.WriteString(w, `{}`)
				}
			}))
			target := resetTarget(t, m)
			block.Store(true)
			resetDone := make(chan error, 1)
			startReset := func() {
				go func() { _, err := m.ResetQuota(t.Context(), target); resetDone <- err }()
				<-resetStarted
			}
			if during {
				startReset()
			}
			snapshotDone := make(chan QuotaSnapshot, 1)
			fetchErr := make(chan error, 1)
			go func() {
				snapshot, err := m.QuotaSnapshot(t.Context(), openaiProviderID)
				snapshotDone <- snapshot
				fetchErr <- err
			}()
			<-fetchStarted
			if !during {
				startReset()
			}
			close(finishReset)
			require.NoError(t, <-resetDone)
			close(finishFetch)
			require.Nil(t, (<-snapshotDone).ResetTarget)
			require.NoError(t, <-fetchErr)
		})
	}
}

func TestQuotaResetTruncatedResponseIsUnknown(t *testing.T) {
	m := resetTestManager(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = io.WriteString(w, `{"code":"reset"}`)
	})
	_, err := m.ResetQuota(t.Context(), resetTarget(t, m))
	require.ErrorIs(t, err, ErrQuotaResetUnknown)
}
