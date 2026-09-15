package provider

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// newKimiQuotaTestManager builds a Manager whose kimi-code credential is
// pinned to the test server, so usage requests hit httptest instead of the
// real Kimi endpoint. The token is unexpired, so the manager's pre-flight
// refresh keeps the stored credential and touches no network.
func newKimiQuotaTestManager(t *testing.T, handler http.Handler) *Manager {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	kimi := builtinProviders()["kimi-code"]
	kimi.BaseURL = srv.URL + "/coding/v1"
	return &Manager{
		providers: map[string]Info{"kimi-code": kimi},
		credentials: map[string]credential{
			"kimi-code": {
				Type:     "oauth",
				Access:   "access-token",
				Refresh:  "refresh-token",
				BaseURL:  kimi.BaseURL,
				Protocol: llm.ProtocolOpenAI,
				Expires:  time.Now().Add(time.Hour).UnixMilli(),
			},
		},
		httpClient: srv.Client(),
	}
}

func TestQuotaSnapshotKimiHappyPath(t *testing.T) {
	var gotPath, gotAuth string
	m := newKimiQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"usages": {
				"limit_5h": {"used_ratio": 0.3, "reset_time": "2026-09-11T18:00:00Z"},
				"limit_7d": {"used_ratio": 0.2, "reset_time": "2026-09-17T00:00:00Z"},
				"limit_month_total": {"used_ratio": 0.4, "reset_time": "2026-10-01T00:00:00Z"},
				"limit_month_code": {"used_ratio": 0.25}
			},
			"boosterWallet": {
				"balance": {"type": "BOOSTER", "amount": 1250000000, "amountLeft": 500000000},
				"monthlyChargeLimit": {"priceInCents": 0, "currency": ""},
				"monthlyUsed": {"priceInCents": 0, "currency": ""},
				"monthlyChargeLimitEnabled": false
			}
		}`))
	}))

	snapshot, err := m.QuotaSnapshot(t.Context(), "kimi-code")
	require.NoError(t, err)
	require.Equal(t, "kimi-code", snapshot.ProviderID)
	require.Equal(t, "/coding/v1/usages", gotPath)
	require.Equal(t, "Bearer access-token", gotAuth)

	require.Len(t, snapshot.Limits, 5)
	require.Equal(t, "5 hours", snapshot.Limits[0].Window)
	require.Equal(t, "percent", snapshot.Limits[0].Unit)
	require.Equal(t, 30.0, snapshot.Limits[0].UsedPercent)
	require.Equal(t, time.Date(2026, 9, 11, 18, 0, 0, 0, time.UTC), snapshot.Limits[0].ResetsAt)

	require.Equal(t, "7 days", snapshot.Limits[1].Window)
	require.Equal(t, 20.0, snapshot.Limits[1].UsedPercent)

	require.Equal(t, "month", snapshot.Limits[2].Window)
	require.Equal(t, 40.0, snapshot.Limits[2].UsedPercent)
	require.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), snapshot.Limits[2].ResetsAt)

	require.Equal(t, "month · code", snapshot.Limits[3].Window)
	require.Equal(t, 25.0, snapshot.Limits[3].UsedPercent)
	require.True(t, snapshot.Limits[3].ResetsAt.IsZero(), "a window without reset_time has no reset")

	// Wallet: amount 1250000000 fixed-point cents = 1250 cents total,
	// amount_left 500000000 = 500 cents remaining; currency falls back to USD.
	require.Equal(t, "top-up wallet", snapshot.Limits[4].Window)
	require.Equal(t, int64(750), snapshot.Limits[4].Used)
	require.Equal(t, int64(500), snapshot.Limits[4].Remaining)
	require.Equal(t, int64(1250), snapshot.Limits[4].Total)
	require.Equal(t, "USD", snapshot.Limits[4].Unit)
}

func TestQuotaSnapshotKimiSkipsAbsentWindowsAndWallet(t *testing.T) {
	m := newKimiQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"usages": {"limit_7d": {"used_ratio": "0.2"}}}`))
	}))

	snapshot, err := m.QuotaSnapshot(t.Context(), "kimi-code")
	require.NoError(t, err)
	require.Len(t, snapshot.Limits, 1)
	require.Equal(t, "7 days", snapshot.Limits[0].Window)
	require.Equal(t, 20.0, snapshot.Limits[0].UsedPercent, "the wire carries the ratio as a string too")
}

