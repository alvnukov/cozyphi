package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxQuotaBytes bounds a quota response body; the document is small JSON.
const maxQuotaBytes = 1 << 20

// ErrQuotaUnsupported reports a provider that has no quota adapter yet.
var ErrQuotaUnsupported = errors.New("subscription quota is not supported for this provider")

// QuotaSnapshot is a provider-neutral subscription usage report, safe for
// display: no credentials, only plan metadata and usage numbers.
type QuotaSnapshot struct {
	ProviderID string
	PlanName   string
	Limits     []QuotaLimit
}

// QuotaLimit is one usage window of a subscription plan.
type QuotaLimit struct {
	Window    string // display label, e.g. "5 hours"
	Used      int64
	Remaining int64
	Total     int64     // granted budget; Used + Remaining for token limits
	ResetsAt  time.Time // zero when unknown
}

// quotaAdapter fetches one provider's subscription quota over one stored
// credential. It is the internal seam behind Manager.QuotaSnapshot: adding a
// provider means adding a map entry, not a new caller-facing method.
type quotaAdapter func(ctx context.Context, client *http.Client, cred credential) (QuotaSnapshot, error)

var quotaAdapters = map[string]quotaAdapter{
	"zai-coding-plan": fetchZAIQuota,
}

// QuotaSnapshot returns the subscription quota of a connected provider.
// The API key stays inside the package: it rides the Authorization header
// and never appears in returned errors.
func (m *Manager) QuotaSnapshot(ctx context.Context, providerID string) (QuotaSnapshot, error) {
	if m == nil {
		return QuotaSnapshot{}, errors.New("provider: manager is nil")
	}
	id := strings.TrimSpace(providerID)
	adapter, supported := quotaAdapters[id]
	if !supported {
		return QuotaSnapshot{}, fmt.Errorf("%w: %q has no quota endpoint", ErrQuotaUnsupported, id)
	}
	m.mu.RLock()
	cred, connected := m.credentials[id]
	m.mu.RUnlock()
	if !connected {
		return QuotaSnapshot{}, fmt.Errorf("provider: %q is not connected; open /connect to add it", id)
	}
	snapshot, err := adapter(ctx, m.httpClient, cred)
	if err != nil {
		return QuotaSnapshot{}, fmt.Errorf("provider: quota for %q: %w", id, err)
	}
	snapshot.ProviderID = id
	return snapshot, nil
}

// z.ai monitor endpoints, both answering the same envelope. The legacy
// quota path rejects some valid coding-plan keys (openchamber/openchamber#3012)
// while the plain usage path still serves them, so the fetcher tries both.
// The host comes from the pinned credential BaseURL origin, never from the
// remote catalog.
const (
	zaiQuotaPath         = "/api/monitor/usage/quota/limit"
	zaiQuotaFallbackPath = "/api/monitor/usage"
)

type zaiQuotaResponse struct {
	Success bool   `json:"success"`
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Data    struct {
		PlanName    string          `json:"planName"`
		Plan        string          `json:"plan"`
		PlanType    string          `json:"planType"`
		PackageName string          `json:"packageName"`
		Level       string          `json:"level"`
		Limits      []zaiQuotaLimit `json:"limits"`
	} `json:"data"`
}

type zaiQuotaLimit struct {
	Type          string  `json:"type"`
	Unit          int     `json:"unit"`
	Number        int64   `json:"number"`
	Usage         int64   `json:"usage"`
	CurrentValue  int64   `json:"currentValue"`
	Remaining     int64   `json:"remaining"`
	Percentage    float64 `json:"percentage"`
	NextResetTime int64   `json:"nextResetTime"`
}

// fetchZAIQuota walks z.ai's two monitor endpoints on the credential's
// origin: the legacy quota path, then — when that path answers a rejection
// an alternate endpoint can clear (HTTP 401 or an API-level refusal) — the
// plain usage path, which still returns live data in the same envelope for
// some valid coding-plan keys.
func fetchZAIQuota(ctx context.Context, client *http.Client, cred credential) (QuotaSnapshot, error) {
	origin, err := zaiQuotaOrigin(cred.BaseURL)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	snapshot, err := fetchZAIQuotaOnce(ctx, client, cred, origin+zaiQuotaPath)
	if err == nil {
		return snapshot, nil
	}
	var quotaErr *zaiQuotaError
	if !errors.As(err, &quotaErr) || !quotaErr.rejected {
		return QuotaSnapshot{}, err
	}
	return fetchZAIQuotaOnce(ctx, client, cred, origin+zaiQuotaFallbackPath)
}

// zaiQuotaError carries why one endpoint attempt failed; rejected is set
// only for the refusals the fallback endpoint can answer (HTTP 401 or an
// API-level success=false envelope), never for transport or decode errors.
type zaiQuotaError struct {
	rejected bool
	err      error
}

func (e *zaiQuotaError) Error() string { return e.err.Error() }

func (e *zaiQuotaError) Unwrap() error { return e.err }

