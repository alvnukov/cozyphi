package session

// CompactionStats is what the compactions on the current context path did,
// in counters and nothing else. A Compaction also carries the summary text
// that replaced the history, the entry ids it kept or dropped and the file
// lists it recorded; all three describe the user's work, so none of them
// leave the manager through here. What is left is the arithmetic: how much
// context there was, how much remained, and how many messages that took.
type CompactionStats struct {
	// Count is how many compaction entries the projected context carries.
	// A trim the user asked for is one of them.
	Count int
	// Known is whether the members below describe a real compaction. Without
	// one they are all zero, and zero from a compaction that measured nothing
	// is a different answer from zero because none ever ran.
	Known bool
	// FromTrim is true when the latest one was the user cutting context by
	// hand rather than a generated summary. Its numbers read differently:
	// nothing was summarized, history was simply dropped.
	FromTrim bool
	// TokensBefore and TokensAfter are what the latest compaction measured
	// on either side of itself.
	TokensBefore int
	TokensAfter  int
	// MessagesSummarized and MessagesKept are what it did with the history:
	// how many messages became the summary, how many rode through verbatim.
	MessagesSummarized int
	MessagesKept       int
}

// CompactionStats reports the compactions on the path the model's context is
// built from, the latest one winning. It is a read-only walk of entries the
// manager already holds: asking compacts nothing, summarizes nothing, writes
// nothing to the log and reads nothing back from disk.
func (sm *Manager) CompactionStats() CompactionStats {
	if sm == nil {
		return CompactionStats{}
	}
	stats := CompactionStats{}
	for _, entry := range sm.BuildContext() {
		compacted, ok := entry.(CompactionEntry)
		if !ok {
			continue
		}
		stats.Count++
		stats.Known = true
		stats.FromTrim = compacted.Compaction.FromTrim
		stats.TokensBefore = compacted.Compaction.TokensBefore
		stats.TokensAfter = compacted.Compaction.TokensAfter
		stats.MessagesSummarized = compacted.Compaction.MessagesSummarized
		stats.MessagesKept = compacted.Compaction.MessagesKept
	}
	return stats
}