func TestQuotaSnapshotKimiGenericRateLimitShape(t *testing.T) {
	// The form api.kimi.com/coding/v1/usages returns for accounts without
	// the managed usages view, captured live on 2026-09-15. The server
	// reports exactly the windows it currently enforces — one entry here,
	// not a fixed set. `remaining` rides along but the decode derives the
	// share from used/limit, so it stays unread on purpose.
	m := newKimiQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"limits": [
			{"window": {"duration": 300, "timeUnit": "TIME_UNIT_MINUTE"},
			 "detail": {"limit": "100", "used": "5", "remaining": "95",
			             "resetTime": "2026-09-15T17:57:32.438199Z"}}
		]}`))
	}))

	snapshot, err := m.QuotaSnapshot(t.Context(), "kimi-code")
	require.NoError(t, err)
	require.Len(t, snapshot.Limits, 1)

	require.Equal(t, "5 hours", snapshot.Limits[0].Window)
	require.Equal(t, "percent", snapshot.Limits[0].Unit)
	require.Equal(t, 5.0, snapshot.Limits[0].UsedPercent)
	require.Equal(t,
		time.Date(2026, 9, 15, 17, 57, 32, 438199000, time.UTC),
		snapshot.Limits[0].ResetsAt,
	)
}

func TestQuotaSnapshotKimiEmptyResponseIsNotAZeroedSnapshot(t *testing.T) {
	m := newKimiQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"usages": {}, "boosterWallet": null}`))
	}))

	_, err := m.QuotaSnapshot(t.Context(), "kimi-code")
	require.ErrorContains(t, err, "no usage windows")
}

func TestQuotaSnapshotKimiNonBoosterWalletIsAbsent(t *testing.T) {
	m := newKimiQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"usages": {"limit_5h": {"used_ratio": 0.1}},
			"boosterWallet": {"balance": {"type": "OTHER", "amount": 1250000000, "amountLeft": 0}}
		}`))
	}))

	snapshot, err := m.QuotaSnapshot(t.Context(), "kimi-code")
	require.NoError(t, err)
	require.Len(t, snapshot.Limits, 1, "a wallet that is not BOOSTER holds no balance to show")
	require.Equal(t, "5 hours", snapshot.Limits[0].Window)
}

func TestQuotaSnapshotKimiErrorPaths(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		m := newKimiQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		_, err := m.QuotaSnapshot(t.Context(), "kimi-code")
		require.ErrorContains(t, err, "unexpected HTTP status 401")
		require.NotContains(t, err.Error(), "access-token")
	})

	t.Run("not found", func(t *testing.T) {
		m := newKimiQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		_, err := m.QuotaSnapshot(t.Context(), "kimi-code")
		require.ErrorContains(t, err, "unexpected HTTP status 404")
	})

	t.Run("invalid json", func(t *testing.T) {
		m := newKimiQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{not json`))
		}))
		_, err := m.QuotaSnapshot(t.Context(), "kimi-code")
		require.ErrorContains(t, err, "invalid kimi usage response")
	})

	t.Run("disconnected", func(t *testing.T) {
		m := newKimiQuotaTestManager(t, http.NotFoundHandler())
		delete(m.credentials, "kimi-code")
		_, err := m.QuotaSnapshot(t.Context(), "kimi-code")
		require.ErrorContains(t, err, "not connected")
	})
}

func TestQuotaSnapshotKimiAPIKeyUnsupported(t *testing.T) {
	m := newKimiQuotaTestManager(t, http.NotFoundHandler())
	m.credentials["kimi-code"] = credential{
		Type:     "api",
		Key:      "secret",
		BaseURL:  kimiAPIBaseURL,
		Protocol: llm.ProtocolOpenAI,
	}
	_, err := m.QuotaSnapshot(t.Context(), "kimi-code")
	require.ErrorIs(t, err, ErrQuotaUnsupported)
	require.NotContains(t, err.Error(), "secret")
}
