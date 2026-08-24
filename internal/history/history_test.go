package history

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendPersistsJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt-history.jsonl")
	s := Open(path)
	s.Append("первый")
	s.Append("second prompt")
	s.Append("second prompt") // consecutive duplicate dropped
	s.Append("   ")           // blank dropped
	s.Append("\n\t")
	require.Equal(t, 2, s.Len())

	reloaded := Open(path)
	require.Equal(t, []string{"первый", "second prompt"}, reloaded.Entries())
}

func TestOpenMissingFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.jsonl")
	s := Open(path)
	require.Equal(t, 0, s.Len())
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Open must not create the file, stat err=%v", err)
	}
}

func TestOpenSelfHealsCorruptLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt-history.jsonl")
	raw := "{not json\n{\"input\":\"ok\"}\n\n{\"input\": 42}\n"
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	s := Open(path)
	require.Equal(t, []string{"ok"}, s.Entries())

	// The rewrite drops corrupt lines so the file stops failing to parse.
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, `{"input":"ok"}`, string(b))
}

func TestAppendCapsAtMaxEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt-history.jsonl")
	s := Open(path)
	for i := range 60 {
		s.Append(fmt.Sprintf("p%d", i))
	}
	require.Equal(t, MaxEntries, s.Len())
	require.Equal(t, "p10", s.Entries()[0])  // oldest survivors start at p10
	require.Equal(t, "p59", s.Entries()[49]) // newest last

	// The trim rewrites the whole file; a reload sees the same window.
	reloaded := Open(path)
	require.Equal(t, s.Entries(), reloaded.Entries())
}

func TestPrevNextWalk(t *testing.T) {
	s := Open("")

	_, ok := s.Prev("my draft")
	require.False(t, ok, "empty history never recalls")

	s.Append("a")
	s.Append("b")

	// Up starts the walk from a draft of any shape — empty or typed — and
	// captures it for the way back (bash-like).
	got, ok := s.Prev("my draft")
	require.True(t, ok)
	require.Equal(t, "b", got)
	got, ok = s.Prev(got)
	require.True(t, ok)
	require.Equal(t, "a", got)

	// At the oldest end the walk stops without changing the slot.
	_, ok = s.Prev(got)
	require.False(t, ok)

	// Down walks back toward the draft…
	got, ok = s.Next(got)
	require.True(t, ok)
	require.Equal(t, "b", got)
	// …and past the newest entry restores the captured draft.
	got, ok = s.Next(got)
	require.True(t, ok)
	require.Equal(t, "my draft", got)

	// At the draft slot Down is a no-op.
	_, ok = s.Next("my draft")
	require.False(t, ok)
}

func TestPrevRefusesDivergedText(t *testing.T) {
	s := Open("")
	s.Append("x")

	got, ok := s.Prev("")
	require.True(t, ok)
	require.Equal(t, "x", got)

	// The composer text no longer matches the recalled slot — the walk
	// refuses instead of yanking the user's edit.
	_, ok = s.Prev("edited")
	require.False(t, ok)

	// The walk is still at slot "x": Next brings the draft back.
	got, ok = s.Next("")
	require.True(t, ok)
	require.Empty(t, got)
}

func TestAppendResetsWalk(t *testing.T) {
	s := Open("")
	s.Append("old")
	got, ok := s.Prev("")
	require.True(t, ok)
	require.Equal(t, "old", got)

	// Submitting resets the walk to the draft slot…
	s.Append("new")
	_, ok = s.Next("new")
	require.False(t, ok, "walk must be back at the draft slot after Append")

	// …and the next Up recalls the newest entry.
	got, ok = s.Prev("")
	require.True(t, ok)
	require.Equal(t, "new", got)
}

func TestResetReturnsToDraft(t *testing.T) {
	s := Open("")
	s.Append("a")
	s.Append("b")
	got, ok := s.Prev("")
	require.True(t, ok)
	require.Equal(t, "b", got)

	s.Reset()
	_, ok = s.Next("b")
	require.False(t, ok, "Reset must return the walk to the draft slot")
	require.Equal(t, 2, s.Len())

	got, ok = s.Prev("")
	require.True(t, ok)
	require.Equal(t, "b", got)
}

func TestNilStoreIsInert(t *testing.T) {
	var s *Store
	s.Append("x")
	s.Reset()
	require.Equal(t, 0, s.Len())
	require.Empty(t, s.Entries())
	_, ok := s.Prev("")
	require.False(t, ok)
	_, ok = s.Next("")
	require.False(t, ok)
}
