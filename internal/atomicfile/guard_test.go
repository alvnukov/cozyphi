package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A guard that refuses the destination must stop the write before it creates
// anything: the parents of a redirected path are as unwanted as the file.
func TestGuardRefusalCreatesNoDirectories(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a", "b", "note.txt")

	err := WriteWith(path, 0o644, []byte("payload"), Options{
		Guard: func(string) error { return errors.New("outside workspace denied") },
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside workspace denied")
	assert.Contains(t, err.Error(), path)
	_, statErr := os.Stat(filepath.Join(root, "a"))
	assert.True(t, os.IsNotExist(statErr), "the refused write created a parent directory")
}

// The guard is asked twice: once before the directories exist and once with
// the staged bytes on disk. A refusal on the second call leaves the previous
// contents in place and strands no staging file.
func TestGuardRefusalBeforeRenameKeepsTargetAndCleansStaging(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	calls := 0
	err := WriteWith(path, 0o644, []byte("payload"), Options{
		Guard: func(string) error {
			calls++
			if calls == 1 {
				return nil
			}
			return errors.New("directory now resolves outside the workspace")
		},
	})

	require.Error(t, err)
	assert.Equal(t, 2, calls, "the guard runs before the directories and again before the rename")
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(got))
	entries, readDirErr := os.ReadDir(dir)
	require.NoError(t, readDirErr)
	assert.Len(t, entries, 1, "the staging file outlived the refused write")
}

// The window the guard exists for: an ancestor is swapped for a symlink out of
// the workspace between the permission check and the call. Resolution alone
// cannot see it — the module resolves what the path points at now — so the
// guard is what keeps the bytes from landing outside.
func TestGuardStopsAncestorSwappedBeforeTheWrite(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	inner := filepath.Join(ws, "a")
	require.NoError(t, os.MkdirAll(inner, 0o755))

	// The approved path, as the gate resolved it while a/ was a real
	// directory inside the workspace.
	path := filepath.Join(inner, "b", "note.txt")
	require.NoError(t, os.Remove(inner))
	symlinkTo(t, outside, inner)

	err := WriteWith(path, 0o644, []byte("payload"), Options{
		Guard: func(p string) error {
			resolved, resolveErr := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(p)))
			if resolveErr != nil {
				return resolveErr
			}
			if resolved != ws {
				return errors.New("write outside workspace denied: " + resolved)
			}
			return nil
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside workspace denied")
	_, statErr := os.Stat(filepath.Join(outside, "b"))
	assert.True(t, os.IsNotExist(statErr), "the write escaped through the swapped ancestor")
}
