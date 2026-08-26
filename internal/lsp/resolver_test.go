package lsp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkexe writes an executable gopls stand-in so resolver precedence can be
// tested hermetically, without the real HOME or PATH.
func mkexe(t *testing.T, dir string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, "gopls")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755))
	return path
}

func TestResolveBinaryPrecedence(t *testing.T) {
	base := t.TempDir()
	ownerBin := filepath.Join(base, "owner")
	pathDir1 := filepath.Join(base, "p1")
	pathDir2 := filepath.Join(base, "p2")
	inOwner := mkexe(t, ownerBin)
	inPath1 := mkexe(t, pathDir1)
	inPath2 := mkexe(t, pathDir2)

	got, ok := resolveBinary("gopls", ownerBin, []string{pathDir1, pathDir2})
	require.True(t, ok)
	assert.Equal(t, inOwner, got)

	got, ok = resolveBinary("gopls", filepath.Join(base, "missing"), []string{pathDir1, pathDir2})
	require.True(t, ok)
	assert.Equal(t, inPath1, got)

	got, ok = resolveBinary("gopls", filepath.Join(base, "missing"), []string{"", ".", base, pathDir2})
	require.True(t, ok)
	// Empty, dot, and non-absolute entries are skipped, so p2 wins over the
	// dot entry that would resolve against the working directory.
	assert.Equal(t, inPath2, got)
}

func TestResolveBinaryAbsoluteAndMisses(t *testing.T) {
	base := t.TempDir()
	exe := mkexe(t, base)

	got, ok := resolveBinary(exe, base, nil)
	require.True(t, ok)
	assert.Equal(t, exe, got)

	_, ok = resolveBinary(filepath.Join(base, "no-such-binary"), base, nil)
	assert.False(t, ok)

	_, ok = resolveBinary("definitely-not-anywhere-xyz", base, []string{base})
	assert.False(t, ok)

	// Separator-bearing names fail closed here even if the loader missed them.
	_, ok = resolveBinary("./gopls", base, []string{base})
	assert.False(t, ok)
}

func TestResolveBinaryRequiresExecBit(t *testing.T) {
	base := t.TempDir()
	plain := filepath.Join(base, "gopls")
	require.NoError(t, os.WriteFile(plain, []byte("data"), 0o644))
	_, ok := resolveBinary("gopls", base, nil)
	assert.False(t, ok)
}
