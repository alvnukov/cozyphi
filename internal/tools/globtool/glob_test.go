package globtool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderGlobResult(t *testing.T) {
	assert.Equal(t, "No files found", renderGlobResult(nil))
	assert.Equal(
		t,
		"/a\n/b\n(Results are truncated. Consider using a more specific path or pattern.)",
		renderGlobResultTruncated([]string{"/a", "/b"}),
	)
}

func TestScanGlob_MatchSortLimitOffset(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	subDir := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	b := filepath.Join(subDir, "b.TXT")
	d := filepath.Join(subDir, "d.txt")
	c := filepath.Join(subDir, "c.go")

	require.NoError(t, os.WriteFile(a, []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(b, []byte("b"), 0o644))
	require.NoError(t, os.WriteFile(d, []byte("d"), 0o644))
	require.NoError(t, os.WriteFile(c, []byte("c"), 0o644))

	now := time.Now()
	require.NoError(t, os.Chtimes(a, now.Add(-3*time.Minute), now.Add(-3*time.Minute)))
	require.NoError(t, os.Chtimes(b, now.Add(-2*time.Minute), now.Add(-2*time.Minute)))
	require.NoError(t, os.Chtimes(d, now.Add(-1*time.Minute), now.Add(-1*time.Minute)))
	require.NoError(t, os.Chtimes(c, now.Add(-1*time.Minute), now.Add(-1*time.Minute)))

	files, truncated, err := scanGlob(t.Context(), "**/*.txt", root, 1, 1)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.True(t, truncated)
	assert.Equal(t, b, files[0])
}

func TestGlob_DefaultPathAndCaseInsensitive(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "ONE.TS"), []byte("x"), 0o644))

	t.Chdir(root)

	raw, err := json.Marshal(globInput{Pattern: "*.ts"})
	require.NoError(t, err)
	out, err := runGlob(t.Context(), raw)
	require.NoError(t, err)
	assert.Contains(t, out.Content, "ONE.TS")
}

func TestGlob_Errors(t *testing.T) {
	_, err := runGlob(t.Context(), []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pattern is required")

	_, err = runGlob(t.Context(), []byte(`{"pattern":"["}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid glob pattern")

	missing := filepath.Join(t.TempDir(), "missing")
	raw, mErr := json.Marshal(globInput{Pattern: "*.go", Path: missing})
	require.NoError(t, mErr)
	_, err = runGlob(t.Context(), raw)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "path not found")
}
