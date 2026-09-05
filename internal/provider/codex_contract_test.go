package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// Contract from openai/codex 531f3836a1e38ea61eaaba3dccda6711eb6c0dca:
// codex-rs/backend-client/src/client/rate_limit_resets_tests.rs.
func TestCodexUsageUpstreamContract(t *testing.T) {
	m := newOpenAIQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			_, _ = w.Write(
				[]byte(
					`{"plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":25,"limit_window_seconds":18000,"reset_after_seconds":60,"reset_at":2000000000}}}`,
				),
			)
		case "/backend-api/wham/profiles/me":
			_, _ = w.Write([]byte(`{"stats":{"lifetime_tokens":12345}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	snapshot, err := m.QuotaSnapshot(t.Context(), "openai")
	require.NoError(t, err)
	require.Len(t, snapshot.Limits, 1)
	require.Equal(t, 25.0, snapshot.Limits[0].UsedPercent)
	require.Equal(t, "5 hours", snapshot.Limits[0].Window)
	require.EqualValues(t, 2000000000, snapshot.Limits[0].ResetsAt.Unix())
	require.Equal(t, []QuotaTokenUsage{{Scope: "Codex profile lifetime", Tokens: 12345}}, snapshot.Tokens)
}

func TestCodexMissingObservations(t *testing.T) {
	for _, usage := range []string{
		`{}`, `{"rate_limit":null}`, `{"rate_limit":{}}`,
		`{"rate_limit":{"primary_window":null,"secondary_window":null}}`,
		`{"rate_limit":{"primary_window":{},"secondary_window":{"used_percent":null}}}`,
		`{"rate_limits":[{"used_percent":20}],"usage":{"total_tokens":999}}`,
	} {
		t.Run(usage, func(t *testing.T) {
			m := codexFixtureManager(t, usage, `{"stats":null}`, http.StatusOK)
			snapshot, err := m.QuotaSnapshot(t.Context(), "openai")
			require.NoError(t, err)
			require.Empty(t, snapshot.Limits)
			require.Empty(t, snapshot.Tokens)
			require.False(t, snapshot.Reset.Supported)
		})
	}
}

func TestCodexProfileObservations(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []QuotaTokenUsage
	}{
		{"absent stats", `{}`, nil},
		{"null stats", `{"stats":null}`, nil},
		{"absent tokens", `{"stats":{}}`, nil},
		{"null tokens", `{"stats":{"lifetime_tokens":null,"daily_usage_buckets":null}}`, nil},
		{"zero lifetime", `{"stats":{"lifetime_tokens":0}}`, []QuotaTokenUsage{{Scope: "Codex profile lifetime", Tokens: 0}}},
		{"precise lifetime", `{"stats":{"lifetime_tokens":9007199254740993}}`, []QuotaTokenUsage{{Scope: "Codex profile lifetime", Tokens: 9007199254740993}}},
		{"daily observations", `{"stats":{"daily_usage_buckets":[{"start_date":"2026-09-04","tokens":0},{"start_date":"2026-09-05","tokens":12},{"start_date":"2026-09-06"},{"start_date":"2026-09-07","tokens":null},{"tokens":10}]}}`, []QuotaTokenUsage{
			{Scope: "Codex profile daily bucket 2026-09-04", Tokens: 0}, {Scope: "Codex profile daily bucket 2026-09-05", Tokens: 12},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := codexFixtureManager(t, `{}`, tc.body, http.StatusOK)
			snapshot, err := m.QuotaSnapshot(t.Context(), "openai")
			require.NoError(t, err)
			require.Equal(t, tc.want, snapshot.Tokens)
		})
	}
}

func TestCodexMultipleWindowsAndScopes(t *testing.T) {
	m := codexFixtureManager(t, `{"plan_type":"pro","rate_limit":{
  "primary_window":{"used_percent":0,"limit_window_seconds":18000,"reset_at":2000000000},
  "secondary_window":{"used_percent":90,"limit_window_seconds":604800,"reset_at":2000000300}},
  "additional_rate_limits":[{"limit_name":"Other quota","metered_feature":"other","rate_limit":{
   "primary_window":{"used_percent":70,"limit_window_seconds":900},
   "secondary_window":{"used_percent":100,"limit_window_seconds":86400}}},
   {"limit_name":"Unavailable","rate_limit":null}],
  "rate_limit_reset_credits":{"available_count":0}}`, `{}`, http.StatusOK)
	snapshot, err := m.QuotaSnapshot(t.Context(), "openai")
	require.NoError(t, err)
	require.Len(t, snapshot.Limits, 4)
	for i, want := range []string{"5 hours", "1 week", "Other quota · 15 minutes", "Other quota · 1 day"} {
		require.Equal(t, want, snapshot.Limits[i].Window)
		require.Equal(t, "percent", snapshot.Limits[i].Unit)
	}
	require.Zero(t, snapshot.Limits[0].UsedPercent)
	require.Equal(t, 90.0, snapshot.Limits[1].UsedPercent)
	require.EqualValues(t, 2000000300, snapshot.Limits[1].ResetsAt.Unix())
	require.True(t, snapshot.Reset.Supported)
	require.Zero(t, snapshot.Reset.Available)
}

func TestCodexOptionalFailuresKeepLimits(t *testing.T) {
	for _, tc := range []struct {
		name, profile string
		status        int
	}{
		{"unavailable", "secret-body", 503},
		{"unauthorized", "secret-body", 401},
		{"invalid", "secret-body", 200},
		{"wrong stats", `{"stats":"secret-body"}`, 200},
		{"oversized", strings.Repeat("x", maxQuotaBytes+1), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := codexFixtureManager(
				t,
				`{"rate_limit":{"primary_window":{"used_percent":25}},"rate_limit_reset_credits":{"available_count":"secret-body"}}`,
				tc.profile,
				tc.status,
			)
			snapshot, err := m.QuotaSnapshot(t.Context(), "openai")
			require.NoError(t, err)
			require.Len(t, snapshot.Limits, 1)
			require.Empty(t, snapshot.Tokens)
			require.False(t, snapshot.Reset.Supported)
		})
	}
}

func TestCodexHeaders(t *testing.T) {
	access := testJWT(
		t,
		map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_compute_residency": "eu"}},
	)
	m := newOpenAIQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+access {
			t.Error("wrong authorization")
		}
		if r.Header.Get("ChatGPT-Account-Id") != "acct_123" {
			t.Error("wrong account")
		}
		if r.Header.Get("x-openai-internal-codex-residency") != "eu" {
			t.Error("wrong residency")
		}
		if r.Header.Get("originator") != "cozyphi" || r.Header.Get("User-Agent") != "cozyphi" {
			t.Error("wrong client headers")
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Error("wrong accept")
		}
		if r.Header.Get("x-openai-codex-luna-reserve") != "" {
			t.Error("passive usage reader must not opt in")
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	cred := m.credentials["openai"]
	cred.Access = access
	m.credentials["openai"] = cred
	_, err := m.QuotaSnapshot(t.Context(), "openai")
	require.NoError(t, err)
}

func TestCodexQuotaAuthBoundary(t *testing.T) {
	cred := credential{Type: "oauth", Access: "secret-token", BaseURL: "https://chatgpt.com/backend-api/codex"}
	for _, target := range []string{
		"https://other.test/backend-api/wham/usage", "http://chatgpt.com/backend-api/wham/usage",
		"https://chatgpt.com:444/backend-api/wham/usage", "https://user@chatgpt.com/backend-api/wham/usage",
		"https://chatgpt.com/backend-api/wham/usage?secret-token", "https://chatgpt.com/backend-api/wham/usage#fragment",
		"https://chatgpt.com/backend-api/wham/%75sage", "https://chatgpt.com/backend-api/wham/usage/../profiles/me",
		"https://chatgpt.com/backend-api/wham/usage/", "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits/consume",
		"https://chatgpt.com/api/codex/usage", "https://chatgpt.com/backend-api/codex/responses",
	} {
		t.Run(target, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
			require.NoError(t, err)
			err = authorizeOpenAICodexQuotaRequest(req, cred)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret-token")
			require.Empty(t, req.Header.Get("Authorization"))
		})
	}
	for _, path := range []string{openAICodexUsagePath, openAICodexProfilePath} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://chatgpt.com"+path, http.NoBody)
		require.NoError(t, err)
		require.NoError(t, authorizeOpenAICodexQuotaRequest(req, cred))
		req.Method = http.MethodPost
		req.Header.Del("Authorization")
		require.Error(t, authorizeOpenAICodexQuotaRequest(req, cred))
		require.Empty(t, req.Header.Get("Authorization"))
	}
	for _, base := range []string{"https://chatgpt.com", "https://chatgpt.com/wrong", "https://user@chatgpt.com/backend-api/codex", "https://chatgpt.com/backend-api/codex?secret-token", "ftp://chatgpt.com/backend-api/codex", "https://chatgpt.com/backend-api/%63odex"} {
		_, err := openAICodexEndpoint(base, openAICodexUsagePath)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret-token")
	}
}

func TestCodexRedirectNeverReachesDestination(t *testing.T) {
	var hits atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer destination.Close()
	for _, optional := range []bool{false, true} {
		m := newOpenAIQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if optional && r.URL.Path == openAICodexUsagePath {
				_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":25}}}`))
				return
			}
			http.Redirect(w, r, destination.URL+"/secret-token", http.StatusFound)
		}))
		snapshot, err := m.QuotaSnapshot(t.Context(), "openai")
		if optional {
			require.NoError(t, err)
			require.Len(t, snapshot.Limits, 1)
		} else {
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret-token")
		}
	}
	require.Zero(t, hits.Load())
}

func TestCodexUsageErrorsAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
		want   string
	}{
		{"secret-body", 401, "HTTP status 401"},
		{`{"plan_type":123,"secret":"secret-body"}`, 200, "invalid Codex quota response"},
		{strings.Repeat("x", maxQuotaBytes+1), 200, "exceeds"},
	} {
		m := newOpenAIQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		_, err := m.QuotaSnapshot(t.Context(), "openai")
		require.ErrorContains(t, err, tc.want)
		require.NotContains(t, err.Error(), "secret-body")
		require.NotContains(t, err.Error(), "access-token")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	m := newOpenAIQuotaTestManager(t, http.NotFoundHandler())
	_, err := m.QuotaSnapshot(ctx, "openai")
	require.ErrorIs(t, err, context.Canceled)
}

func TestCodexCancellationDuringProfile(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	m := newOpenAIQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == openAICodexProfilePath {
			cancel()
			return
		}
		_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":25}}}`))
	}))
	_, err := m.QuotaSnapshot(ctx, "openai")
	require.ErrorIs(t, err, context.Canceled)
}

func codexFixtureManager(t *testing.T, usage, profile string, status int) *Manager {
	t.Helper()
	return newOpenAIQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("unexpected mutation")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case openAICodexUsagePath:
			_, _ = w.Write([]byte(usage))
		case openAICodexProfilePath:
			w.WriteHeader(status)
			_, _ = w.Write([]byte(profile))
		default:
			t.Error("unexpected endpoint")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}
