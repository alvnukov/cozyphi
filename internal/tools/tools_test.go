package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tools/writetool"
	"github.com/alvnukov/cozyphi/internal/util"
)

func TestDefaultToolsEditRequiresAndConsumesEditableReadAuthorization(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, "sample.txt")
	original := "alpha\nbeta\ngamma"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))
	registry := NewRegistry(DefaultTools())

	viewArgs, err := json.Marshal(map[string]any{"path": "sample.txt"})
	require.NoError(t, err)
	_, err = registry["read"].Run(t.Context(), viewArgs)
	require.NoError(t, err)

	replacement := "BETA"
	editArgs, err := json.Marshal(writetool.EditInput{
		Path: "sample.txt",
		Hash: util.ComputeFileHash(original),
		Edits: []writetool.FlatEdit{{
			From:    testHashlineRef(2, "beta"),
			To:      testHashlineRef(2, "beta"),
			Content: &replacement,
		}},
	})
	require.NoError(t, err)
	_, err = registry["edit"].Run(t.Context(), editArgs)
	require.ErrorContains(t, err, "editable read")

	editableArgs, err := json.Marshal(map[string]any{"path": "sample.txt", "mode": "edit", "offset": 2, "limit": 1})
	require.NoError(t, err)
	_, err = registry["read"].Run(t.Context(), editableArgs)
	require.NoError(t, err)

	unauthorizedArgs, err := json.Marshal(writetool.EditInput{
		Path: "sample.txt",
		Hash: util.ComputeFileHash(original),
		Edits: []writetool.FlatEdit{
			{From: testHashlineRef(1, "alpha"), To: testHashlineRef(1, "alpha"), Content: &replacement},
		},
	})
	require.NoError(t, err)
	_, err = registry["edit"].Run(t.Context(), unauthorizedArgs)
	require.ErrorContains(t, err, "not authorized")

	_, err = registry["edit"].Run(t.Context(), editArgs)
	require.ErrorContains(t, err, "editable read", "the failed attempt must consume the snapshot authorization")

	_, err = registry["read"].Run(t.Context(), editableArgs)
	require.NoError(t, err)
	_, err = registry["edit"].Run(t.Context(), editArgs)
	require.NoError(t, err)

	_, err = registry["edit"].Run(t.Context(), editArgs)
	require.ErrorContains(t, err, "editable read", "successful edits must not be replayable")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "alpha\nBETA\ngamma", string(got))
}

func TestDefaultToolsStaleEditConsumesAuthorization(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, "sample.txt")
	original := "alpha\nbeta\ngamma"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))
	registry := NewRegistry(DefaultTools())

	editableArgs, err := json.Marshal(map[string]any{"path": "sample.txt", "mode": "edit"})
	require.NoError(t, err)
	_, err = registry["read"].Run(t.Context(), editableArgs)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("changed\nbeta\ngamma"), 0o644))

	replacement := "BETA"
	editArgs, err := json.Marshal(writetool.EditInput{
		Path: "sample.txt",
		Hash: util.ComputeFileHash(original),
		Edits: []writetool.FlatEdit{
			{From: testHashlineRef(2, "beta"), To: testHashlineRef(2, "beta"), Content: &replacement},
		},
	})
	require.NoError(t, err)
	_, err = registry["edit"].Run(t.Context(), editArgs)
	require.ErrorContains(t, err, "file TAG mismatch")

	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))
	_, err = registry["edit"].Run(t.Context(), editArgs)
	require.ErrorContains(t, err, "editable read")
}

func TestDefaultToolsLedgerIsOwnedByOneRegistrySession(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	original := "alpha\nbeta\ngamma"
	require.NoError(t, os.WriteFile("sample.txt", []byte(original), 0o644))
	first := NewRegistry(DefaultTools())
	second := NewRegistry(DefaultTools())

	readArgs, err := json.Marshal(map[string]any{"path": "sample.txt", "mode": "edit"})
	require.NoError(t, err)
	_, err = first["read"].Run(t.Context(), readArgs)
	require.NoError(t, err)

	replacement := "BETA"
	editArgs, err := json.Marshal(writetool.EditInput{
		Path: "sample.txt",
		Hash: util.ComputeFileHash(original),
		Edits: []writetool.FlatEdit{
			{From: testHashlineRef(2, "beta"), To: testHashlineRef(2, "beta"), Content: &replacement},
		},
	})
	require.NoError(t, err)
	_, err = second["edit"].Run(t.Context(), editArgs)
	require.ErrorContains(t, err, "current-session")
	_, err = first["edit"].Run(t.Context(), editArgs)
	require.NoError(t, err)
}

func TestDefaultToolsGrepOutputAuthorizesReturnedAnchors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	original := "alpha\nbeta\ngamma"
	require.NoError(t, os.WriteFile("sample.txt", []byte(original), 0o644))
	registry := NewRegistry(DefaultTools())

	grepArgs, err := json.Marshal(map[string]any{"pattern": "beta", "path": "sample.txt", "literal": true})
	require.NoError(t, err)
	result, err := registry["grep"].Run(t.Context(), grepArgs)
	require.NoError(t, err)
	require.Contains(t, result.Content, testHashlineRef(2, "beta"))

	replacement := "BETA"
	editArgs, err := json.Marshal(writetool.EditInput{
		Path: "sample.txt",
		Hash: util.ComputeFileHash(original),
		Edits: []writetool.FlatEdit{
			{From: testHashlineRef(2, "beta"), To: testHashlineRef(2, "beta"), Content: &replacement},
		},
	})
	require.NoError(t, err)
	_, err = registry["edit"].Run(t.Context(), editArgs)
	require.NoError(t, err)
}

func testHashlineRef(line int, content string) string {
	return fmt.Sprintf("%d#%s", line, util.ComputeLineHash(content))
}
