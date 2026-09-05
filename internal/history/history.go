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
	"sync"

	"github.com/alvnukov/cozyphi/internal/debuglog"
)

// MaxEntries caps the history (opencode MAX_HISTORY_ENTRIES).
const MaxEntries = 50

// entry is one JSON line on disk. The shape mirrors opencode's PromptInfo
// input field so both tools can read the same file.
type entry struct {
	Input string `json:"input"`
}

// Store is one navigation cursor over a shared prompt history corpus.
// The walk starts at the draft slot (pos 0): Prev steps to the newest submission
// and older, Next steps back and finally restores the draft captured when the
// walk left the draft slot.
// A walk started from a '/'-leading draft visits only slash entries, so Up
// from "/" is a slash-command history; any other draft walks everything.
// Every method tolerates a nil *Store, so a failed Open degrades to no
// history instead of nil checks at call sites. The zero value is in-memory.
// Methods are safe for concurrent use. A Store must not be copied after use;
// use NewCursor to share its corpus with independent navigation.
type Store struct {
	mu    sync.Mutex // guards this cursor; acquired before the corpus lock
	data  *corpus
	pos   int    // 0 = draft slot, 1..len(walk) = distance into the past
	draft string // composer text captured by the first Prev
	// walk is the slice the current walk visits — every entry, or only the
	// slash commands when the walk started from a '/'-leading draft. nil
	// means no walk is in progress.
	walk []string
}

type corpus struct {
	mu      sync.Mutex // serializes corpus access, including the entire rewrite
	path    string     // "" keeps everything in memory
	entries []string   // oldest first, newest last
}

// NewCursor returns a fresh draft-slot cursor sharing this history's corpus
// and writer, without changing the original cursor. A nil receiver returns nil.
// Walks snapshot the corpus on the first Prev; later submissions appear on the
// next walk. Entries, Len and Search always read the current shared corpus.
func (s *Store) NewCursor() *Store {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return &Store{data: s.corpus()}
}

// corpus keeps the zero value usable; the caller holds s.mu.
func (s *Store) corpus() *corpus {
	if s.data == nil {
		s.data = &corpus{}
	}
	return s.data
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
// Each Open owns a separate corpus/writer, even for the same path. Use
// NewCursor on one opened Store to share it within the process.
func Open(path string) *Store {
	c := &corpus{path: path}
	s := &Store{data: c}
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
		c.entries = append(c.entries, input)
	}
	if over := len(c.entries) - MaxEntries; over > 0 {
		c.entries = c.entries[over:]
		dropped++
	}
	if dropped > 0 {
		c.rewrite()
	}
	return s
}

// Append records a submission: trimmed, non-empty, and not a consecutive
// duplicate across the shared corpus. It resets only this cursor's walk and
// persists best-effort; other cursors keep their current walk and draft.
func (s *Store) Append(text string) {
	if s == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.corpus()
	c.mu.Lock()
	defer c.mu.Unlock()
	if n := len(c.entries); n > 0 && c.entries[n-1] == text {
		s.reset()
		return
	}
	c.entries = append(c.entries, text)
	if len(c.entries) > MaxEntries {
		c.entries = c.entries[len(c.entries)-MaxEntries:]
	}
	s.reset()
	c.rewrite()
}

// Len reports how many entries the history holds.
func (s *Store) Len() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.corpus()
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Entries returns a copy of the history, oldest first.
func (s *Store) Entries() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.corpus().snapshot()
}

func (s *corpus) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.entries...)
}

// walkView snapshots the entries a new walk visits while the caller holds s.mu:
// a '/'-leading draft walks slash commands only, anything else walks the full
// history. Later submissions and eviction must not change a retained walk.
func (s *Store) walkView(draft string) []string {
	entries := s.corpus().snapshot()
	if !strings.HasPrefix(draft, "/") {
		return entries
	}
	var out []string
	for _, e := range entries {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.walk == nil {
		s.walk = s.walkView(draft)
		if len(s.walk) == 0 {
			s.reset()
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
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pos == 0 {
		return "", false
	}
	if draft != s.slot() && draft != "" {
		return "", false
	}
	s.pos--
	if s.pos == 0 {
		draft := s.draft
		s.reset()
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
	for _, e := range slices.Backward(s.Entries()) {
		if strings.Contains(strings.ToLower(e), q) {
			out = append(out, e)
		}
	}
	return out
}

// Reset returns only this cursor to the draft slot without recording anything.
func (s *Store) Reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reset()
}

// reset requires s.mu.
func (s *Store) reset() {
	s.pos = 0
	s.draft = ""
	s.walk = nil
}

// rewrite persists the whole history; the file is at most MaxEntries short
// lines, so one write path beats incremental appends and self-heals by
// construction. The caller holds the corpus lock, or has not published it yet.
func (s *corpus) rewrite() {
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
