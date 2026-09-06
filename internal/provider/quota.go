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
	ProviderID  string
	PlanName    string
	Limits      []QuotaLimit
	Tokens      []QuotaTokenUsage
	Reset       QuotaResetSummary
	ResetTarget *QuotaResetTarget
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

// QuotaTokenUsage is provider-reported token usage with an explicit accounting scope.
type QuotaTokenUsage struct {
	Scope  string
	Tokens int64
}

// QuotaResetSummary describes manual rate-limit reset credits. Supported means
// the provider reported a count, including zero; ResetTarget grants an action.
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
	epoch := m.quotaResetEpoch
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
	m.mu.RLock()
	generation := m.credentialGeneration
	current := m.credentials[id]
	m.mu.RUnlock()
	if id == openaiProviderID && !sameOAuthCredential(current, cred) {
		return QuotaSnapshot{}, ErrQuotaResetStale
	}
	snapshot, err := adapter(ctx, m.httpClient, cred)
	if err != nil {
		return QuotaSnapshot{}, fmt.Errorf("provider: quota for %q: %w", id, err)
	}
	snapshot.ProviderID = id
	if id == openaiProviderID {
		snapshot.ResetTarget = m.issueQuotaResetTarget(cred, snapshot.Reset, generation, epoch)
	}
	return snapshot, nil
}

const (
	openAICodexUsagePath   = "/backend-api/wham/usage"
	openAICodexProfilePath = "/backend-api/wham/profiles/me"
	openAIResetNote        = "A manual reset spends an available Codex reset credit and requires confirmation."
)

// These read-only wire fields follow the pinned backend-client contract in
// doc/codex-usage.md. Pointers distinguish missing observations from zero.
type codexQuotaWindow struct {
	UsedPercent   *float64 `json:"used_percent"`
	WindowSeconds int64    `json:"limit_window_seconds"`
	ResetAt       *int64   `json:"reset_at"`
}

type codexQuotaWindows struct {
	Primary   *codexQuotaWindow `json:"primary_window"`
	Secondary *codexQuotaWindow `json:"secondary_window"`
}

type codexQuotaResponse struct {
	Plan       string             `json:"plan_type"`
	RateLimit  *codexQuotaWindows `json:"rate_limit"`
	Additional []struct {
		Name      string             `json:"limit_name"`
		Feature   string             `json:"metered_feature"`
		RateLimit *codexQuotaWindows `json:"rate_limit"`
	} `json:"additional_rate_limits"`
	Reset json.RawMessage `json:"rate_limit_reset_credits"`
}

type codexProfileResponse struct {
	Stats *struct {
		Lifetime *int64 `json:"lifetime_tokens"`
		Daily    []struct {
			Date   string `json:"start_date"`
			Tokens *int64 `json:"tokens"`
		} `json:"daily_usage_buckets"`
	} `json:"stats"`
}

func fetchOpenAIQuota(ctx context.Context, client *http.Client, cred credential) (QuotaSnapshot, error) {
	if cred.Type != "oauth" {
		return QuotaSnapshot{}, fmt.Errorf(
			"%w: OpenAI API keys do not expose Codex subscription limits",
			ErrQuotaUnsupported,
		)
	}
	var usage codexQuotaResponse
	if err := fetchOpenAICodexJSON(ctx, client, cred, openAICodexUsagePath, &usage); err != nil {
		return QuotaSnapshot{}, err
	}
	snapshot := decodeOpenAIQuota(usage)
	var profile codexProfileResponse
	if err := fetchOpenAICodexJSON(ctx, client, cred, openAICodexProfilePath, &profile); err == nil {
		snapshot.Tokens = decodeOpenAITokenUsage(profile)
	}
	// Optional profile failures must not discard good limits, but caller
	// cancellation still terminates the operation.
	if err := ctx.Err(); err != nil {
		return QuotaSnapshot{}, err
	}
	return snapshot, nil
}

func fetchOpenAICodexJSON(
	ctx context.Context, client *http.Client, cred credential, path string, target any,
) error {
	endpoint, err := openAICodexEndpoint(cred.BaseURL, path)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return errors.New("invalid Codex quota request")
	}
	if err := authorizeOpenAICodexQuotaRequest(req, cred); err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cozyphi")
	if client == nil {
		client = http.DefaultClient
	}
	quotaClient := *client
	quotaClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("OpenAI Codex quota redirects are not allowed")
	}
	resp, err := quotaClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Transport errors can contain redirect URLs or reflected credentials.
		return errors.New("fetch Codex quota failed; check connection (redirects are not allowed)")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxQuotaBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("read Codex quota response failed")
	}
	if len(data) > maxQuotaBytes {
		return fmt.Errorf("quota response exceeds %d bytes", maxQuotaBytes)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return errors.New("invalid Codex quota response")
	}
	return nil
}

func authorizeOpenAICodexQuotaRequest(req *http.Request, cred credential) error {
	if req == nil || req.URL == nil || req.Method != http.MethodGet {
		return errors.New("provider: OAuth request target is invalid")
	}
	endpoint, err := openAICodexEndpoint(cred.BaseURL, req.URL.Path)
	if err != nil {
		return err
	}
	// Exact URLs reject query strings, encoded paths and same-origin siblings.
	if req.URL.String() != endpoint {
		return errors.New("provider: OAuth request target does not match the connected endpoint")
	}
	cred.BaseURL = strings.TrimSuffix(endpoint, req.URL.Path) + "/backend-api/wham"
	return authorizeOAuthRequest(req, cred)
}

