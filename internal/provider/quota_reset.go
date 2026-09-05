package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// QuotaResetTarget is an opaque, process-local confirmation capability. Copies
// share one intent: attempting any copy invalidates every outstanding pane.
// It contains only a random nonce, a digest and generations, never credentials.
type QuotaResetTarget struct {
	nonce      string
	binding    [32]byte
	generation uint64
	epoch      uint64
}

func (QuotaResetTarget) String() string   { return "Codex quota reset target" }
func (QuotaResetTarget) GoString() string { return "provider.QuotaResetTarget{opaque}" }

// QuotaResetCode is a verified Codex consume response code.
type QuotaResetCode string

const (
	QuotaResetDone            QuotaResetCode = "reset"
	QuotaResetNothing         QuotaResetCode = "nothing_to_reset"
	QuotaResetNoCredit        QuotaResetCode = "no_credit"
	QuotaResetAlreadyRedeemed QuotaResetCode = "already_redeemed"
)

// QuotaResetResult contains only the display-safe part of the consume response.
type QuotaResetResult struct {
	Code         QuotaResetCode `json:"code"`
	WindowsReset int64          `json:"windows_reset"`
}

var (
	// ErrQuotaResetUnknown means a request may have consumed a credit. Never retry
	// automatically; refresh usage and inspect the account before another intent.
	ErrQuotaResetUnknown = errors.New(
		"provider: Codex reset outcome is unknown; refresh usage before considering another reset",
	)
	// ErrQuotaResetStale requires a new quota snapshot and a new confirmation.
	ErrQuotaResetStale = errors.New("provider: Codex reset target is no longer valid; refresh usage and confirm again")
)

const (
	openAICodexResetPath = "/backend-api/wham/rate-limit-reset-credits/consume"
	quotaResetTimeout    = 30 * time.Second
)

func quotaResetBinding(c credential) [32]byte {
	// Length-delimited encoding avoids ambiguity between fields in the digest.
	data, _ := json.Marshal([]string{c.Type, c.AccountID, c.BaseURL, string(c.Protocol)})
	return sha256.Sum256(data)
}

func sameOAuthCredential(a, b credential) bool {
	return quotaResetBinding(a) == quotaResetBinding(b) && a.Access == b.Access &&
		a.Refresh == b.Refresh && a.Expires == b.Expires
}

func (m *Manager) issueQuotaResetTarget(
	cred credential, summary QuotaResetSummary, generation, epoch uint64,
) *QuotaResetTarget {
	if cred.Type != "oauth" || cred.AccountID == "" || !summary.Supported || summary.Available <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.quotaResetInFlight || generation != m.credentialGeneration || epoch != m.quotaResetEpoch ||
		!sameOAuthCredential(cred, m.credentials[openaiProviderID]) {
		return nil
	}
	if !m.quotaResetTargetValidLocked(m.quotaResetTarget) {
		nonce, err := randomOAuthValue()
		if err != nil {
			return nil // Usage is still useful if an action capability cannot be issued.
		}
		m.quotaResetTarget = &QuotaResetTarget{
			nonce: nonce, binding: quotaResetBinding(cred), generation: generation, epoch: epoch,
		}
	}
	// No mutable state is shared with the consumer, even inside this package.
	target := *m.quotaResetTarget
	return &target
}

// QuotaResetTargetValid checks a confirmation without I/O or OAuth refresh.
// ResetQuota repeats this check atomically when claiming the intent.
func (m *Manager) QuotaResetTargetValid(target *QuotaResetTarget) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.quotaResetTargetValidLocked(target)
}

func (m *Manager) quotaResetTargetValidLocked(target *QuotaResetTarget) bool {
	cred := m.credentials[openaiProviderID]
	return target != nil && m.quotaResetTarget != nil && *target == *m.quotaResetTarget &&
		!m.quotaResetInFlight && target.epoch == m.quotaResetEpoch &&
		target.generation == m.credentialGeneration && cred.Type == "oauth" && cred.AccountID != "" &&
		target.binding == quotaResetBinding(cred)
}

