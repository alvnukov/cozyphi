package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/session"
)

// contextFillLevel ranks context-window pressure for the fill label.
type contextFillLevel int

const (
	contextFillOK contextFillLevel = iota
	contextFillRecommend
	contextFillWarning
	contextFillDanger
)

// formatTokens formats counts like panda: 999, 1.2k, 15k, 1.5M.
func formatTokens(count int) string {
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

func contextFillRatio(used, window int) float64 {
	if window <= 0 || used <= 0 {
		return 0
	}
	return float64(used) / float64(window)
}

func contextFillLevelFor(ratio float64, window int) contextFillLevel {
	// Tiers scaled by window size: very large windows tolerate higher fills.
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
		return contextFillDanger
	case ratio >= warning:
		return contextFillWarning
	case ratio >= recommend:
		return contextFillRecommend
	default:
		return contextFillOK
	}
}

// formatContextLabel builds a "context: 4% of 128k" fill label (empty when unknown).
func formatContextLabel(usage session.TokenUsage, window int) string {
	if window <= 0 {
		return ""
	}
	used := usage.ContextTokens()
	if used <= 0 {
		return ""
	}
	pct := min(max(int(contextFillRatio(used, window)*100), 0), 100)
	if window >= 1000 {
		return fmt.Sprintf("%d%%/%s", pct, formatTokens(window))
	}
	return fmt.Sprintf("%d%%", pct)
}

// formatUsageStats builds panda-style "↑1.2k ↓800 C900 Σ2.0k" (empty when unknown).
func formatUsageStats(usage session.TokenUsage) string {
	if !usage.Reported() {
		return ""
	}
	parts := make([]string, 0, 4)
	if usage.PromptTokens > 0 {
		parts = append(parts, "↑"+formatTokens(usage.PromptTokens))
	}
	if usage.CompletionTokens > 0 {
		parts = append(parts, "↓"+formatTokens(usage.CompletionTokens))
	}
	if usage.CachedTokens > 0 {
		parts = append(parts, "C"+formatTokens(usage.CachedTokens))
	}
	total := usage.TotalTokens
	if total <= 0 {
		total = usage.PromptTokens + usage.CompletionTokens
	}
	if total > 0 {
		parts = append(parts, "Σ"+formatTokens(total))
	}
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(parts[0])
	for i := 1; i < len(parts); i++ {
		b.WriteString(" " + parts[i])
	}
	return b.String()
}

func pathLabelStyle(th components.Theme) xui.Style {
	// Muted without Dim so the cwd stays readable on dark borders.
	st := th.Muted
	st.Dim = false
	return st
}

func contextLabelStyle(th components.Theme, usage session.TokenUsage, window int) xui.Style {
	used := usage.ContextTokens()
	ratio := contextFillRatio(used, window)
	switch contextFillLevelFor(ratio, window) {
	case contextFillDanger:
		return th.Destructive
	case contextFillWarning:
		return th.Warning
	case contextFillRecommend:
		st := th.Accent
		st.Underline = false
		return st
	default:
		st := th.ToolName
		st.Bold = false
		return st
	}
}

// joinBorderParts concatenates non-empty label fragments with a single space.
func joinBorderParts(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += " "
		}
		out += p
	}
	return out
}
