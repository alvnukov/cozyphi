package compaction

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettingsThreshold(t *testing.T) {
	require.Equal(t, 90, Settings{enabled: true, reverseTokens: 10}.Threshold(100))
	require.Equal(t, 0, Settings{enabled: true, reverseTokens: 10}.Threshold(0), "unknown window has no threshold")
	require.Equal(t, 0, Settings{}.Threshold(100), "disabled compaction has no threshold")
	require.Equal(t, 0, Settings{enabled: true, reverseTokens: 200}.Threshold(100), "threshold clamps at zero")
	require.Equal(t, 131072-16384, DefaultSettings().Threshold(131072))
}

func TestShouldCompact(t *testing.T) {
	settings := Settings{
		enabled:          true,
		reverseTokens:    10,
		keepRecentTokens: 20000,
	}

	// threshold = contextWindow - reverseTokens = 90
	if ShouldCompact(50, 100, settings) {
		t.Fatalf("expected ShouldCompact=false when contextTokens below threshold")
	}
	if ShouldCompact(90, 100, settings) {
		t.Fatalf("expected ShouldCompact=false when contextTokens equals threshold")
	}
	if !ShouldCompact(95, 100, settings) {
		t.Fatalf("expected ShouldCompact=true when contextTokens above threshold")
	}

	disabled := settings
	disabled.enabled = false
	if ShouldCompact(95, 100, disabled) {
		t.Fatalf("expected ShouldCompact=false when compaction disabled")
	}

	if ShouldCompact(95, 0, settings) {
		t.Fatalf("expected ShouldCompact=false when contextWindow <= 0")
	}

	// threshold clamping when reverseTokens > contextWindow
	settings2 := settings
	settings2.reverseTokens = 200
	// threshold becomes 0
	if ShouldCompact(0, 100, settings2) {
		t.Fatalf("expected ShouldCompact=false when contextTokens==0 and threshold clamped to 0")
	}
	if !ShouldCompact(1, 100, settings2) {
		t.Fatalf("expected ShouldCompact=true when contextTokens>0 and threshold clamped to 0")
	}
}
