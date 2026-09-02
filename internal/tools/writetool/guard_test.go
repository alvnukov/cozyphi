package writetool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
	"github.com/alvnukov/cozyphi/internal/util"
)

// workspaceGuard is the shape of the re-check the executor installs: a
// destination that resolves outside the workspace is refused.
func workspaceGuard(ws string) tooldef.MutationGuard {
	return func(_ context.Context, path string) error {
		dir := filepath.Dir(path)
		for {
			resolved, err := filepath.EvalSymlinks(dir)
			if err == nil {
				if resolved != ws && !strings.HasPrefix(resolved, ws+string(os.PathSeparator)) {
					return errors.New("write outside workspace denied: " + resolved)
				}
				return nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return err
			}
			dir = parent
		}
	}
}

// The gate approved a path inside the workspace; before the tool ran, an
// ancestor became a symlink out of it. The write must fail and the directory
// the link points at must gain nothing.
func TestRunWriteRefusesAncestorSwappedAfterApproval(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	inner := filepath.Join(ws, "src")
	require.NoError(t, os.MkdirAll(inner, 0o755))
	path := filepath.Join(inner, "pkg", "note.txt")
	require.NoError(t, os.Remove(inner))
	symlinkOrSkip(t, outside, inner)
	ctx := tooldef.WithMutationGuard(t.Context(), workspaceGuard(ws))

	_, err := WriteTool().Run(ctx, mustWriteArgs(t, path, "payload"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "outside workspace denied")
	_, statErr := os.Stat(filepath.Join(outside, "pkg"))
	require.True(t, os.IsNotExist(statErr), "the write escaped through the swapped ancestor")
}

// A destination that is still where the gate left it writes normally: the
// guard re-applies the verdict, it does not add a new restriction.
func TestRunWriteUnderGuardStillWritesInsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	resolvedWS, err := filepath.EvalSymlinks(ws)
	require.NoError(t, err)
	path := filepath.Join(ws, "pkg", "note.txt")
	ctx := tooldef.WithMutationGuard(t.Context(), workspaceGuard(resolvedWS))

	_, runErr := WriteTool().Run(ctx, mustWriteArgs(t, path, "payload"))

	require.NoError(t, runErr)
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "payload", string(got))
}

// The edit path carries the same guard: a swapped ancestor stops the rename
// even though the file was read successfully through the link.
func TestRunEditRefusesAncestorSwappedAfterApproval(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	inner := filepath.Join(ws, "src")
	require.NoError(t, os.MkdirAll(inner, 0o755))
	target := filepath.Join(outside, "note.txt")
	require.NoError(t, os.WriteFile(target, []byte("first\n"), 0o644))
	require.NoError(t, os.Remove(inner))
	symlinkOrSkip(t, outside, inner)
	path := filepath.Join(inner, "note.txt")
	tag := util.ComputeFileHash("first\n")
	lineTag := util.ComputeLineHash("first")
	args, err := json.Marshal(EditInput{
		Path:  path,
		Hash:  tag,
		Edits: []FlatEdit{{From: "1#" + lineTag, To: "1#" + lineTag, Content: new("second")}},
	})
	require.NoError(t, err)
	ctx := tooldef.WithMutationGuard(t.Context(), workspaceGuard(ws))

	_, runErr := runEdit(ctx, args)

	require.Error(t, runErr)
	require.Contains(t, runErr.Error(), "outside workspace denied")
	got, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, "first\n", string(got), "the external file must stay untouched")
}
