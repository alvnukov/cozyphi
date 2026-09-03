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

// symlinkTo makes dst a symlink to src and skips the test where the platform
// or filesystem refuses symlinks.
func symlinkTo(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.Symlink(src, dst); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

// The race the module exists to close: the leaf that was a regular file at
// permission-check time is a symlink by mutation time. The write must refuse
// it, and whatever the link points at must not gain a single byte.
func TestWriteRefusesLeafSymlinkWithoutTouchingTarget(t *testing.T) {
	outside := t.TempDir()
	target := filepath.Join(outside, "stolen.txt")
	require.NoError(t, os.WriteFile(target, []byte("secret"), 0o644))
	ws := t.TempDir()
	link := filepath.Join(ws, "note.md")
	symlinkTo(t, target, link)

	err := Write(link, 0o644, []byte("payload"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink", "the refusal must say what was refused")
	got, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "secret", string(got), "the link target must stay untouched")
	info, statErr := os.Lstat(link)
	require.NoError(t, statErr)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the symlink itself is left in place")
}

// A guard read must not follow a leaf symlink either: foreign bytes could
// flow into the caller's comparison and error messages.
func TestWriteCheckedRefusesToReadGuardThroughLeafSymlink(t *testing.T) {
	outside := t.TempDir()
	target := filepath.Join(outside, "stolen.txt")
	require.NoError(t, os.WriteFile(target, []byte("secret"), 0o644))
	ws := t.TempDir()
	link := filepath.Join(ws, "note.md")
	symlinkTo(t, target, link)

	verifyCalled := false
	err := WriteChecked(link, 0o644, []byte("payload"), func(_ []byte) error {
		verifyCalled = true
		return nil
	})

	require.Error(t, err)
	assert.False(t, verifyCalled, "the guard never sees foreign bytes")
	got, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "secret", string(got))
}

// The ancestor race: an ancestor directory is swapped for a symlink between
// the check and the rename, trying to steer the replacement outside. The
// re-verification must abort the write with the payload never landing there.
func TestWriteCheckedDetectsAncestorSwapDuringWrite(t *testing.T) {
	outside := t.TempDir()
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, "a", "b"), 0o755))
	path := filepath.Join(ws, "a", "b", "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("base"), 0o644))

	err := WriteChecked(path, 0o644, []byte("payload"), func(_ []byte) error {
		// The swap lands between staging and rename: the only window an
		// attacker controls.
		require.NoError(t, os.Rename(filepath.Join(ws, "a"), filepath.Join(ws, "a-old")))
		symlinkTo(t, outside, filepath.Join(ws, "a"))
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "changed during the write")
	entries, listErr := os.ReadDir(outside)
	require.NoError(t, listErr)
	assert.Empty(t, entries, "nothing may land in the swapped-to directory")
	got, readErr := os.ReadFile(filepath.Join(ws, "a-old", "b", "f.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "base", string(got), "the original file is untouched")
	// The staging dotfile may survive beside the original when the ancestor
	// was renamed mid-write: unlink is path-based and the staging path no
	// longer resolves. That is cosmetic; the security property is that
	// nothing landed outside the original directory.
}

func TestReadNoFollowRefusesLeafSymlink(t *testing.T) {
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(target, []byte("secret"), 0o644))
	ws := t.TempDir()
	link := filepath.Join(ws, "alias.txt")
	symlinkTo(t, target, link)

	_, err := ReadNoFollow(link)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")

	got, err := ReadNoFollow(target)
	require.NoError(t, err)
	assert.Equal(t, "secret", string(got))
}
