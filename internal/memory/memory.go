package memory

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	// IndexFile is the generated, human-facing index of a memory directory.
	// The harness owns it; the agent owns the fact files beside it.
	IndexFile = "MEMORY.md"
	fileExt   = ".md"
)

// Kind is what a memory is about. The four kinds are the whole taxonomy:
// anything that fits none of them is not worth remembering.
type Kind string

const (
	// KindUser is who the user is: role, expertise, standing preferences.
	KindUser Kind = "user"
	// KindFeedback is guidance on how the agent should work, with the why.
	KindFeedback Kind = "feedback"
	// KindProject is ongoing work, goals and constraints that the code and
	// git history do not already record.
	KindProject Kind = "project"
	// KindReference points at something outside the repo: a URL, a dashboard,
	// a ticket.
	KindReference Kind = "reference"
)

// kindOrder fixes both index grouping and recall tie-breaks: who the user is
// outranks how to work, which outranks what is being worked on.
var kindOrder = []Kind{KindUser, KindFeedback, KindProject, KindReference}

// ParseKind maps frontmatter text onto a Kind. An unknown or missing value
// becomes KindProject rather than an error: a fact with a wrong label is still
// a fact, and dropping it would lose what the agent tried to keep.
func ParseKind(value string) Kind {
	k := Kind(strings.ToLower(strings.TrimSpace(value)))
	if slices.Contains(kindOrder, k) {
		return k
	}
	return KindProject
}

func (k Kind) rank() int {
	if i := slices.Index(kindOrder, k); i >= 0 {
		return i
	}
	return len(kindOrder)
}

// Entry is one remembered fact.
type Entry struct {
	Name        string   // frontmatter name, or the file's base name
	Description string   // one line, the text recall scores against
	Kind        Kind     // user | feedback | project | reference
	Body        string   // the fact itself, Markdown
	Path        string   // absolute path of the file
	File        string   // base name of the file
	Links       []string // [[other-memory]] names found in the body
	Modified    time.Time
	// Pinned marks a memory the harness may never demote: it stays in the
	// prompt whatever the budget and whatever the usage history says.
	Pinned bool
}

// Usage is how the harness tells memory what is actually being used: a fact
// recalled into a turn, or read back on purpose. It is what keeps decay from
// being a function of age alone — a memory that waits three months for its
// topic is not stale, it is patient.
//
// A nil Usage disables decay entirely: nothing is ever stale, and the prompt
// keeps whatever was written most recently.
type Usage interface {
	Use(name string)
	Seen(name string) (count int, last time.Time)
}

// Store is one project's memory directory.
//
// It is deliberately forgiving: a directory that cannot be read, or a file
// that is not a memory, costs the caller nothing. Memory is an accessory to a
// turn, never a precondition for one.
type Store struct {
	dir string
	use Usage

	// mu guards the parsed index and the directory state it was built from:
	// the TUI reads the store while a turn scores against it.
	mu       sync.Mutex
	cached   *index
	dirty    bool
	dirMod   time.Time
	verified time.Time
	// overlapFor is the index version the merge candidates were computed for.
	// Finding them walks posting lists, so it happens once per change, not
	// once per prompt.
	overlapFor  uint64
	overlapping []Overlap
}

// Open prepares dir as a memory store and refreshes its index. The directory
// is created if missing, so the system prompt can promise the agent that
// writing a file there just works.
//
// use may be nil; see Usage for what that turns off.
func Open(dir string, use Usage) (*Store, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("memory: empty directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("memory: resolve %q: %w", dir, err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("memory: create %q: %w", abs, err)
	}
	store := &Store{dir: abs, use: use}
	if _, err := store.SyncIndex(); err != nil {
		return nil, err
	}
	return store, nil
}

// Dir returns the directory the store owns. Empty for a nil store.
func (s *Store) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

// Entries returns every readable memory, ordered by kind then name, from the
// cached index — unreadable and malformed files are skipped and logged there,
// because one bad file must never hide the rest.
//
// The slice belongs to the index: read it, do not mutate it.
func (s *Store) Entries() []Entry {
	return s.index().entries
}

