package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newQuotaTestManager builds a Manager whose zai-coding-plan credential is
// pinned to the test server, so quota requests hit httptest instead of the
// real z.ai endpoint.
func newQuotaTestManager(t *testing.T, handler http.Handler) *Manager {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	zai := builtinProviders()["zai-coding-plan"]
	zai.BaseURL = srv.URL + "/api/coding/paas/v4"
	return &Manager{
		providers: map[string]Info{"zai-coding-plan": zai},
		credentials: map[string]credential{
			"zai-coding-plan": {Type: "api", Key: "test-key", BaseURL: zai.BaseURL},
		},
		httpClient: srv.Client(),
	}
}

func TestQuotaSnapshotZAIHappyPath(t *testing.T) {
	var gotPath, gotAuth, gotAccept string
	m := newQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotAccept = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true, "code": 200, "msg": "",
			"data": {
				"planName": "GLM Coding Pro",
				"limits": [
					{"type": "TOKENS_LIMIT", "unit": 6, "number": 1, "usage": 42000, "remaining": 958000, "percentage": 4.2, "nextResetTime": 1759500000000},
					{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "usage": 12000, "remaining": 88000, "percentage": 12, "nextResetTime": 1759400000000},
					{"type": "TOKENS_LIMIT", "unit": 1, "number": 30, "usage": 300000, "remaining": 9700000, "percentage": 3, "nextResetTime": 1759800000000},
					{"type": "TIME_LIMIT", "unit": 5, "number": 1, "usage": 0, "remaining": 0, "percentage": 0, "nextResetTime": 1759800000000}
				]
			}
		}`))
	}))

	snapshot, err := m.QuotaSnapshot(t.Context(), "zai-coding-plan")
	require.NoError(t, err)
	require.Equal(t, "/api/monitor/usage/quota/limit", gotPath)
	require.Equal(t, "Bearer test-key", gotAuth)
	require.Equal(t, "application/json", gotAccept)

	require.Equal(t, "zai-coding-plan", snapshot.ProviderID)
	require.Equal(t, "GLM Coding Pro", snapshot.PlanName)
	require.Len(t, snapshot.Limits, 3, "TIME_LIMIT sentinel must be skipped")

	// Sorted by window length ascending: 5 hours, 1 week, 30 days.
	require.Equal(t, "5 hours", snapshot.Limits[0].Window)
	require.Equal(t, int64(12000), snapshot.Limits[0].Used)
	require.Equal(t, int64(88000), snapshot.Limits[0].Remaining)
	require.Equal(t, int64(100000), snapshot.Limits[0].Total)
	require.Equal(t, time.UnixMilli(1759400000000), snapshot.Limits[0].ResetsAt)

	require.Equal(t, "1 week", snapshot.Limits[1].Window)
	require.Equal(t, "30 days", snapshot.Limits[2].Window)
	require.Equal(t, int64(10000000), snapshot.Limits[2].Total)
}

func TestQuotaSnapshotZAIPlanFieldVariants(t *testing.T) {
	m := newQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success": true, "code": 200, "data": {"planType": "Lite Plan", "limits": [
			{"type": "TOKENS_LIMIT", "unit": 1, "number": 7, "usage": 0, "currentValue": 5000, "remaining": 45000, "nextResetTime": 0}
		]}}`))
	}))
	snapshot, err := m.QuotaSnapshot(t.Context(), "zai-coding-plan")
	require.NoError(t, err)
	require.Equal(t, "Lite Plan", snapshot.PlanName)
	require.Equal(t, int64(5000), snapshot.Limits[0].Used, "currentValue must back up a zero usage field")
	require.True(t, snapshot.Limits[0].ResetsAt.IsZero(), "missing nextResetTime means no reset time")
}

