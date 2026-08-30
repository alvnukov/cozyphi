package compaction

// Settings configures compaction: whether it is enabled and the token
// thresholds used to decide when to compact.
type Settings struct {
	enabled          bool
	reverseTokens    int
	keepRecentTokens int
	// reminderTokens is the user-set context-token count at which the
	// engine starts advising the model to compact; 0 keeps the default.
	reminderTokens int
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

// ConfiguredSettings returns the default settings with a user-set reminder
// threshold: the context-token count at which the engine starts advising the
// model to compact. reminderTokens <= 0 keeps the default — advice starts
// exactly where compaction used to fire on its own.
func ConfiguredSettings(reminderTokens int) Settings {
	s := defaultSettings
	if reminderTokens > 0 {
		s.reminderTokens = reminderTokens
	}
	return s
}

// ReminderThreshold returns the token count at which the compact advice
// starts: the user-set value, or the compaction threshold by default.
// Returns 0 when compaction is disabled or the window is unknown.
func (s Settings) ReminderThreshold(contextWindow int) int {
	if !s.enabled || contextWindow <= 0 {
		return 0
	}
	if s.reminderTokens > 0 {
		return s.reminderTokens
	}
	return s.Threshold(contextWindow)
}

// ShouldRemind reports whether contextTokens warrants the compact advice.
func ShouldRemind(contextTokens, contextWindow int, s Settings) bool {
	threshold := s.ReminderThreshold(contextWindow)
	return threshold > 0 && contextTokens > threshold
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
