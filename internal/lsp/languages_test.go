package lsp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLanguagesNeverStartsProcess pins the frozen contract: languages needs
// no target fields and never launches a server, even when the configured
// executable is missing.
func TestLanguagesNeverStartsProcess(t *testing.T) {
	dir, _, _ := setupWorkspace(t)
	mgr, err := Open(t.Context(), dir, Config{
		Enabled: true,
		Gopls:   GoplsConfig{Command: []string{filepath.Join(t.TempDir(), "missing-gopls")}},
	})
	require.NoError(t, err)
	defer mgr.Close(t.Context())

	for range 2 {
		res, err := mgr.Query(t.Context(), Query{Op: OpLanguages})
		require.NoError(t, err)
		require.Len(t, res.Languages, 1)
		rec := res.Languages[0]
		assert.Equal(t, "go", rec.Language)
		assert.Equal(t, "gopls", rec.Server)
		assert.True(t, rec.Configured)
		assert.False(t, rec.Installed)
		assert.False(t, rec.Running)
		assert.Equal(t, 0, rec.ActiveRoots)
		assert.Empty(t, rec.Error)
		assert.Equal(t, "go install golang.org/x/tools/gopls@latest", rec.InstallHint)
		assert.Equal(t,
			[]string{
				"definition", "references", "implementations", "type_definition",
				"hover", "symbols", "calls", "diagnostics",
			},
			rec.Operations,
		)
	}
}

// TestLanguagesReportsRunningClient drives one real client generation through
// the fake server and then reports it as running with one active root.
func TestLanguagesReportsRunningClient(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	mgr, err := Open(t.Context(), dir, fakeConfig(
		"LSP_TEST_DEF_RESULT="+defFixture(uriFromPath(otherFile)),
	))
	require.NoError(t, err)
	defer mgr.Close(t.Context())

	_, err = mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
	require.NoError(t, err)

	res, err := mgr.Query(t.Context(), Query{Op: OpLanguages})
	require.NoError(t, err)
	require.Len(t, res.Languages, 1)
	rec := res.Languages[0]
	assert.True(t, rec.Installed)
	assert.True(t, rec.Running)
	assert.Equal(t, 1, rec.ActiveRoots)
	assert.Empty(t, rec.Error)
	assert.Empty(t, rec.InstallHint)
	assert.Contains(t, rec.Operations, "definition")
}

// TestLanguagesReportsStartError: after a failed start attempt the bounded
// sanitized reason surfaces in the record and the executable still counts as
// not installed.
func TestLanguagesReportsStartError(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	notExecutable := filepath.Join(t.TempDir(), "gopls-text")
	require.NoError(t, os.WriteFile(notExecutable, []byte("not a binary"), 0o644))
	mgr, err := Open(t.Context(), dir, Config{
		Enabled: true,
		Gopls:   GoplsConfig{Command: []string{notExecutable}},
	})
	require.NoError(t, err)
	defer mgr.Close(t.Context())

	_, qerr := mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 1, Character: 1})
	require.Error(t, qerr)
	assert.Equal(t, ErrUnavailable, errKind(qerr))

	res, err := mgr.Query(t.Context(), Query{Op: OpLanguages})
	require.NoError(t, err)
	require.Len(t, res.Languages, 1)
	rec := res.Languages[0]
	assert.False(t, rec.Installed)
	assert.False(t, rec.Running)
	assert.NotEmpty(t, rec.Error)
	assert.LessOrEqual(t, len(rec.Error), MaxTextFieldBytes)
}