func TestQuotaSnapshotZAICreditLimits(t *testing.T) {
	m := newQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true, "code": 200, "msg": "",
			"data": {
				"level": "lite",
				"limits": [
					{"type": "CREDIT_LIMIT", "unit": 3, "number": 5, "usage": 2000, "currentValue": 1653, "remaining": 346, "percentage": 82, "nextResetTime": 1787176502893},
					{"type": "CREDIT_LIMIT", "unit": 6, "number": 1, "usage": 10000, "currentValue": 4562, "remaining": 5437, "percentage": 45, "nextResetTime": 1787607163997}
				]
			}
		}`))
	}))

	snapshot, err := m.QuotaSnapshot(t.Context(), "zai-coding-plan")
	require.NoError(t, err)
	require.Equal(t, "lite", snapshot.PlanName, "data.level must back up the missing plan fields")
	require.Len(t, snapshot.Limits, 2)

	// Sorted shortest window first: 5 hours, then 1 week. Credit limits report
	// the granted budget in usage and consumption in currentValue.
	require.Equal(t, "5 hours", snapshot.Limits[0].Window)
	require.Equal(t, int64(1653), snapshot.Limits[0].Used)
	require.Equal(t, int64(346), snapshot.Limits[0].Remaining)
	require.Equal(t, int64(2000), snapshot.Limits[0].Total)
	require.Equal(t, time.UnixMilli(1787176502893), snapshot.Limits[0].ResetsAt)

	require.Equal(t, "1 week", snapshot.Limits[1].Window)
	require.Equal(t, int64(4562), snapshot.Limits[1].Used)
	require.Equal(t, int64(5437), snapshot.Limits[1].Remaining)
	require.Equal(t, int64(10000), snapshot.Limits[1].Total)
}

func TestQuotaSnapshotZAIMixedLimitTypes(t *testing.T) {
	m := newQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success": true, "code": 200, "data": {"planName": "Pro", "limits": [
			{"type": "TOKENS_LIMIT", "unit": 6, "number": 1, "usage": 42000, "remaining": 958000, "nextResetTime": 1759500000000},
			{"type": "CREDIT_LIMIT", "unit": 3, "number": 5, "usage": 2000, "currentValue": 1653, "remaining": 346, "nextResetTime": 1787176502893}
		]}}`))
	}))

	snapshot, err := m.QuotaSnapshot(t.Context(), "zai-coding-plan")
	require.NoError(t, err)
	require.Len(t, snapshot.Limits, 2)

	// Both kinds survive with their own semantics, sorted shortest window first.
	require.Equal(t, "5 hours", snapshot.Limits[0].Window)
	require.Equal(t, int64(1653), snapshot.Limits[0].Used)
	require.Equal(t, int64(2000), snapshot.Limits[0].Total)

	require.Equal(t, "1 week", snapshot.Limits[1].Window)
	require.Equal(t, int64(42000), snapshot.Limits[1].Used, "token limits keep counting consumption in usage")
	require.Equal(t, int64(958000), snapshot.Limits[1].Remaining)
	require.Equal(t, int64(1000000), snapshot.Limits[1].Total)
}

