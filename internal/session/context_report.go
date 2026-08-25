package session

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// previewWidth bounds the one-line preview each context item shows.
const previewWidth = 80

// ContextItem describes one entry exactly as the model receives it on the
// next request: its entry ID (usable for trim), display kind, first-line
// preview, estimated token cost, and the running total up to and including it.
type ContextItem struct {
	EntryID          string
	Kind             string // summary | user | assistant | tool
	Preview          string
	Tokens           int
	CumulativeTokens int
}

// ContextReport is the itemized shape of the current LLM context. The token
// numbers are estimates (len(json)/4) shared with compaction's cut logic;
// provider-accurate totals live in the engine's usage stats and are merged in
// by callers that have them.
type ContextReport struct {
	Items           []ContextItem
	EstimatedTokens int
	LastCompaction  *Compaction
}

// EstimateMessageTokens returns the shared rough token cost of a message:
// one token per four bytes of its JSON wire form, floored at one. Every
// consumer of per-message token numbers (context browser, compaction cut)
// goes through here so the numbers cannot drift apart.
func EstimateMessageTokens(msg llm.Message) int {
	raw, err := json.Marshal(msg)
	if err != nil {
		// Message contains only JSON-safe fields today. Failing closed to one
		// token keeps cut decisions valid if that contract changes.
		return 1
	}
	return max((len(raw)+3)/4, 1)
}

// InspectContext itemizes what BuildContext would send to the model, oldest
// first. It is a read-only projection; callers render or act on it (trim,
// compact) without touching the append-only log.
func (sm *Manager) InspectContext() ContextReport {
	entries := sm.BuildContext()

	report := ContextReport{}
	var lastCompaction *Compaction
	running := 0
	for _, entry := range entries {
		item := ContextItem{EntryID: entry.GetID()}
		switch e := entry.(type) {
		case CompactionEntry:
			item.Kind = "summary"
			item.Preview = previewLine(e.Compaction.Summary)
			lastCompaction = &e.Compaction
		case SessionMessageEntry:
			item.Kind = string(e.Message.Role)
			item.Preview = previewLine(e.Message.Content)
		default:
			continue
		}
		item.Tokens = estimateEntryTokens(entry)
		running += item.Tokens
		item.CumulativeTokens = running
		report.Items = append(report.Items, item)
	}
	report.EstimatedTokens = running
	report.LastCompaction = lastCompaction
	return report
}

// TrimContextFrom drops everything before the entry with the given ID from
// the model's context, keeping that entry onward. It is append-only like any
// other edit of the context: a compaction entry whose summary is a short
// user-facing note instead of an LLM summary, so the audit log stays intact
// and the model sees an honest marker instead of silently missing history.
func (sm *Manager) TrimContextFrom(entryID string) (string, error) {
	sm.mu.Lock()
	_, ok := sm.byIDs[entryID]
	sm.mu.Unlock()
	if !ok {
		return "", errUnknownTrimEntry(entryID)
	}

	return sm.AppendCompaction(Compaction{
		Summary: "Context trimmed by the user at " + time.Now().
			Format("15:04") +
			"; earlier messages were dropped.",
		FirstKeptEntryID: entryID,
		FromTrim:         true,
	})
}

func errUnknownTrimEntry(entryID string) error {
	return &UnknownTrimEntryError{EntryID: entryID}
}

// UnknownTrimEntryError reports a trim requested against an entry that is not
// part of this session.
type UnknownTrimEntryError struct{ EntryID string }

func (e *UnknownTrimEntryError) Error() string {
	return "session: unknown entry to trim from: " + e.EntryID
}

func estimateEntryTokens(entry MessageEntry) int {
	switch e := entry.(type) {
	case SessionMessageEntry:
		return EstimateMessageTokens(e.Message)
	case CompactionEntry:
		return EstimateMessageTokens(llm.Message{Role: llm.RoleUser, Content: e.Compaction.Summary})
	default:
		return 1
	}
}

func previewLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) <= previewWidth {
		return s
	}
	return s[:previewWidth]
}
