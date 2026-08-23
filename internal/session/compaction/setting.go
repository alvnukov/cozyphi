package compaction

// Settings configures compaction: whether it is enabled and the token
// thresholds used to decide when to compact.
type Settings struct {
	enabled          bool
	reverseTokens    int
	keepRecentTokens int
}

var defaultSettings = Settings{
	enabled:          true,
	reverseTokens:    16384,
	keepRecentTokens: 20000,
}

// DefaultSettings returns the default compaction settings for use by callers outside this package.
func DefaultSettings() Settings {
	return defaultSettings
}

// Threshold returns the context-token count at which compaction should fire
// for the given window: the window minus reverseTokens headroom, clamped at
// zero. Returns 0 when compaction is disabled or the window is unknown.
func (s Settings) Threshold(contextWindow int) int {
	if !s.enabled || contextWindow <= 0 {
		return 0
	}
	return max(contextWindow-s.reverseTokens, 0)
}

// ShouldCompact reports whether contextTokens warrants compaction given
// contextWindow and settings.
func ShouldCompact(contextTokens, contextWindow int, settings Settings) bool {
	if !settings.enabled || contextWindow <= 0 {
		return false
	}
	return contextTokens > settings.Threshold(contextWindow)
}