func TestQuotaSnapshotZAIEndpointFallback(t *testing.T) {
	// The plain usage path answers the same envelope the legacy quota path
	// refuses to some valid coding-plan keys (openchamber/openchamber#3012).
	const fallbackPayload = `{"success": true, "code": 200, "data": {"level": "lite", "limits": [
		{"type": "CREDIT_LIMIT", "unit": 3, "number": 5, "usage": 2000, "currentValue": 1653, "remaining": 346}
	]}}`
	tests := []struct {
		name          string
		primaryStatus int
		primaryBody   string
	}{
		{
			name:          "http 401 on primary",
			primaryStatus: http.StatusUnauthorized,
			primaryBody:   "unauthorized account token",
		},
		{
			name:          "api level rejection on primary",
			primaryStatus: http.StatusOK,
			primaryBody:   `{"success": false, "code": 401, "msg": "token expired or incorrect"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var paths []string
			m := newQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.URL.Path != "/api/monitor/usage/quota/limit" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(fallbackPayload))
					return
				}
				w.WriteHeader(tt.primaryStatus)
				_, _ = w.Write([]byte(tt.primaryBody))
			}))

			snapshot, err := m.QuotaSnapshot(t.Context(), "zai-coding-plan")
			require.NoError(t, err)
			require.Equal(t, []string{"/api/monitor/usage/quota/limit", "/api/monitor/usage"}, paths,
				"the fallback endpoint must be tried exactly once")
			require.Equal(t, "lite", snapshot.PlanName)
			require.Len(t, snapshot.Limits, 1)
			require.Equal(t, int64(1653), snapshot.Limits[0].Used)
			require.Equal(t, int64(346), snapshot.Limits[0].Remaining)
			require.Equal(t, int64(2000), snapshot.Limits[0].Total)
		})
	}
}

func TestQuotaSnapshotZAINoFallbackWithoutRejection(t *testing.T) {
	var paths []string
	m := newQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	_, err := m.QuotaSnapshot(t.Context(), "zai-coding-plan")
	require.ErrorContains(t, err, "unexpected HTTP status 500")
	require.Equal(t, []string{"/api/monitor/usage/quota/limit"}, paths,
		"only rejections the fallback can answer may trigger it")
}

func TestQuotaSnapshotUnsupportedProvider(t *testing.T) {
	m := newQuotaTestManager(t, http.NotFoundHandler())
	m.providers["codex"] = builtinProviders()["codex"]
	m.credentials["codex"] = credential{Type: "oauth", BaseURL: "https://chatgpt.com/backend-api/codex"}
	_, err := m.QuotaSnapshot(t.Context(), "codex")
	require.ErrorIs(t, err, ErrQuotaUnsupported)
	require.Contains(t, err.Error(), "codex")

	_, err = m.QuotaSnapshot(t.Context(), "anthropic")
	require.ErrorIs(t, err, ErrQuotaUnsupported)
}

func TestQuotaSnapshotNotConnected(t *testing.T) {
	m := newQuotaTestManager(t, http.NotFoundHandler())
	delete(m.credentials, "zai-coding-plan")
	_, err := m.QuotaSnapshot(t.Context(), "zai-coding-plan")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not connected")
	require.Contains(t, err.Error(), "/connect")
}

func TestQuotaSnapshotZAIErrorPaths(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{
			name: "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("unauthorized account token"))
			},
			wantErr: "unexpected HTTP status 401",
		},
		{
			name: "api level rejection",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"success": false, "code": 1111, "msg": "invalid api key"}`))
			},
			wantErr: "quota API rejected the request: invalid api key",
		},
		{
			name:    "malformed body",
			handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not json")) },
			wantErr: "invalid quota response",
		},
		{
			name: "no usage limits",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"success": true, "code": 200, "data": {"planName": "Pro", "limits": [
				{"type": "TIME_LIMIT", "unit": 5, "number": 1, "nextResetTime": 1759800000000}
			]}}`))
			},
			wantErr: "contains no usage limits",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newQuotaTestManager(t, tt.handler)
			_, err := m.QuotaSnapshot(t.Context(), "zai-coding-plan")
			require.ErrorContains(t, err, tt.wantErr)
			require.NotContains(t, err.Error(), "test-key", "the API key must never reach an error")
		})
	}
}

func TestQuotaSnapshotUnknownUnitSkipped(t *testing.T) {
	m := newQuotaTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success": true, "code": 200, "data": {"planName": "Pro", "limits": [
			{"type": "TOKENS_LIMIT", "unit": 9, "number": 1, "usage": 1, "remaining": 2},
			{"type": "TOKENS_LIMIT", "unit": 1, "number": 7, "usage": 3, "remaining": 4}
		]}}`))
	}))
	snapshot, err := m.QuotaSnapshot(t.Context(), "zai-coding-plan")
	require.NoError(t, err)
	require.Len(t, snapshot.Limits, 1)
	require.Equal(t, "7 days", snapshot.Limits[0].Window)
}

func TestQuotaSnapshotContextCanceled(t *testing.T) {
	m := newQuotaTestManager(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := m.QuotaSnapshot(ctx, "zai-coding-plan")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "test-key")
}