// ResetQuota spends at most one credit for an explicitly confirmed target.
// Claiming an intent invalidates all outstanding targets, even on preflight
// failure. No status, refresh failure or ambiguous response triggers a retry.
func (m *Manager) ResetQuota(ctx context.Context, target *QuotaResetTarget) (QuotaResetResult, error) {
	if m == nil {
		return QuotaResetResult{}, ErrQuotaResetStale
	}
	m.mu.Lock()
	if !m.quotaResetTargetValidLocked(target) {
		m.mu.Unlock()
		return QuotaResetResult{}, ErrQuotaResetStale
	}
	confirmed := *target
	m.quotaResetEpoch++
	m.quotaResetTarget = nil
	m.quotaResetInFlight = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.quotaResetInFlight = false
		// A fetch begun during the attempt must not authorize a later reset.
		m.quotaResetEpoch++
		m.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(ctx, quotaResetTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return QuotaResetResult{}, err
	}
	cred, err := m.validOAuthCredential(ctx, openaiProviderID)
	if err != nil {
		if errors.Is(err, ErrQuotaResetStale) {
			return QuotaResetResult{}, ErrQuotaResetStale
		}
		if ctx.Err() != nil {
			return QuotaResetResult{}, ctx.Err()
		}
		// OAuth/storage errors can contain reflected tokens, URLs or paths.
		return QuotaResetResult{}, errors.New(
			"provider: Codex reset was not sent; subscription refresh failed; reconnect and refresh usage",
		)
	}
	m.mu.RLock()
	valid := confirmed.generation == m.credentialGeneration && confirmed.binding == quotaResetBinding(cred) &&
		sameOAuthCredential(cred, m.credentials[openaiProviderID])
	m.mu.RUnlock()
	if !valid {
		return QuotaResetResult{}, ErrQuotaResetStale
	}
	return consumeOpenAIQuotaReset(ctx, m.httpClient, cred)
}

// This wire contract is pinned to official openai/codex
// 531f3836a1e38ea61eaaba3dccda6711eb6c0dca, rate_limit_resets.rs/types.rs.
// credit_id is deliberately omitted: the backend selects the available credit.
func consumeOpenAIQuotaReset(ctx context.Context, client *http.Client, cred credential) (QuotaResetResult, error) {
	endpoint, err := openAICodexEndpoint(cred.BaseURL, openAICodexUsagePath)
	if err != nil {
		return QuotaResetResult{}, ErrQuotaResetStale
	}
	endpoint = strings.TrimSuffix(endpoint, openAICodexUsagePath) + openAICodexResetPath
	nonce, err := randomOAuthValue()
	if err != nil {
		return QuotaResetResult{}, errors.New(
			"provider: Codex reset was not sent; could not generate a request identifier",
		)
	}
	payload, _ := json.Marshal(struct {
		RequestID string `json:"redeem_request_id"`
	}{RequestID: nonce})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return QuotaResetResult{}, ErrQuotaResetStale
	}
	if err := authorizeOpenAICodexResetRequest(req, cred); err != nil {
		return QuotaResetResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cozyphi")
	// A non-replayable POST body also prevents net/http transport retries.
	req.GetBody = nil
	if err := ctx.Err(); err != nil {
		return QuotaResetResult{}, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	resetClient := *client
	resetClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := resetClient.Do(req)
	if err != nil {
		return QuotaResetResult{}, ErrQuotaResetUnknown
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return QuotaResetResult{}, ErrQuotaResetUnknown
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxQuotaBytes+1))
	if err != nil || len(data) > maxQuotaBytes {
		return QuotaResetResult{}, ErrQuotaResetUnknown
	}
	var wire struct {
		Code    QuotaResetCode  `json:"code"`
		Windows json.RawMessage `json:"windows_reset"`
	}
	if json.Unmarshal(data, &wire) != nil {
		return QuotaResetResult{}, ErrQuotaResetUnknown
	}
	result := QuotaResetResult{Code: wire.Code}
	// Upstream defaults an absent count to zero, but rejects explicit null.
	if len(wire.Windows) != 0 && (string(wire.Windows) == "null" ||
		json.Unmarshal(wire.Windows, &result.WindowsReset) != nil || result.WindowsReset < 0) {
		return QuotaResetResult{}, ErrQuotaResetUnknown
	}
	switch result.Code {
	case QuotaResetDone, QuotaResetNothing, QuotaResetNoCredit, QuotaResetAlreadyRedeemed:
		return result, nil
	default:
		return QuotaResetResult{}, ErrQuotaResetUnknown
	}
}

func authorizeOpenAICodexResetRequest(req *http.Request, cred credential) error {
	if req == nil || req.URL == nil || req.Method != http.MethodPost || cred.Type != "oauth" ||
		cred.Access == "" || cred.AccountID == "" {
		return ErrQuotaResetStale
	}
	usage, err := openAICodexEndpoint(cred.BaseURL, openAICodexUsagePath)
	if err != nil {
		return ErrQuotaResetStale
	}
	origin := strings.TrimSuffix(usage, openAICodexUsagePath)
	if req.URL.String() != origin+openAICodexResetPath {
		return ErrQuotaResetStale
	}
	cred.BaseURL = origin + "/backend-api/wham"
	return authorizeOAuthRequest(req, cred)
}
