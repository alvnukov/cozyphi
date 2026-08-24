// Package history is the composer's prompt history: submitted prompts kept
// newest-last, capped, persisted as JSON lines — cozyphi's port of opencode's
// prompt-history.jsonl (packages/tui/src/prompt/history.tsx).
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/pulseaiclub/phi/internal/debuglog"
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
// Every method tolerates a nil *Store, so a failed Open degrades to no
// history instead of nil checks at call sites.
type Store struct {
	path    string   // "" keeps everything in memory
	entries []string // oldest first, newest last
	pos     int      // 0 = draft slot, 1..len = distance into the past
	draft   string   // composer text captured by the first Prev
}

// DefaultPath returns ~/.phi/prompt-history.jsonl, or "" when the home
// directory is unknown (the store then stays in memory).
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".phi", "prompt-history.jsonl")
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

// slot is the entry the walk currently sits on; pos 1 is the newest.
func (s *Store) slot() string {
	return s.entries[len(s.entries)-s.pos]
}

// Prev recalls one entry older than the draft. Up starts the walk from a
// draft of any shape — empty or typed — and captures the draft for the way
// back (bash-like). Mid-walk, a draft that no longer matches the current slot
// refuses further steps, so edits are never yanked. ok is false without
// history and at the oldest entry.
func (s *Store) Prev(draft string) (string, bool) {
	if s == nil || len(s.entries) == 0 || s.pos >= len(s.entries) {
		return "", false
	}
	if s.pos == 0 {
		s.draft = draft
	} else if draft != s.slot() && draft != "" {
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
		s.draft = ""
		return draft, true
	}
	return s.slot(), true
}

// Reset returns the walk to the draft slot without recording anything.
func (s *Store) Reset() {
	if s == nil {
		return
	}
	s.pos = 0
	s.draft = ""
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
