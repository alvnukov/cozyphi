package tui

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/session"
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
		if got := formatTokens(n); got != want {
			t.Fatalf("formatTokens(%d)=%q want %q", n, got, want)
		}
	}
}

func TestFormatContextLabel(t *testing.T) {
	u := session.TokenUsage{PromptTokens: 5120, TotalTokens: 6000}
	got := formatContextLabel(u, 128000)
	if got != "4%/128k" {
		t.Fatalf("got %q", got)
	}
	if formatContextLabel(session.TokenUsage{}, 128000) != "" {
		t.Fatal("empty usage should hide label")
	}
	if formatContextLabel(u, 0) != "" {
		t.Fatal("zero window should hide label")
	}
}

func TestFormatUsageStats(t *testing.T) {
	got := formatUsageStats(session.TokenUsage{
		PromptTokens:     1200,
		CompletionTokens: 800,
		TotalTokens:      2000,
	})
	if got != "↑1.2k ↓800 Σ2.0k" {
		t.Fatalf("got %q", got)
	}
	got = formatUsageStats(session.TokenUsage{
		PromptTokens:     1200,
		CompletionTokens: 800,
		CachedTokens:     900,
		TotalTokens:      2000,
	})
	if got != "↑1.2k ↓800 C900 Σ2.0k" {
		t.Fatalf("got %q", got)
	}
}

func TestJoinBorderParts(t *testing.T) {
	if got := joinBorderParts(
		"↑1.2k ↓800 Σ2.0k",
		"context: 4% of 128k",
	); got != "↑1.2k ↓800 Σ2.0k context: 4% of 128k" {
		t.Fatalf("got %q", got)
	}
	if got := joinBorderParts("", "context: 4% of 128k"); got != "context: 4% of 128k" {
		t.Fatalf("got %q", got)
	}
	if got := joinBorderParts("↑1.2k", ""); got != "↑1.2k" {
		t.Fatalf("got %q", got)
	}
	if got := joinBorderParts("", ""); got != "" {
		t.Fatalf("got %q", got)
	}
}
