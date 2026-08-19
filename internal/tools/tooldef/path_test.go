package tooldef

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelTo_InsideCwd(t *testing.T) {
	cwd := filepath.Join(string(filepath.Separator), "repo")
	abs := filepath.Join(cwd, "src", "main.go")
	assert.Equal(t, "src/main.go", RelTo(cwd, abs))
	assert.Equal(t, ".", RelTo(cwd, cwd))
}

func TestRelTo_OutsideCwdStaysAbsolute(t *testing.T) {
	cwd := filepath.Join(string(filepath.Separator), "repo")
	other := filepath.Join(string(filepath.Separator), "tmp", "x.go")
	assert.Equal(t, filepath.ToSlash(filepath.Clean(other)), RelTo(cwd, other))
}

func TestResolveToCwdAndRelToCwdRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "pkg"), 0o755))

	abs, err := ResolveToCwd(t.Context(), "pkg/x.go")
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(abs))
	assert.Equal(t, "pkg/x.go", RelToCwd(t.Context(), abs))
}

func TestResolveToCwd_UsesContextCwd(t *testing.T) {
	root := t.TempDir()
	ctx := WithCwd(t.Context(), root)

	abs, err := ResolveToCwd(ctx, "pkg/x.go")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "pkg", "x.go"), abs)
	assert.Equal(t, "pkg/x.go", RelToCwd(ctx, abs))
}