// Used records that a memory was applied: recall calls it for what it
// surfaces, the memory tool for what it reads back. It is the only thing that
// keeps a memory off the stale list, so it is recorded where a memory actually
// reaches the model — not where a human merely lists one.
func (s *Store) Used(name string) {
	if s == nil || s.use == nil {
		return
	}
	s.use.Use(name)
}

// priority is how the prompt decides what to keep when a budget runs out:
// pinned first, then how useful a memory has proven, then how recently it was
// written. Frequency saturates and recency decays, the same shape the rest of
// the harness ranks history by.
//
// A memory just written scores on recency alone — it has not had a chance to
// be used yet, and demoting it for that would be a trap.
func (s *Store) priority(entry Entry, now time.Time) float64 {
	if entry.Pinned {
		return math.Inf(1)
	}
	score := math.Exp(-float64(now.Sub(entry.Modified)) / float64(usageWindow))
	if s == nil || s.use == nil {
		return score
	}
	count, last := s.use.Seen(entry.Name)
	if count > 0 && !last.IsZero() {
		score += math.Log1p(float64(count)) + math.Exp(-float64(now.Sub(last))/float64(usageWindow))
	}
	return score
}

// Fact returns one memory by name, matching either the frontmatter name or
// the file name, with or without the extension: the model reads back what an
// index row named, and rows name both.
func (s *Store) Fact(name string) (Entry, bool) {
	name = strings.TrimSuffix(strings.TrimSpace(name), fileExt)
	for _, entry := range s.Entries() {
		if strings.EqualFold(entry.Name, name) ||
			strings.EqualFold(strings.TrimSuffix(entry.File, fileExt), name) {
			return entry, true
		}
	}
	return Entry{}, false
}

// byKindThenName is the one order everything reads in: the index file, the
// prompt, and the document ids the scorer breaks ties on.
func byKindThenName(a, b Entry) int {
	if c := cmp.Compare(a.Kind.rank(), b.Kind.rank()); c != 0 {
		return c
	}
	return cmp.Compare(a.Name, b.Name)
}

// SyncIndex rewrites MEMORY.md from the files on disk and reports whether the
// index changed. The agent never maintains the index by hand, so it cannot
// drift from what is actually stored.
func (s *Store) SyncIndex() (bool, error) {
	if s == nil {
		return false, nil
	}
	path := filepath.Join(s.dir, IndexFile)
	want := renderIndex(s.Entries())
	if current, err := os.ReadFile(path); err == nil && string(current) == want {
		return false, nil
	}
	// Owner-only, like sessions and config: memory is the user's data.
	if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
		return false, fmt.Errorf("memory: write index %s: %w", path, err)
	}
	// The index file lives in the same directory, so writing it moves the
	// directory mtime. Re-mark, or the next read re-parses every file for a
	// change it made itself.
	s.mu.Lock()
	s.mark()
	s.mu.Unlock()
	return true, nil
}

func renderIndex(entries []Entry) string {
	var sb strings.Builder
	sb.WriteString("# Memory index\n\n")
	sb.WriteString("Generated by cozyphi from the memory files in this directory.\n")
	sb.WriteString("Edit a memory in its own file; changes made here are overwritten.\n")
	if len(entries) == 0 {
		sb.WriteString("\nNo memories saved yet.\n")
		return sb.String()
	}
	for _, kind := range kindOrder {
		group := kindGroup(entries, kind)
		if len(group) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "\n## %s\n\n", kind)
		for _, entry := range group {
			fmt.Fprintf(&sb, "- [%s](%s) — %s\n", entry.Name, entry.File, oneLine(entry.Description))
		}
	}
	return sb.String()
}

func kindGroup(entries []Entry, kind Kind) []Entry {
	var group []Entry
	for _, entry := range entries {
		if entry.Kind == kind {
			group = append(group, entry)
		}
	}
	return group
}

// oneLine keeps an index row on one line even when the description was
// written as a folded block or carries stray whitespace.
func oneLine(text string) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if text == "" {
		return "(no description)"
	}
	return text
}
