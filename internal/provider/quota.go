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
	Total     int64     // Used + Remaining
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

// zaiQuotaPath is the subscription quota endpoint the official z.ai status
// tools use. The host comes from the pinned credential BaseURL origin, never
// from the remote catalog.
const zaiQuotaPath = "/api/monitor/usage/quota/limit"

type zaiQuotaResponse struct {
	Success bool   `json:"success"`
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Data    struct {
		PlanName    string          `json:"planName"`
		Plan        string          `json:"plan"`
		PlanType    string          `json:"planType"`
		PackageName string          `json:"packageName"`
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

func fetchZAIQuota(ctx context.Context, client *http.Client, cred credential) (QuotaSnapshot, error) {
	endpoint, err := zaiQuotaEndpoint(cred.BaseURL)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return QuotaSnapshot{}, fmt.Errorf("quota request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cred.Key)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return QuotaSnapshot{}, fmt.Errorf("fetch quota: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return QuotaSnapshot{}, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxQuotaBytes+1))
	if err != nil {
		return QuotaSnapshot{}, fmt.Errorf("read quota response: %w", err)
	}
	if len(data) > maxQuotaBytes {
		return QuotaSnapshot{}, fmt.Errorf("quota response exceeds %d bytes", maxQuotaBytes)
	}
	var payload zaiQuotaResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return QuotaSnapshot{}, fmt.Errorf("invalid quota response: %w", err)
	}
	if !payload.Success || payload.Code != 200 {
		msg := strings.TrimSpace(payload.Msg)
		if msg == "" {
			msg = fmt.Sprintf("code %d", payload.Code)
		}
		return QuotaSnapshot{}, fmt.Errorf("quota API rejected the request: %s", truncateText(msg, 200))
	}
	return decodeZAIQuota(payload)
}

func decodeZAIQuota(payload zaiQuotaResponse) (QuotaSnapshot, error) {
	planName := firstNonEmpty(payload.Data.PlanName, payload.Data.Plan, payload.Data.PlanType, payload.Data.PackageName)
	var limits []QuotaLimit
	var windowMinutes []int64
	for _, item := range payload.Data.Limits {
		// TIME_LIMIT entries are reset sentinels, not token budgets; each
		// TOKENS_LIMIT carries its own reset time.
		if item.Type != "TOKENS_LIMIT" {
			continue
		}
		window, minutes, ok := zaiWindow(item.Unit, item.Number)
		if !ok {
			continue
		}
		used := item.Usage
		if used == 0 && item.CurrentValue != 0 {
			used = item.CurrentValue
		}
		var resetsAt time.Time
		if item.NextResetTime > 0 {
			resetsAt = time.UnixMilli(item.NextResetTime)
		}
		limits = append(limits, QuotaLimit{
			Window:    window,
			Used:      used,
			Remaining: item.Remaining,
			Total:     used + item.Remaining,
			ResetsAt:  resetsAt,
		})
		windowMinutes = append(windowMinutes, minutes)
	}
	if len(limits) == 0 {
		return QuotaSnapshot{}, errors.New("quota response contains no token limits")
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

func zaiQuotaEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" {
		return "", errors.New("credential base URL has no host; reconnect the provider")
	}
	return parsed.Scheme + "://" + parsed.Host + zaiQuotaPath, nil
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
