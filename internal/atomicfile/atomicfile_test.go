package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteReplacesContentCreatesDirectoryAndSetsMode(t *testing.T) {
	for _, mode := range []os.FileMode{0o600, 0o644} {
		dir := filepath.Join(t.TempDir(), "nested", "deeper")
		path := filepath.Join(dir, "state.json")

		require.NoError(t, Write(path, mode, []byte("first")))
		require.NoError(t, Write(path, mode, []byte("second")))

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "second", string(got), "mode %o: the second write must fully replace the first", mode)
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, mode, info.Mode().Perm(), "mode %o: permissions are applied explicitly", mode)
	}
}

func TestWriteLeavesNoStagingFileBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")

	require.NoError(t, Write(path, 0o600, []byte("first")))
	require.NoError(t, Write(path, 0o600, []byte("second")))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.Equal(t, []string{"MEMORY.md"}, names)
}

func TestWriteCheckedSeesCurrentBytesAndAbandonsSwapOnReject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.go")
	require.NoError(t, os.WriteFile(path, []byte("planned base"), 0o644))

	var seen []byte
	err := WriteChecked(path, 0o644, []byte("planned rewrite"), func(current []byte) error {
		// The concurrent writer lands inside the callback itself, before the
		// guard rejects: whatever the swap decides, its change must survive.
		seen = append([]byte(nil), current...)
		require.NoError(t, os.WriteFile(path, []byte("concurrent edit"), 0o644))
		return errors.New("file changed during edit")
	})
	require.Error(t, err)

	assert.Equal(t, "planned base", string(seen), "verify sees the target's current bytes")
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "concurrent edit", string(got), "a rejected guard leaves the target untouched")
	entries, listErr := os.ReadDir(dir)
	require.NoError(t, listErr)
	assert.Len(t, entries, 1, "the abandoned staging file is removed")
}

func TestWriteCheckedSwapsWhenVerifyAccepts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.go")
	require.NoError(t, os.WriteFile(path, []byte("planned base"), 0o644))

	err := WriteChecked(path, 0o644, []byte("planned rewrite"), func(current []byte) error {
		if string(current) != "planned base" {
			return errors.New("file changed during edit")
		}
		return nil
	})
	require.NoError(t, err)

	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "planned rewrite", string(got))
}
