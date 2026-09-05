package history_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/history"
)

func TestNewCursorIsolatesNavigationAndDraft(t *testing.T) {
	main := history.Open("")
	main.Append("older")
	main.Append("newer")

	got, ok := main.Prev("main draft")
	require.True(t, ok)
	require.Equal(t, "newer", got)

	child := main.NewCursor()
	got, ok = child.Prev("child draft")
	require.True(t, ok, "a new cursor starts at its own draft slot")
	require.Equal(t, "newer", got)
	got, ok = child.Prev(got)
	require.True(t, ok)
	require.Equal(t, "older", got)

	got, ok = main.Next("newer")
	require.True(t, ok)
	require.Equal(t, "main draft", got)
	main.Reset()

	got, ok = child.Next("older")
	require.True(t, ok)
	require.Equal(t, "newer", got)
	got, ok = child.Next(got)
	require.True(t, ok)
	require.Equal(t, "child draft", got)
}

func TestCursorsSerializeConcurrentAppend(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(fmt.Sprintf("duplicate=%t", duplicate), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "history.jsonl")
			root := history.Open(path)
			cursors := []*history.Store{root, root.NewCursor(), root.NewCursor()}
			const submissions = 24
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range submissions {
				wg.Go(func() {
					<-start
					text := "same prompt"
					if !duplicate {
						text = fmt.Sprintf("prompt %02d", i)
					}
					cursor := cursors[i%len(cursors)]
					cursor.Append(text)
					cursor.Len()
					cursor.Entries()
					cursor.Search("prompt")
					cursor.NewCursor()
					cursor.Prev("")
					cursor.Next("")
					cursor.Reset()
				})
			}
			close(start)
			wg.Wait()

			var want []string
			if duplicate {
				want = []string{"same prompt"}
			} else {
				for i := range submissions {
					want = append(want, fmt.Sprintf("prompt %02d", i))
				}
			}
			require.ElementsMatch(t, want, root.Entries(), "no accepted submission is lost")
			for _, cursor := range cursors {
				require.Equal(t, len(want), cursor.Len())
				require.Equal(t, root.Entries(), cursor.Entries())
			}
			require.Equal(t, root.Entries(), history.Open(path).Entries(), "writer persists the shared order")
		})
	}
}

func TestCursorAppendSharesCorpusAndOnlyResetsItsOwnWalk(t *testing.T) {
	for _, text := range []string{"new", " old \n", " \t"} {
		t.Run(fmt.Sprintf("append=%q", text), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "history.jsonl")
			main := history.Open(path)
			main.Append("old")
			child := main.NewCursor()
			got, ok := main.Prev("main draft")
			require.True(t, ok)
			require.Equal(t, "old", got)
			got, ok = child.Prev("child draft")
			require.True(t, ok)
			require.Equal(t, "old", got)

			child.Append(text)
			got, ok = main.Next("old")
			require.True(t, ok, "another cursor's Append must not reset this walk")
			require.Equal(t, "main draft", got)
			got, ok = child.Next("old")
			if text == " \t" {
				require.True(t, ok, "blank submissions leave even the submitting cursor alone")
				require.Equal(t, "child draft", got)
			} else {
				require.False(t, ok, "nonblank submissions reset the submitting cursor, including duplicates")
			}

			want := []string{"old"}
			if text == "new" {
				want = append(want, "new")
			}
			for _, cursor := range []*history.Store{main, child, child.NewCursor()} {
				require.Equal(t, want, cursor.Entries())
				require.Equal(t, len(want), cursor.Len())
				got, ok = cursor.Prev("fresh draft")
				require.True(t, ok)
				require.Equal(t, want[len(want)-1], got)
			}
			require.Equal(t, want, history.Open(path).Entries())
		})
	}
}

func TestCursorWalkSnapshotsSurviveSharedEviction(t *testing.T) {
	writer := history.Open(filepath.Join(t.TempDir(), "history.jsonl"))
	writer.Append("/older")
	writer.Append("/newer")
	writer.Append("plain")
	plain, slash := writer.NewCursor(), writer.NewCursor()
	got, ok := plain.Prev("plain draft")
	require.True(t, ok)
	require.Equal(t, "plain", got)
	got, ok = slash.Prev("/draft")
	require.True(t, ok)
	require.Equal(t, "/newer", got)

	for i := range history.MaxEntries + 5 {
		writer.Append(fmt.Sprintf("replacement %02d", i))
	}
	require.Equal(t, history.MaxEntries, plain.Len())
	require.Equal(t, "replacement 05", slash.Entries()[0])
	require.Empty(t, slash.Search("older"), "search sees the live corpus, not the retained walk")
	require.Equal(t, []string{"replacement 54"}, plain.Search("REPLACEMENT 54"))

	_, ok = slash.Prev("/edited recall")
	require.False(t, ok, "editing a recalled slash entry still refuses navigation")
	_, ok = slash.Next("/edited recall")
	require.False(t, ok)
	got, ok = slash.Prev("/newer")
	require.True(t, ok)
	require.Equal(t, "/older", got)
	got, ok = plain.Next("plain")
	require.True(t, ok)
	require.Equal(t, "plain draft", got)
	got, ok = slash.Next("/older")
	require.True(t, ok)
	require.Equal(t, "/newer", got)
	got, ok = slash.Next(got)
	require.True(t, ok)
	require.Equal(t, "/draft", got)
	_, ok = slash.Prev("/fresh")
	require.False(t, ok, "a fresh slash walk sees the current corpus without slash entries")
	got, ok = plain.Prev("fresh")
	require.True(t, ok)
	require.Equal(t, "replacement 54", got)
}

func TestCursorSharesZeroValueCorpusAndReturnsDetachedResults(t *testing.T) {
	var root history.Store
	child := root.NewCursor()
	child.Append("first")
	root.Append("second")
	child.Append("first") // only consecutive duplicates are suppressed
	require.Equal(t, []string{"first", "second", "first"}, root.Entries())

	entries := child.Entries()
	entries[0] = "changed"
	matches := child.Search("first")
	matches[0] = "changed"
	require.Equal(t, []string{"first", "second", "first"}, root.Entries())
	require.Equal(t, []string{"first", "first"}, root.Search("first"))

	var absent *history.Store
	require.Nil(t, absent.NewCursor())
}