// fetchZAIQuotaOnce performs one GET against a single z.ai quota endpoint.
// The API key rides the Authorization header and never reaches an error.
func fetchZAIQuotaOnce(
	ctx context.Context, client *http.Client, cred credential, endpoint string,
) (QuotaSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return QuotaSnapshot{}, &zaiQuotaError{err: fmt.Errorf("quota request: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+cred.Key)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return QuotaSnapshot{}, &zaiQuotaError{err: fmt.Errorf("fetch quota: %w", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return QuotaSnapshot{}, &zaiQuotaError{
			rejected: resp.StatusCode == http.StatusUnauthorized,
			err:      fmt.Errorf("unexpected HTTP status %d", resp.StatusCode),
		}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxQuotaBytes+1))
	if err != nil {
		return QuotaSnapshot{}, &zaiQuotaError{err: fmt.Errorf("read quota response: %w", err)}
	}
	if len(data) > maxQuotaBytes {
		return QuotaSnapshot{}, &zaiQuotaError{err: fmt.Errorf("quota response exceeds %d bytes", maxQuotaBytes)}
	}
	var payload zaiQuotaResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return QuotaSnapshot{}, &zaiQuotaError{err: fmt.Errorf("invalid quota response: %w", err)}
	}
	if !payload.Success || payload.Code != 200 {
		msg := strings.TrimSpace(payload.Msg)
		if msg == "" {
			msg = fmt.Sprintf("code %d", payload.Code)
		}
		return QuotaSnapshot{}, &zaiQuotaError{
			rejected: true,
			err:      fmt.Errorf("quota API rejected the request: %s", truncateText(msg, 200)),
		}
	}
	return decodeZAIQuota(payload)
}

func decodeZAIQuota(payload zaiQuotaResponse) (QuotaSnapshot, error) {
	planName := firstNonEmpty(
		payload.Data.PlanName, payload.Data.Plan, payload.Data.PlanType, payload.Data.PackageName, payload.Data.Level,
	)
	var limits []QuotaLimit
	var windowMinutes []int64
	for _, item := range payload.Data.Limits {
		// TIME_LIMIT entries are reset sentinels, not budgets; each limit
		// entry carries its own reset time.
		used, total, ok := zaiLimitAmounts(item)
		if !ok {
			continue
		}
		window, minutes, ok := zaiWindow(item.Unit, item.Number)
		if !ok {
			continue
		}
		var resetsAt time.Time
		if item.NextResetTime > 0 {
			resetsAt = time.UnixMilli(item.NextResetTime)
		}
		limits = append(limits, QuotaLimit{
			Window:    window,
			Used:      used,
			Remaining: item.Remaining,
			Total:     total,
			ResetsAt:  resetsAt,
		})
		windowMinutes = append(windowMinutes, minutes)
	}
	if len(limits) == 0 {
		return QuotaSnapshot{}, errors.New("quota response contains no usage limits")
	}
	// Sort by window length ascending so the pane can render shortest first
	// without knowing z.ai's unit codes.
	for i := 1; i < len(limits); i++ {
		for j := i; j > 0 && windowMinutes[j] < windowMinutes[j-1]; j-- {
			limits[j], limits[j-1] = limits[j-1], limits[j]
			windowMinutes[j], windowMinutes[j-1] = windowMinutes[j-1], windowMinutes[j]
		}
	}
	return QuotaSnapshot{PlanName: planName, Limits: limits}, nil
}

// zaiLimitAmounts maps one limit entry to used/total by budget kind. The two
// kinds disagree on field semantics: token budgets count consumed tokens in
// usage (currentValue only backs up a zero usage), while credit budgets
// report the granted credits in usage and the consumed ones in currentValue.
func zaiLimitAmounts(item zaiQuotaLimit) (used, total int64, ok bool) {
	switch item.Type {
	case "TOKENS_LIMIT":
		used = item.Usage
		if used == 0 && item.CurrentValue != 0 {
			used = item.CurrentValue
		}
		return used, used + item.Remaining, true
	case "CREDIT_LIMIT":
		total = item.Usage
		if total <= 0 {
			total = item.CurrentValue + item.Remaining
		}
		return item.CurrentValue, total, true
	default:
		return 0, 0, false
	}
}

// zaiWindow maps the API's unit code and count to a display label and a
// length in minutes. Unit codes: 1 day, 3 hour, 5 minute, 6 week.
func zaiWindow(unit int, number int64) (string, int64, bool) {
	var name string
	var minutes int64
	switch unit {
	case 1:
		name, minutes = "day", 1440
	case 3:
		name, minutes = "hour", 60
	case 5:
		name, minutes = "minute", 1
	case 6:
		name, minutes = "week", 10080
	default:
		return "", 0, false
	}
	if number <= 0 {
		return "", 0, false
	}
	if number == 1 {
		return "1 " + name, minutes, true
	}
	return fmt.Sprintf("%d %ss", number, name), minutes * number, true
}

// zaiQuotaOrigin extracts scheme://host from the credential BaseURL so both
// quota endpoints are pinned to the origin the chat API itself uses.
func zaiQuotaOrigin(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" {
		return "", errors.New("credential base URL has no host; reconnect the provider")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func truncateText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
