package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Kimi's subscription state lives at GET {base}/usages on the coding API,
// authorized with the subscription's Bearer access token. Endpoint, payload
// shape, and error hints are confirmed by MoonshotAI/kimi-code
// (packages/oauth/src/managed-usage.ts), the same first-hand source the OAuth
// profile was ported from. The payload mixes conventions on purpose, matching
// the wire: usage windows are snake_case while the wallet's money fields are
// camelCase.
const (
	kimiUsagePath = "/usages"
	// kimiFixedPointCents is the wallet's fixed-point scale: amount values
	// carry cents multiplied by 10^6 (managed-usage.ts FIXED_POINT_CENTS).
	kimiFixedPointCents = 1_000_000
)

// kimiRatio is a usage share the wire may carry as a JSON number or as its
// string form; both arrive in the wild and the reference client accepts
// either (managed-usage.ts ratioValue).
type kimiRatio float64

func (r *kimiRatio) UnmarshalJSON(data []byte) error {
	var number float64
	if err := json.Unmarshal(data, &number); err == nil {
		*r = kimiRatio(number)
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return errors.New("usage ratio is neither a number nor a string")
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return errors.New("usage ratio string is not a number")
	}
	*r = kimiRatio(value)
	return nil
}

type kimiQuotaEntry struct {
	UsedRatio kimiRatio `json:"used_ratio"`
	ResetTime string    `json:"reset_time"`
}

type kimiQuotaBalance struct {
	Type       string `json:"type"`
	Amount     int64  `json:"amount"`
	AmountLeft int64  `json:"amount_left"`
}

type kimiQuotaMoney struct {
	PriceInCents int64  `json:"priceInCents"`
	Currency     string `json:"currency"`
}

type kimiBoosterWallet struct {
	Balance                   kimiQuotaBalance `json:"balance"`
	MonthlyChargeLimit        kimiQuotaMoney   `json:"monthly_charge_limit"`
	MonthlyUsed               kimiQuotaMoney   `json:"monthly_used"`
	MonthlyChargeLimitEnabled bool             `json:"monthly_charge_limit_enabled"`
}

type kimiQuotaResponse struct {
	Usages struct {
		Limit5h         *kimiQuotaEntry `json:"limit_5h"`
		Limit7d         *kimiQuotaEntry `json:"limit_7d"`
		LimitMonthTotal *kimiQuotaEntry `json:"limit_month_total"`
		LimitMonthCode  *kimiQuotaEntry `json:"limit_month_code"`
	} `json:"usages"`
	BoosterWallet *kimiBoosterWallet `json:"booster_wallet"`
}

// fetchKimiQuota reads the kimi-code subscription state. The credential stays
// inside the package: the access token rides the Authorization header and the
// request is confined to the connected endpoint by the same authorize step
// that signs chat requests.
func fetchKimiQuota(ctx context.Context, client *http.Client, cred credential) (QuotaSnapshot, error) {
	if cred.Type != "oauth" {
		return QuotaSnapshot{}, fmt.Errorf(
			"%w: kimi-code exposes subscription usage only for the subscription sign-in",
			ErrQuotaUnsupported,
		)
	}
	endpoint, err := kimiUsageEndpoint(cred.BaseURL)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return QuotaSnapshot{}, errors.New("invalid kimi usage request")
	}
	if err := (kimiGrant{}).authorize(req, cred); err != nil {
		return QuotaSnapshot{}, err
	}
	req.Header.Set("Accept", "application/json")
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return QuotaSnapshot{}, ctx.Err()
		}
		return QuotaSnapshot{}, errors.New("fetch kimi usage failed; check connection")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return QuotaSnapshot{}, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxQuotaBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return QuotaSnapshot{}, ctx.Err()
		}
		return QuotaSnapshot{}, errors.New("read kimi usage response failed")
	}
	if len(data) > maxQuotaBytes {
		return QuotaSnapshot{}, fmt.Errorf("usage response exceeds %d bytes", maxQuotaBytes)
	}
	var payload kimiQuotaResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return QuotaSnapshot{}, errors.New("invalid kimi usage response")
	}
	return decodeKimiQuota(payload)
}

// kimiUsageEndpoint pins the usage request to the connected credential's
// origin and path; authorize re-checks the confinement when signing.
func kimiUsageEndpoint(baseURL string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Host == "" || base.User != nil ||
		(base.Scheme != "https" && base.Scheme != "http") ||
		base.RawQuery != "" || base.ForceQuery || base.Fragment != "" {
		return "", errors.New("invalid kimi credential base URL; reconnect the provider")
	}
	base.Path = strings.TrimSuffix(base.EscapedPath(), "/") + kimiUsagePath
	return base.String(), nil
}

// decodeKimiQuota maps the four usage windows and the optional booster wallet
// onto provider-neutral limits. Windows the account does not report stay
// absent; a response with nothing to show is an observation of nothing, not a
// zeroed snapshot.
func decodeKimiQuota(payload kimiQuotaResponse) (QuotaSnapshot, error) {
	var limits []QuotaLimit
	for _, window := range []struct {
		entry *kimiQuotaEntry
		label string
	}{
		{payload.Usages.Limit5h, "5 hours"},
		{payload.Usages.Limit7d, "7 days"},
		{payload.Usages.LimitMonthTotal, "month"},
		{payload.Usages.LimitMonthCode, "month · code"},
	} {
		if window.entry == nil {
			continue
		}
		// used_ratio is a 0..1 share; renderers read UsedPercent like the
		// codex percent windows do.
		limit := QuotaLimit{
			Window:      window.label,
			Unit:        "percent",
			UsedPercent: max(0, min(100, float64(window.entry.UsedRatio)*100)),
		}
		if reset, err := time.Parse(time.RFC3339, window.entry.ResetTime); err == nil {
			limit.ResetsAt = reset
		}
		limits = append(limits, limit)
	}
	if wallet := decodeKimiBoosterWallet(payload.BoosterWallet); wallet != nil {
		limits = append(limits, *wallet)
	}
	if len(limits) == 0 {
		return QuotaSnapshot{}, errors.New("usage response contains no usage windows")
	}
	return QuotaSnapshot{Limits: limits}, nil
}

// decodeKimiBoosterWallet maps the optional top-up wallet to one currency
// limit. The monthly charge-limit knobs are confirmed by the source but name
// no displayable budget on their own, so they stay off the snapshot; the
// wallet itself appears only when it actually holds a balance.
func decodeKimiBoosterWallet(wallet *kimiBoosterWallet) *QuotaLimit {
	if wallet == nil || wallet.Balance.Type != "BOOSTER" || wallet.Balance.Amount <= 0 {
		return nil
	}
	total := kimiFixedPointToCents(wallet.Balance.Amount)
	balance := kimiFixedPointToCents(wallet.Balance.AmountLeft)
	currency := firstNonEmpty(
		wallet.MonthlyChargeLimit.Currency, wallet.MonthlyUsed.Currency, "USD",
	)
	return &QuotaLimit{
		Window:    "top-up wallet",
		Used:      total - balance,
		Remaining: balance,
		Total:     total,
		Unit:      currency,
	}
}

// kimiFixedPointToCents converts the wallet's fixed-point cents; a sub-cent
// remainder rounds up to one cent so a non-zero balance never reads as zero
// (same rule as the reference client's fixedPointToCents).
func kimiFixedPointToCents(value int64) int64 {
	cents := float64(value) / kimiFixedPointCents
	if cents > 0 && cents < 1 {
		return 1
	}
	return int64(cents + 0.5)
}
