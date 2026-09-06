package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDependencySyncPrecedesTarget(t *testing.T) {
	dir, main, dependency := setupWorkspace(t)
	params := paramsPath(t)
	mgr := openNav(t, dir, "LSP_TEST_PARAMS="+params, "LSP_TEST_DEF_RESULT=null")
	query := func(file string) {
		t.Helper()
		_, err := mgr.Query(t.Context(), Query{Op: OpDefinition, File: file, Line: 1, Character: 1})
		require.NoError(t, err)
	}
	query(dependency)
	query(main)
	require.NoError(t, os.WriteFile(dependency, []byte("package main\nfunc newDependency() {}\n"), 0o600))
	require.NoError(t, os.WriteFile(main, []byte("package main\nfunc main() { newDependency() }\n"), 0o600))
	query(main)
	require.Equal(t, 2, countParams(t, params, "textDocument/didChange"))
	require.Contains(t, wireParams(t, params, "textDocument/didChange"), uriFromPath(main), "target sync is last")
	require.Equal(t, 1, countParams(t, params, "workspace/didChangeWatchedFiles"))
	query(main)
	require.Equal(t, 2, countParams(t, params, "textDocument/didChange"), "unchanged queries send no new changes")
}

func TestSourceScanIgnoresHiddenTreesAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"main.go", "nested/go.mod", "nested/source.go", ".worktrees/other/main.go", "node_modules/main.go", "notes.txt"} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("test"), 0o600))
	}
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(dir, "outside")))
	files, err := scanSources(t.Context(), dir)
	require.NoError(t, err)
	require.Len(t, files, 3)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = scanSources(ctx, dir)
	require.Error(t, err)
}

func TestQueryWaitHonorsCancellation(t *testing.T) {
	c := &client{queryGate: make(chan struct{}, 1), done: make(chan struct{})}
	c.queryGate <- struct{}{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	release, err := c.beginQuery(ctx, "")
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, release)
}
