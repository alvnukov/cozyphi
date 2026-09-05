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
	Tokens     []QuotaTokenUsage
	Reset      QuotaResetSummary
}

// QuotaLimit is one usage window of a subscription plan.
type QuotaLimit struct {
	Window      string // display label, e.g. "5 hours"
	Used        int64
	Remaining   int64
	Total       int64     // granted budget; Used + Remaining for token limits
	ResetsAt    time.Time // zero when unknown
	Unit        string    // tokens, credits, or percent
	UsedPercent float64   // for Codex rate-limit windows that expose only a percentage
}

// QuotaTokenUsage is account-level token usage reported by the provider.
type QuotaTokenUsage struct {
	Scope  string
	Tokens int64
}

// QuotaResetSummary describes manual rate-limit reset credits without exposing
// a mutation. CozyPhi deliberately does not call the consume endpoint until the
// request schema and confirmation UX are implemented.
type QuotaResetSummary struct {
	Available int64
	Supported bool
	Note      string
}

// quotaAdapter fetches one provider's subscription quota over one stored
// credential. It is the internal seam behind Manager.QuotaSnapshot: adding a
// provider means adding a map entry, not a new caller-facing method.
type quotaAdapter func(ctx context.Context, client *http.Client, cred credential) (QuotaSnapshot, error)

var quotaAdapters = map[string]quotaAdapter{
	"openai":          fetchOpenAIQuota,
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
	if cred.Type == "oauth" {
		refreshed, err := m.validOAuthCredential(ctx, id)
		if err != nil {
			return QuotaSnapshot{}, err
		}
		cred = refreshed
	}
	snapshot, err := adapter(ctx, m.httpClient, cred)
	if err != nil {
		return QuotaSnapshot{}, fmt.Errorf("provider: quota for %q: %w", id, err)
	}
	snapshot.ProviderID = id
	return snapshot, nil
}

const (
	openAICodexUsagePath       = "/api/codex/usage"
	openAIResetCreditsPath     = "/api/codex/rate-limit-reset-credits"
	openAIResetConsumePathNote = "Codex exposes reset credits, but CozyPhi does not consume them yet; use the official Codex UI for manual resets."
)

// fetchOpenAIQuota reads Codex account usage from the same ChatGPT backend the
// subscription models use. The endpoint and its companion reset-credit paths
// are present in the Codex client; the consume path is intentionally not called
// here because it mutates account state.
func fetchOpenAIQuota(ctx context.Context, client *http.Client, cred credential) (QuotaSnapshot, error) {
	if cred.Type != "oauth" {
		return QuotaSnapshot{}, fmt.Errorf("%w: OpenAI API keys do not expose Codex subscription limits", ErrQuotaUnsupported)
	}
	usage, err := fetchOpenAICodexJSON(ctx, client, cred, openAICodexUsagePath)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	snapshot, err := decodeOpenAIQuota(usage)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	if resetPayload, resetErr := fetchOpenAICodexJSON(ctx, client, cred, openAIResetCreditsPath); resetErr == nil {
		snapshot.Reset = decodeOpenAIResetCredits(resetPayload)
	}
	return snapshot, nil
}

func fetchOpenAICodexJSON(
	ctx context.Context, client *http.Client, cred credential, path string,
) (map[string]any, error) {
	endpoint, err := openAICodexEndpoint(cred.BaseURL, path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("quota request: %w", err)
	}
	if err := authorizeOpenAICodexQuotaRequest(req, cred); err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if client == nil {
		client = http.DefaultClient
	}
	quotaClient := *client
	quotaClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("OpenAI Codex quota redirects are not allowed")
	}
	resp, err := quotaClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch quota: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxQuotaBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read quota response: %w", err)
	}
	if len(data) > maxQuotaBytes {
		return nil, fmt.Errorf("quota response exceeds %d bytes", maxQuotaBytes)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("invalid quota response: %w", err)
	}
	return payload, nil
}

