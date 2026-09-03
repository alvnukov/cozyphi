package writetool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

func TestAskPreviewRendersTheWriteDiff(t *testing.T) {
	dir := t.TempDir()
	ctx := tooldef.WithCwd(t.Context(), dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hello\n"), 0o644))

	got := AskPreview(ctx, "write", json.RawMessage(`{"path":"note.txt","content":"hello\nworld\n"}`))
	require.Contains(t, got, "+++ b/note.txt")
	require.Contains(t, got, "+world")
	require.NotContains(t, got, "+hello", "the unchanged line is context, not an addition")
}

func TestAskPreviewShowsANewFileAsAdditions(t *testing.T) {
	ctx := tooldef.WithCwd(t.Context(), t.TempDir())
	got := AskPreview(ctx, "write", json.RawMessage(`{"path":"fresh.txt","content":"alpha\nbeta"}`))
	require.Contains(t, got, "+alpha")
	require.Contains(t, got, "+beta")
}

func TestAskPreviewRendersTheEditDiff(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha\nbeta\ngamma"), 0o644))
	ctx := tooldef.WithCwd(t.Context(), dir)

	args, err := json.Marshal(EditInput{
		Path: "a.txt",
		Edits: []FlatEdit{{
			From:    hashlineRef(2, "beta"),
			To:      hashlineRef(2, "beta"),
			Content: new("BETA"),
		}},
	})
	require.NoError(t, err)
	got := AskPreview(ctx, "edit", args)
	require.Contains(t, got, "-beta")
	require.Contains(t, got, "+BETA")
}

// TestAskPreviewFailsSoft: the preview is evidence, not a gate — anything
// it cannot render falls back to "" and the ask shows the path list.
func TestAskPreviewFailsSoft(t *testing.T) {
	ctx := tooldef.WithCwd(t.Context(), t.TempDir())
	require.Empty(t, AskPreview(ctx, "edit", json.RawMessage(`{"path":"missing.txt","edits":[]}`)))
	require.Empty(t, AskPreview(ctx, "write", json.RawMessage(`not json`)))
	require.Empty(t, AskPreview(ctx, "write", json.RawMessage(`{"path":"","content":"x"}`)))
	require.Empty(t, AskPreview(ctx, "bash", json.RawMessage(`{"command":"ls"}`)))
}

func TestAskPreviewRefusesAStaleEdit(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha\nbeta"), 0o644))
	ctx := tooldef.WithCwd(t.Context(), dir)

	args, err := json.Marshal(EditInput{
		Path: "a.txt",
		Edits: []FlatEdit{{
			From:    hashlineRef(2, "not what is there"),
			To:      hashlineRef(2, "not what is there"),
			Content: new("X"),
		}},
	})
	require.NoError(t, err)
	require.Empty(t, AskPreview(ctx, "edit", args), "a diff the real run would refuse must not be shown")
}

func TestAskPreviewClipsAMonsterDiff(t *testing.T) {
	ctx := tooldef.WithCwd(t.Context(), t.TempDir())
	var b strings.Builder
	for i := range 600 {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	args, err := json.Marshal(map[string]string{"path": "big.txt", "content": b.String()})
	require.NoError(t, err)

	got := AskPreview(ctx, "write", args)
	require.Contains(t, got, "more diff lines")
	require.Equal(t, askPreviewLines+1, strings.Count(got, "\n")+1)
}
