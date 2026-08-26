package tokens

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestFormatTokens(t *testing.T) {
	cases := map[int]string{
		0:       "0",
		999:     "999",
		1200:    "1.2k",
		15000:   "15k",
		1500000: "1.5M",
	}
	for n, want := range cases {
		if got := FormatTokens(n); got != want {
			t.Fatalf("FormatTokens(%d)=%q want %q", n, got, want)
		}
	}
}

func TestFormatContextLabel(t *testing.T) {
	u := session.TokenUsage{PromptTokens: 5120, TotalTokens: 6000}
	got := FormatContextLabel(u, 128000)
	if got != "4%/128k" {
		t.Fatalf("got %q", got)
	}
	if FormatContextLabel(session.TokenUsage{}, 128000) != "" {
		t.Fatal("empty usage should hide label")
	}
	if FormatContextLabel(u, 0) != "" {
		t.Fatal("zero window should hide label")
	}
	u.Estimated = true
	if got := FormatContextLabel(u, 128000); got != "~4%/128k" {
		t.Fatalf("estimated context label = %q", got)
	}
}

func TestFormatUsageStats(t *testing.T) {
	got := FormatUsageStats(session.TokenUsage{
		PromptTokens:     1200,
		CompletionTokens: 800,
		TotalTokens:      2000,
	})
	if got != "↑1.2k ↓800 Σ2.0k" {
		t.Fatalf("got %q", got)
	}
	got = FormatUsageStats(session.TokenUsage{
		PromptTokens:     1200,
		CompletionTokens: 800,
		CachedTokens:     900,
		TotalTokens:      2000,
	})
	if got != "↑1.2k ↓800 C900 Σ2.0k" {
		t.Fatalf("got %q", got)
	}
	got = FormatUsageStats(session.TokenUsage{PromptTokens: 3200, TotalTokens: 3200, Estimated: true})
	if got != "~3.2k context" {
		t.Fatalf("estimated usage = %q", got)
	}
}

func TestBreakdownLines(t *testing.T) {
	assert.Equal(t, []string{"in 1.2k", "out 800", "cache 900", "total 2.0k"},
		BreakdownLines(session.TokenUsage{
			PromptTokens:     1200,
			CompletionTokens: 800,
			CachedTokens:     900,
			TotalTokens:      2000,
		}))
	assert.Equal(t, []string{"~3.2k context"},
		BreakdownLines(session.TokenUsage{PromptTokens: 3200, TotalTokens: 3200, Estimated: true}))
	assert.Nil(t, BreakdownLines(session.TokenUsage{}))
}

func TestContextFillLevelFor(t *testing.T) {
	// small windows: 0.8 / 0.9 / 0.95
	assert.Equal(t, FillOK, ContextFillLevelFor(0.5, 128000))
	assert.Equal(t, FillRecommend, ContextFillLevelFor(0.85, 128000))
	assert.Equal(t, FillWarning, ContextFillLevelFor(0.92, 128000))
	assert.Equal(t, FillDanger, ContextFillLevelFor(0.96, 128000))
	// very large windows flip early: 0.2 / 0.8 / 0.9
	assert.Equal(t, FillOK, ContextFillLevelFor(0.1, 1000000))
	assert.Equal(t, FillRecommend, ContextFillLevelFor(0.3, 1000000))
	assert.Equal(t, FillWarning, ContextFillLevelFor(0.85, 1000000))
}

func TestFillStyleTiers(t *testing.T) {
	th := components.DefaultTheme()
	assert.Equal(t, th.Destructive, FillStyle(th, FillDanger))
	assert.Equal(t, th.Warning, FillStyle(th, FillWarning))
	assert.Equal(t, xui.Style{Fg: th.Accent.Fg}, FillStyle(th, FillRecommend))
}
