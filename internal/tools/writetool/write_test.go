package writetool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// symlinkOrSkip links newname to oldname, skipping the test where the platform
// or filesystem refuses symlinks. Several tests here turn on symlink behavior
// and every one of them needs the same escape hatch.
func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func TestRunWriteCreatesAndOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "out.txt")
	tool := WriteTool()

	created, err := tool.Run(t.Context(), mustWriteArgs(t, path, "first\n"))
	require.NoError(t, err)
	require.Contains(t, created.Content, "wrote")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "first\n", string(got))

	overwritten, err := tool.Run(t.Context(), mustWriteArgs(t, path, "second\n"))
	require.NoError(t, err)
	require.Contains(t, overwritten.Content, "wrote")
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "second\n", string(got))
}

func TestRunWriteRelativePathInResult(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	tool := WriteTool()

	created, err := tool.Run(t.Context(), mustWriteArgs(t, "nested/out.txt", "first\n"))
	require.NoError(t, err)
	require.Equal(t, "wrote 6 bytes to nested/out.txt", created.Content)
	require.Equal(t, "nested/out.txt", created.Detail)
}

func TestRunWriteOutputIsTheDiff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	tool := WriteTool()

	created, err := tool.Run(t.Context(), mustWriteArgs(t, path, "first\n"))
	require.NoError(t, err)
	require.Contains(t, created.Output, "+first", "a new file diffs against nothing")
	require.NotContains(t, created.Output, "wrote", "the byte count stays model-facing")

	overwritten, err := tool.Run(t.Context(), mustWriteArgs(t, path, "second\n"))
	require.NoError(t, err)
	require.Contains(t, overwritten.Output, "-first")
	require.Contains(t, overwritten.Output, "+second")
}

func TestRunWriteRequiresPath(t *testing.T) {
	tool := WriteTool()

	_, err := tool.Run(t.Context(), mustWriteArgs(t, "", "content"))
	require.ErrorContains(t, err, "path is required",
		"the refusal must name what is missing, not fail anonymously")
}

// The write tool must not write through a leaf symlink swapped in after the
// permission check: the external target stays byte-identical.
func TestRunWriteRefusesSymlinkedTargetAndKeepsExternalIntact(t *testing.T) {
	outside := t.TempDir()
	target := filepath.Join(outside, "stolen.txt")
	require.NoError(t, os.WriteFile(target, []byte("secret"), 0o644))
	ws := t.TempDir()
	link := filepath.Join(ws, "note.md")
	symlinkOrSkip(t, target, link)
	tool := WriteTool()

	_, err := tool.Run(t.Context(), mustWriteArgs(t, link, "payload"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "symlink")
	got, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, "secret", string(got), "the external target must stay untouched")
}

// The edit tool refuses before any content flows: not even a foreign file's
// bytes may reach the TAG comparison or the mismatch report.
func TestRunEditRefusesLeafSymlink(t *testing.T) {
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(target, []byte("secret"), 0o644))
	ws := t.TempDir()
	link := filepath.Join(ws, "note.md")
	symlinkOrSkip(t, target, link)
	args, err := json.Marshal(EditInput{Path: link, Hash: "ABCD", Edits: []FlatEdit{{From: "1#aaaa", To: "1#aaaa"}}})
	require.NoError(t, err)

	_, runErr := runEdit(t.Context(), args)
	require.Error(t, runErr)
	require.Contains(t, runErr.Error(), "symlink")
	got, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, "secret", string(got))
}

func mustWriteArgs(t *testing.T, path, content string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]string{"path": path, "content": content})
	require.NoError(t, err)
	return b
}
