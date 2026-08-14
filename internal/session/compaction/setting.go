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

// ShouldCompact reports whether contextTokens warrants compaction given
// contextWindow and settings.
func ShouldCompact(contextTokens, contextWindow int, settings Settings) bool {
	if !settings.enabled || contextWindow <= 0 {
		return false
	}

	// Keep `reverseTokens` headroom from the context window. When current usage
	// exceeds (contextWindow - reverseTokens), we should compact.
	threshold := max(contextWindow-settings.reverseTokens, 0)
	return contextTokens > threshold
}
