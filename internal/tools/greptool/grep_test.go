package greptool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunGrep_CwdRelativeHeaders(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\nfunc Hello() {}\n"), 0o644))
	t.Chdir(root)

	raw, err := json.Marshal(grepInput{Pattern: "Hello", Path: "src"})
	require.NoError(t, err)
	out, err := runGrep(t.Context(), raw)
	if err != nil && strings.Contains(err.Error(), "ripgrep") {
		t.Skip(err.Error())
	}
	require.NoError(t, err)
	assert.Contains(t, out.Content, "@file src/main.go#")
	assert.Contains(t, out.Content, "src/main.go:>>")
	assert.NotContains(t, out.Content, "@file main.go#")
}

func TestRunGrep_DefaultPathUsesCwdRelative(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\nfunc Hello() {}\n"), 0o644))
	t.Chdir(root)

	raw, err := json.Marshal(grepInput{Pattern: "Hello"})
	require.NoError(t, err)
	out, err := runGrep(t.Context(), raw)
	if err != nil && strings.Contains(err.Error(), "ripgrep") {
		t.Skip(err.Error())
	}
	require.NoError(t, err)
	assert.Contains(t, out.Content, "@file src/main.go#")
}