func authorizeOpenAICodexQuotaRequest(req *http.Request, cred credential) error {
	if req == nil || req.URL == nil || req.URL.User != nil {
		return errors.New("provider: OAuth request target is invalid")
	}
	base, err := url.Parse(cred.BaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" || base.User != nil {
		return errors.New("credential base URL has no host; reconnect the provider")
	}
	if !strings.EqualFold(req.URL.Scheme, base.Scheme) || !strings.EqualFold(req.URL.Host, base.Host) {
		return errors.New("provider: OAuth request target does not match the connected endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+cred.Access)
	if cred.AccountID != "" {
		req.Header.Set("ChatGPT-Account-Id", cred.AccountID)
	}
	if residency := extractResidency(cred.Access); residency != "" {
		req.Header.Set("x-openai-internal-codex-residency", residency)
	}
	req.Header.Set("originator", "cozyphi")
	return nil
}

func openAICodexEndpoint(baseURL, path string) (string, error) {
	origin, err := zaiQuotaOrigin(baseURL)
	if err != nil {
		return "", err
	}
	return origin + path, nil
}

func decodeOpenAIQuota(payload map[string]any) (QuotaSnapshot, error) {
	root := objectPayload(payload)
	snapshot := QuotaSnapshot{
		PlanName: stringField(root, "plan_type", "planType", "plan_name", "planName", "type"),
	}
	for _, item := range arrayField(root, "rate_limits", "rateLimits", "additional_rate_limits", "additionalRateLimits") {
		limit, ok := decodeOpenAIRateLimit(item)
		if ok {
			snapshot.Limits = append(snapshot.Limits, limit)
		}
	}
	snapshot.Tokens = decodeOpenAITokenUsage(root)
	if len(snapshot.Limits) == 0 && len(snapshot.Tokens) == 0 {
		return QuotaSnapshot{}, errors.New("quota response contains no Codex usage data")
	}
	return snapshot, nil
}

func decodeOpenAIRateLimit(item map[string]any) (QuotaLimit, bool) {
	used, hasUsed := intField(item, "used", "usage", "current_value", "currentValue")
	remaining, hasRemaining := intField(item, "remaining", "available")
	total, hasTotal := intField(item, "limit", "total")
	if hasTotal && !hasUsed && hasRemaining {
		used = max(0, total-remaining)
		hasUsed = true
	}
	if !hasTotal && hasUsed && hasRemaining {
		total = used + remaining
		hasTotal = true
	}

	window := openAIWindowLabel(item)
	limit := QuotaLimit{Window: window, ResetsAt: timeField(item, "resets_at", "resetsAt")}
	if hasTotal && hasUsed {
		limit.Unit = "tokens"
		limit.Used, limit.Remaining, limit.Total = used, remaining, total
		return limit, true
	}
	pct, hasPercent := floatField(item, "used_percent", "usedPercent")
	if !hasPercent {
		if remainingPercent, ok := floatField(item, "remaining_percent", "remainingPercent"); ok {
			pct = 100 - remainingPercent
			hasPercent = true
		}
	}
	if !hasPercent {
		return QuotaLimit{}, false
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	limit.Unit = "percent"
	limit.UsedPercent = pct
	return limit, true
}

func openAIWindowLabel(item map[string]any) string {
	if label := stringField(item, "window", "name", "limit_name", "limitName", "metered_limit_name", "meteredLimitName"); label != "" {
		return label
	}
	minutes, ok := intField(item, "window_minutes", "windowMinutes", "window_duration_mins", "windowDurationMins")
	if !ok || minutes <= 0 {
		return "limit"
	}
	if minutes%10080 == 0 {
		return pluralDuration(minutes/10080, "week")
	}
	if minutes%1440 == 0 {
		return pluralDuration(minutes/1440, "day")
	}
	if minutes%60 == 0 {
		return pluralDuration(minutes/60, "hour")
	}
	return pluralDuration(minutes, "minute")
}

func decodeOpenAITokenUsage(root map[string]any) []QuotaTokenUsage {
	usage, _ := root["usage"].(map[string]any)
	if usage == nil {
		usage = root
	}
	if tokens, ok := intField(usage, "tokens", "total_tokens", "totalTokens"); ok {
		return []QuotaTokenUsage{{Scope: "account", Tokens: tokens}}
	}
	buckets := arrayField(usage, "daily_usage_buckets", "dailyUsageBuckets", "buckets")
	var total int64
	hasTokens := false
	for _, bucket := range buckets {
		if tokens, ok := intField(bucket, "tokens", "total_tokens", "totalTokens"); ok {
			total += tokens
			hasTokens = true
		}
	}
	if hasTokens {
		return []QuotaTokenUsage{{Scope: "account daily buckets", Tokens: total}}
	}
	return nil
}

func decodeOpenAIResetCredits(payload map[string]any) QuotaResetSummary {
	root := objectPayload(payload)
	available, ok := intField(root, "available_count", "availableCount", "available")
	if !ok {
		return QuotaResetSummary{}
	}
	return QuotaResetSummary{Available: available, Supported: available > 0, Note: openAIResetConsumePathNote}
}

func objectPayload(payload map[string]any) map[string]any {
	if data, ok := payload["data"].(map[string]any); ok {
		return data
	}
	return payload
}

func arrayField(root map[string]any, names ...string) []map[string]any {
	for _, name := range names {
		raw, ok := root[name]
		if !ok {
			continue
		}
		items, ok := raw.([]any)
		if !ok {
			continue
		}
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if object, ok := item.(map[string]any); ok {
				out = append(out, object)
			}
		}
		return out
	}
	return nil
}

func stringField(root map[string]any, names ...string) string {
	for _, name := range names {
		if value, ok := root[name].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func intField(root map[string]any, names ...string) (int64, bool) {
	for _, name := range names {
		switch value := root[name].(type) {
		case float64:
			return int64(value), true
		case int64:
			return value, true
		case json.Number:
			parsed, err := value.Int64()
			return parsed, err == nil
		}
	}
	return 0, false
}

func floatField(root map[string]any, names ...string) (float64, bool) {
	for _, name := range names {
		switch value := root[name].(type) {
		case float64:
			return value, true
		case json.Number:
			parsed, err := value.Float64()
			return parsed, err == nil
		}
	}
	return 0, false
}

func timeField(root map[string]any, names ...string) time.Time {
	for _, name := range names {
		switch value := root[name].(type) {
		case string:
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				return parsed
			}
		case float64:
			if value > 1e12 {
				return time.UnixMilli(int64(value))
			}
			if value > 0 {
				return time.Unix(int64(value), 0)
			}
		}
	}
	return time.Time{}
}

func hasAnyKey(root map[string]any, names ...string) bool {
	for _, name := range names {
		if _, ok := root[name]; ok {
			return true
		}
	}
	return false
}

func pluralDuration(n int64, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
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
		used, total, unit, ok := zaiLimitAmounts(item)
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
			Unit:      unit,
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
func zaiLimitAmounts(item zaiQuotaLimit) (used, total int64, unit string, ok bool) {
	switch item.Type {
	case "TOKENS_LIMIT":
		used = item.Usage
		if used == 0 && item.CurrentValue != 0 {
			used = item.CurrentValue
		}
		return used, used + item.Remaining, "tokens", true
	case "CREDIT_LIMIT":
		total = item.Usage
		if total <= 0 {
			total = item.CurrentValue + item.Remaining
		}
		return item.CurrentValue, total, "credits", true
	default:
		return 0, 0, "", false
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
