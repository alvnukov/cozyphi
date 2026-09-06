//go:build integration

package lsp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealGoplsDependencyEdit(t *testing.T) {
	t.Run("same module", func(t *testing.T) { testRealDependencyEdit(t, false) })
	t.Run("nested replacement", func(t *testing.T) { testRealDependencyEdit(t, true) })
}

func testRealDependencyEdit(t *testing.T, nested bool) {
	t.Helper()
	dir := t.TempDir()
	put := func(name, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		return path
	}
	put("go.mod", "module example.com/repro\n\ngo 1.24\n")
	if nested {
		put("dep/go.mod", "module example.com/repro/dep\n\ngo 1.24\n")
		put(
			"go.mod",
			"module example.com/repro\n\ngo 1.24\nrequire example.com/repro/dep v0.0.0\nreplace example.com/repro/dep => ./dep\n",
		)
	}
	dep := put("dep/dep.go", "package dep\nfunc Old() int { return 1 }\n")
	main := put("main.go", "package main\nimport \"example.com/repro/dep\"\nfunc main() { _ = dep.Old() }\n")
	mgr, err := Open(t.Context(), dir, DefaultConfig())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, mgr.Close(t.Context())) })
	query := func(path string) Result {
		t.Helper()
		res, err := mgr.Query(t.Context(), Query{Op: OpDiagnostics, File: path})
		require.NoError(t, err)
		return res
	}
	require.Empty(t, query(dep).Diagnostics)
	require.Empty(t, query(main).Diagnostics)
	put("dep/dep.go", "package dep\nfunc New() int { return 1 }\n")
	put("main.go", "package main\nimport \"example.com/repro/dep\"\nfunc main() { _ = dep.New() }\n")
	res := query(main)
	require.Empty(t, res.Diagnostics, "dependency edit must reach gopls; status=%s", res.Status)
	require.Contains(t, []string{StatusFresh, StatusCached}, res.Status)

	put("main.go", "package main\nimport \"example.com/repro/dep\"\nfunc main() { _ = dep.Added() }\n")
	require.NotEmpty(t, query(main).Diagnostics)
	added := put("dep/added.go", "package dep\nfunc Added() int { return 1 }\n")
	require.Empty(t, query(main).Diagnostics, "new unopened source must invalidate unchanged caller")
	require.Empty(t, query(added).Diagnostics)
	require.NoError(t, os.Remove(added))
	require.NotEmpty(t, query(main).Diagnostics, "deleted source must invalidate unchanged caller")
}
