// Package history is the composer's prompt history: submitted prompts kept
// newest-last, capped, persisted as JSON lines — cozyphi's port of opencode's
// prompt-history.jsonl (packages/tui/src/prompt/history.tsx).
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alvnukov/cozyphi/internal/debuglog"
)

// MaxEntries caps the history (opencode MAX_HISTORY_ENTRIES).
const MaxEntries = 50

// entry is one JSON line on disk. The shape mirrors opencode's PromptInfo
// input field so both tools can read the same file.
type entry struct {
	Input string `json:"input"`
}

// Store is a walkable prompt history. The walk starts at the draft slot
// (pos 0): Prev steps to the newest submission and older, Next steps back and
// finally restores the draft captured when the walk left the draft slot.
// A walk started from a '/'-leading draft visits only slash entries, so Up
// from "/" is a slash-command history; any other draft walks everything.
// Every method tolerates a nil *Store, so a failed Open degrades to no
// history instead of nil checks at call sites.
type Store struct {
	path    string   // "" keeps everything in memory
	entries []string // oldest first, newest last
	pos     int      // 0 = draft slot, 1..len(walk) = distance into the past
	draft   string   // composer text captured by the first Prev
	// walk is the slice the current walk visits — every entry, or only the
	// slash commands when the walk started from a '/'-leading draft. nil
	// means no walk is in progress.
	walk []string
}

// DefaultPath returns ~/.cozyphi/prompt-history.jsonl, or "" when the home
// directory is unknown (the store then stays in memory).
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".cozyphi", "prompt-history.jsonl")
}

// Open loads the history at path, best-effort. Corrupt lines are dropped and
// the file rewritten so a damaged history heals itself; an overlong file is
// trimmed. A missing file is not an error — the store starts empty.
func Open(path string) *Store {
	s := &Store{path: path}
	if path == "" {
		return s
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			debuglog.Logf("history read %s: %v", path, err)
		}
		return s
	}
	dropped := 0
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			dropped++
			continue
		}
		input := strings.TrimSpace(e.Input)
		if input == "" {
			dropped++
			continue
		}
		s.entries = append(s.entries, input)
	}
	if over := len(s.entries) - MaxEntries; over > 0 {
		s.entries = s.entries[over:]
		dropped++
	}
	if dropped > 0 {
		s.rewrite()
	}
	return s
}

// Append records a submission: trimmed, non-empty, and not a consecutive
// duplicate. It resets any walk in progress and persists best-effort.
func (s *Store) Append(text string) {
	if s == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if n := len(s.entries); n > 0 && s.entries[n-1] == text {
		s.Reset()
		return
	}
	s.entries = append(s.entries, text)
	if len(s.entries) > MaxEntries {
		s.entries = s.entries[len(s.entries)-MaxEntries:]
	}
	s.Reset()
	s.rewrite()
}

// Len reports how many entries the history holds.
func (s *Store) Len() int {
	if s == nil {
		return 0
	}
	return len(s.entries)
}

// Entries returns a copy of the history, oldest first.
func (s *Store) Entries() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.entries...)
}

// walkView picks the entries a new walk visits: a '/'-leading draft walks
// slash commands only, anything else walks the full history.
func (s *Store) walkView(draft string) []string {
	if !strings.HasPrefix(draft, "/") {
		return s.entries
	}
	var out []string
	for _, e := range s.entries {
		if strings.HasPrefix(e, "/") {
			out = append(out, e)
		}
	}
	return out
}

// slot is the entry the walk currently sits on; pos 1 is the walk's newest.
func (s *Store) slot() string {
	return s.walk[len(s.walk)-s.pos]
}

// Prev recalls one entry older than the draft. Up starts the walk from a
// draft of any shape — empty or typed — and captures the draft for the way
// back (bash-like); a '/'-leading draft walks only slash entries. Mid-walk,
// a draft that no longer matches the current slot refuses further steps, so
// edits are never yanked. ok is false without walkable entries and at the
// oldest one.
func (s *Store) Prev(draft string) (string, bool) {
	if s == nil {
		return "", false
	}
	if s.walk == nil {
		s.walk = s.walkView(draft)
		if len(s.walk) == 0 {
			s.Reset()
			return "", false
		}
		s.draft = draft
	} else if draft != s.slot() && draft != "" {
		return "", false
	}
	if s.pos >= len(s.walk) {
		return "", false
	}
	s.pos++
	return s.slot(), true
}

// Next walks back toward the draft slot; stepping off the newest entry
// restores the draft captured by the first Prev. ok is false at the draft
// slot and on the same text-divergence refusal as Prev.
func (s *Store) Next(draft string) (string, bool) {
	if s == nil || s.pos == 0 {
		return "", false
	}
	if draft != s.slot() && draft != "" {
		return "", false
	}
	s.pos--
	if s.pos == 0 {
		draft := s.draft
		s.Reset()
		return draft, true
	}
	return s.slot(), true
}

// Search returns every entry containing query as a substring,
// case-insensitive, newest first. An empty query matches nothing — like
// bash's reverse-i-search, the matches begin once you type.
func (s *Store) Search(query string) []string {
	if s == nil || query == "" {
		return nil
	}
	q := strings.ToLower(query)
	var out []string
	for _, e := range slices.Backward(s.entries) {
		if strings.Contains(strings.ToLower(e), q) {
			out = append(out, e)
		}
	}
	return out
}

// Reset returns the walk to the draft slot without recording anything.
func (s *Store) Reset() {
	if s == nil {
		return
	}
	s.pos = 0
	s.draft = ""
	s.walk = nil
}

// rewrite persists the whole history; the file is at most MaxEntries short
// lines, so one write path beats incremental appends and self-heals by
// construction.
func (s *Store) rewrite() {
	if s == nil || s.path == "" {
		return
	}
	var b strings.Builder
	for _, e := range s.entries {
		line, err := json.Marshal(entry{Input: e})
		if err != nil {
			continue
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		debuglog.Logf("history mkdir %s: %v", s.path, err)
		return
	}
	if err := os.WriteFile(s.path, []byte(b.String()), 0o600); err != nil {
		debuglog.Logf("history write %s: %v", s.path, err)
	}
}
