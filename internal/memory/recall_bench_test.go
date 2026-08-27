package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// benchDir builds a memory directory of n facts that read like real ones:
// every description shares the common words, one body carries a rare term.
func benchDir(tb testing.TB, n int) string {
	tb.Helper()
	dir := tb.TempDir()
	body := strings.Repeat("The transcript pane renders the row lazily and the footer follows it. ", 12)
	for i := range n {
		name := fmt.Sprintf("note-%05d", i)
		file := fmt.Sprintf("---\nname: %s\ndescription: Note %d about the transcript pane and how it "+
			"renders.\nmetadata:\n  type: project\n---\n%s\n", name, i, body)
		if i == n/2 {
			file += "\nThe kerberos ticket expires nightly.\n"
		}
		require.NoError(tb, os.WriteFile(filepath.Join(dir, name+fileExt), []byte(file), 0o600))
	}
	return dir
}

// benchSizes are a plausible directory and an implausible one.
var benchSizes = []int{100, 10_000}

// BenchmarkReminder is the per-turn cost: what every user prompt pays.
func BenchmarkReminder(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			store, err := Open(benchDir(b, size), nil)
			require.NoError(b, err)
			query := Query{Prompt: "when does the kerberos ticket expire"}
			require.NotEmpty(b, store.Turn().Reminder(query), "the benchmark must measure a hit")

			b.ReportAllocs()
			for b.Loop() {
				if store.Turn().Reminder(query) == "" {
					b.Fatal("no match")
				}
			}
		})
	}
}

// BenchmarkIndexBuild is the cold cost: paid once per change to the directory,
// not per turn.
func BenchmarkIndexBuild(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			store, err := Open(benchDir(b, size), nil)
			require.NoError(b, err)

			b.ReportAllocs()
			for b.Loop() {
				store.mu.Lock()
				store.cached = nil
				store.mu.Unlock()
				store.index()
			}
		})
	}
}

// BenchmarkIndexRebuildAfterOneWrite is what a turn that saved a memory pays:
// one file changed out of ten thousand, and only that one is read again.
func BenchmarkIndexRebuildAfterOneWrite(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			dir := benchDir(b, size)
			store, err := Open(dir, nil)
			require.NoError(b, err)
			path := filepath.Join(dir, "note-00042"+fileExt)

			b.ReportAllocs()
			for revision := 0; b.Loop(); revision++ {
				file := fmt.Sprintf("---\nname: note-00042\ndescription: Rewritten %d times.\n"+
					"metadata:\n  type: project\n---\nThe kerberos ticket expires nightly.\n", revision)
				require.NoError(b, os.WriteFile(path, []byte(file), 0o600))
				store.Invalidate()
				store.index()
			}
		})
	}
}
