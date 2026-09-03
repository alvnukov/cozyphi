package writetool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Overwriting a script must not cost it its exec bit: the tool rewrites
// content, and the rename would otherwise land the staging file's mode.
func TestRunWriteKeepsExistingFileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.sh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho old\n"), 0o755))
	tool := WriteTool()

	_, err := tool.Run(t.Context(), mustWriteArgs(t, path, "#!/bin/sh\necho new\n"))
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), info.Mode().Perm(), "a write rewrites content, not permissions")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "a successful write leaves no staging file behind")
}

func TestRunWriteCreatesNewFileWithDefaultMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "fresh.txt")
	tool := WriteTool()

	_, err := tool.Run(t.Context(), mustWriteArgs(t, path, "hello\n"))
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, defaultFileMode, info.Mode().Perm(), "a new file gets the default mode, umask notwithstanding")
}

// A destination that is not a regular file donates nothing: a symlink's own
// 0777 must not become the replacement's mode. The write is refused anyway,
// so this pins the mode decision itself.
func TestDestinationModeIgnoresNonRegularEntries(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("x\n"), 0o600))
	link := filepath.Join(dir, "link.txt")
	symlinkOrSkip(t, target, link)

	require.Equal(t, defaultFileMode, destinationMode(link), "a leaf symlink is not a mode to preserve")
	require.Equal(t, defaultFileMode, destinationMode(filepath.Join(dir, "missing.txt")))
	require.Equal(t, os.FileMode(0o600), destinationMode(target))
}