func openAICodexEndpoint(baseURL, path string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Host == "" || base.User != nil ||
		(base.Scheme != "https" && base.Scheme != "http") || base.RawQuery != "" || base.ForceQuery ||
		base.Fragment != "" || strings.TrimRight(base.EscapedPath(), "/") != "/backend-api/codex" {
		return "", errors.New("invalid Codex credential base URL; reconnect the provider")
	}
	if path != openAICodexUsagePath && path != openAICodexProfilePath {
		return "", errors.New("unsupported Codex quota request path")
	}
	base.Path, base.RawPath = path, ""
	return base.String(), nil
}

func decodeOpenAIQuota(payload codexQuotaResponse) QuotaSnapshot {
	snapshot := QuotaSnapshot{PlanName: payload.Plan, Reset: QuotaResetSummary{Note: openAIResetNote}}
	snapshot.Limits = appendOpenAIWindows(snapshot.Limits, payload.RateLimit, "")
	for _, additional := range payload.Additional {
		scope := firstNonEmpty(additional.Name, additional.Feature, "additional")
		snapshot.Limits = appendOpenAIWindows(snapshot.Limits, additional.RateLimit, scope)
	}
	var reset struct {
		Available *int64 `json:"available_count"`
	}
	if json.Unmarshal(payload.Reset, &reset) == nil && reset.Available != nil && *reset.Available >= 0 {
		snapshot.Reset.Available = *reset.Available
		snapshot.Reset.Supported = true // summary observed, not permission to mutate
	}
	return snapshot
}

func appendOpenAIWindows(limits []QuotaLimit, windows *codexQuotaWindows, scope string) []QuotaLimit {
	if windows == nil {
		return limits
	}
	for i, window := range []*codexQuotaWindow{windows.Primary, windows.Secondary} {
		if window == nil || window.UsedPercent == nil {
			continue
		}
		label := "primary"
		if i == 1 {
			label = "secondary"
		}
		if window.WindowSeconds > 0 {
			label = openAIWindowLabel(window.WindowSeconds)
		}
		if scope != "" {
			label = scope + " · " + label
		}
		limit := QuotaLimit{Window: label, Unit: "percent", UsedPercent: max(0, min(100, *window.UsedPercent))}
		if window.ResetAt != nil {
			limit.ResetsAt = time.Unix(*window.ResetAt, 0)
		}
		limits = append(limits, limit)
	}
	return limits
}

func openAIWindowLabel(seconds int64) string {
	for _, unit := range []struct {
		seconds int64
		name    string
	}{
		{604800, "week"}, {86400, "day"}, {3600, "hour"}, {60, "minute"},
	} {
		if seconds%unit.seconds == 0 {
			return pluralDuration(seconds/unit.seconds, unit.name)
		}
	}
	return pluralDuration(seconds, "second")
}

func decodeOpenAITokenUsage(profile codexProfileResponse) []QuotaTokenUsage {
	if profile.Stats == nil {
		return nil
	}
	var tokens []QuotaTokenUsage
	if lifetime := profile.Stats.Lifetime; lifetime != nil && *lifetime >= 0 {
		tokens = append(tokens, QuotaTokenUsage{Scope: "Codex profile lifetime", Tokens: *lifetime})
	}
	for _, bucket := range profile.Stats.Daily {
		// Keep the provider's date, not a local-time aggregate or an account total.
		if strings.TrimSpace(bucket.Date) == "" || bucket.Tokens == nil || *bucket.Tokens < 0 {
			continue
		}
		tokens = append(
			tokens,
			QuotaTokenUsage{Scope: "Codex profile daily bucket " + bucket.Date, Tokens: *bucket.Tokens},
		)
	}
	return tokens
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
		limit := QuotaLimit{
			Window:    window,
			Used:      used,
			Remaining: item.Remaining,
			Total:     total,
			ResetsAt:  resetsAt,
			Unit:      unit,
		}
		if unit == "percent" {
			// Percent windows name the used share, not budgets; renderers read
			// UsedPercent, so the transported share leaves Used/Total zeroed.
			limit.Used, limit.Total, limit.UsedPercent = 0, 0, float64(used)
		}
		limits = append(limits, limit)
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
// Windows the API names only as a used percentage (2026-09-06 drift, both
// kinds) fall back to a percent observation instead of decoding as zero
// budgets.
func zaiLimitAmounts(item zaiQuotaLimit) (used, total int64, unit string, ok bool) {
	switch item.Type {
	case "TOKENS_LIMIT":
		used = item.Usage
		if used == 0 && item.CurrentValue != 0 {
			used = item.CurrentValue
		}
		if used == 0 && item.Remaining == 0 {
			return zaiPercentAmounts(item)
		}
		return used, used + item.Remaining, "tokens", true
	case "CREDIT_LIMIT":
		total = item.Usage
		if total <= 0 {
			total = item.CurrentValue + item.Remaining
		}
		if item.CurrentValue == 0 && total == 0 {
			return zaiPercentAmounts(item)
		}
		return item.CurrentValue, total, "credits", true
	default:
		return 0, 0, "", false
	}
}

// zaiPercentAmounts reports a window the API names only by its used share:
// used carries the percentage for decodeZAIQuota to move onto UsedPercent.
// A missing or out-of-range share is no observation, never a zero budget.
func zaiPercentAmounts(item zaiQuotaLimit) (used, total int64, unit string, ok bool) {
	if item.Percentage <= 0 || item.Percentage > 100 {
		return 0, 0, "", false
	}
	return int64(item.Percentage), 100, "percent", true
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
