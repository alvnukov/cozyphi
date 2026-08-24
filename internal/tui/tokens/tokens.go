// Package tokens formats token counts and context-fill tiers for usage
// displays (composer border label, status sidebar). The tiers live here so
// every display colors context pressure the same way.
package tokens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/session"
)

// FillLevel ranks context-window pressure.
type FillLevel int

const (
	FillOK FillLevel = iota
	FillRecommend
	FillWarning
	FillDanger
)

// FormatTokens formats counts like panda: 999, 1.2k, 15k, 1.5M.
func FormatTokens(count int) string {
	if count < 0 {
		count = 0
	}
	if count < 1000 {
		return strconv.Itoa(count)
	}
	if count < 10000 {
		return strconv.FormatFloat(float64(count)/1000, 'f', 1, 64) + "k"
	}
	if count < 1000000 {
		return strconv.Itoa(count/1000) + "k"
	}
	if count < 10000000 {
		return strconv.FormatFloat(float64(count)/1000000, 'f', 1, 64) + "M"
	}
	return strconv.Itoa(count/1000000) + "M"
}

// ContextFillRatio is the share of the context window in use (0 when unknown).
func ContextFillRatio(used, window int) float64 {
	if window <= 0 || used <= 0 {
		return 0
	}
	return float64(used) / float64(window)
}

// ContextFillLevelFor ranks a fill ratio; tiers scale with the window because
// very large windows tolerate higher fills.
func ContextFillLevelFor(ratio float64, window int) FillLevel {
	var recommend, warning, danger float64
	switch {
	case window >= 900000:
		recommend, warning, danger = 0.2, 0.8, 0.9
	case window >= 400000:
		recommend, warning, danger = 0.7, 0.8, 0.9
	default:
		recommend, warning, danger = 0.8, 0.9, 0.95
	}
	switch {
	case ratio >= danger:
		return FillDanger
	case ratio >= warning:
		return FillWarning
	case ratio >= recommend:
		return FillRecommend
	default:
		return FillOK
	}
}

// FillStyle maps a fill level onto the theme slots shared by every usage
// display, so the composer label and the sidebar bar agree on color.
func FillStyle(th components.Theme, level FillLevel) xui.Style {
	switch level {
	case FillDanger:
		return th.Destructive
	case FillWarning:
		return th.Warning
	case FillRecommend:
		st := th.Accent
		st.Underline = false
		return st
	default:
		st := th.ToolName
		st.Bold = false
		return st
	}
}

// FormatContextLabel builds a "4%/128k" fill label (empty when unknown).
func FormatContextLabel(usage session.TokenUsage, window int) string {
	if window <= 0 {
		return ""
	}
	used := usage.ContextTokens()
	if used <= 0 {
		return ""
	}
	pct := min(max(int(ContextFillRatio(used, window)*100), 0), 100)
	prefix := ""
	if usage.Estimated {
		prefix = "~"
	}
	if window >= 1000 {
		return fmt.Sprintf("%s%d%%/%s", prefix, pct, FormatTokens(window))
	}
	return fmt.Sprintf("%s%d%%", prefix, pct)
}

// FormatUsageStats builds panda-style "↑1.2k ↓800 C900 Σ2.0k" (empty when unknown).
func FormatUsageStats(usage session.TokenUsage) string {
	if !usage.Reported() {
		return ""
	}
	if usage.Estimated {
		return "~" + FormatTokens(usage.ContextTokens()) + " context"
	}
	parts := make([]string, 0, 4)
	if usage.PromptTokens > 0 {
		parts = append(parts, "↑"+FormatTokens(usage.PromptTokens))
	}
	if usage.CompletionTokens > 0 {
		parts = append(parts, "↓"+FormatTokens(usage.CompletionTokens))
	}
	if usage.CachedTokens > 0 {
		parts = append(parts, "C"+FormatTokens(usage.CachedTokens))
	}
	total := usage.TotalTokens
	if total <= 0 {
		total = usage.PromptTokens + usage.CompletionTokens
	}
	if total > 0 {
		parts = append(parts, "Σ"+FormatTokens(total))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}
