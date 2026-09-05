package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tools/writetool"
	"github.com/alvnukov/cozyphi/internal/util"
)

func TestDefaultToolsEditRequiresEditableReadAuthorization(t *testing.T) {
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
	require.ErrorContains(
		t,
		err,
		"[edit:anchor_not_observed]",
		"an anchor outside the read window refuses with the typed code",
	)

	_, err = registry["edit"].Run(t.Context(), editArgs)
	require.NoError(t, err, "a refused attempt leaves the file untouched, so its read still authorizes")

	_, err = registry["edit"].Run(t.Context(), editArgs)
	require.ErrorContains(t, err, "editable read", "successful edits must not be replayable")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "alpha\nBETA\ngamma", string(got))
}

func TestDefaultToolsFailedEditKeepsAuthorization(t *testing.T) {
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

	// The failed attempt changed nothing, so the retry against the snapshot it
	// was authorized for applies without another editable read.
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))
	_, err = registry["edit"].Run(t.Context(), editArgs)
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "alpha\nBETA\ngamma", string(got))

	_, err = registry["edit"].Run(t.Context(), editArgs)
	require.ErrorContains(t, err, "editable read", "the applied edit ends the authorization")
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
	if err != nil && strings.Contains(err.Error(), "ripgrep") {
		// The greptool suite skips the same way: no rg on the host (CI
		// runners included) is an environment gap, not a regression.
		t.Skip(err.Error())
	}
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

// The success path of the successor capability: one editable read authorizes
// two chained edits of the same region, the second against anchors the first
// printed — and nothing outside the granted window sneaks in.
func TestDefaultToolsEditChainsThroughSuccessorGrant(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, "sample.txt")
	original := "one\ntwo\nthree\nfour\nfive"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))
	registry := NewRegistry(DefaultTools())

	readArgs, err := json.Marshal(map[string]any{"path": "sample.txt", "mode": "edit"})
	require.NoError(t, err)
	_, err = registry["read"].Run(t.Context(), readArgs)
	require.NoError(t, err)

	first := "SECOND"
	firstEdit, err := json.Marshal(writetool.EditInput{
		Path: "sample.txt",
		Hash: util.ComputeFileHash(original),
		Edits: []writetool.FlatEdit{{
			From: testHashlineRef(2, "two"), To: testHashlineRef(2, "two"), Content: &first,
		}},
	})
	require.NoError(t, err)
	result, err := registry["edit"].Run(t.Context(), firstEdit)
	require.NoError(t, err)
	require.Contains(t, result.Content, "authorize the next edit")
	require.NotContains(t, result.Content, "Re-read this file")

	// The successor grant answered for the new revision's real hashes: the
	// second edit targets the shifted region without any read in between.
	afterFirst := "one\nSECOND\nthree\nfour\nfive"
	second := "IV"
	secondEdit, err := json.Marshal(writetool.EditInput{
		Path: "sample.txt",
		Hash: util.ComputeFileHash(afterFirst),
		Edits: []writetool.FlatEdit{{
			From: testHashlineRef(4, "four"), To: testHashlineRef(4, "four"), Content: &second,
		}},
	})
	require.NoError(t, err)
	_, err = registry["edit"].Run(t.Context(), secondEdit)
	require.NoError(t, err, "the successor grant must authorize the next edit without a re-read")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "one\nSECOND\nthree\nIV\nfive", string(got))

	// The grant is bounded: an anchor outside the changed region's window is
	// not observed, and the typed refusal says to read, not to retry blind.
	_, err = registry["edit"].Run(t.Context(), secondEdit)
	require.ErrorContains(t, err, "[edit:")
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "one\nSECOND\nthree\nIV\nfive", string(got), "a refused successor leaves the file as it was")
}

func TestDefaultToolsWriteChainsThroughPostWriteGrant(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, "created.txt")
	registry := NewRegistry(DefaultTools())

	// Create: the write itself is the trusted observation of the new
	// revision, so the follow-up edit needs no read in between.
	created := "alpha\nbeta\ngamma"
	writeArgs, err := json.Marshal(map[string]any{"path": "created.txt", "content": created})
	require.NoError(t, err)
	result, err := registry["write"].Run(t.Context(), writeArgs)
	require.NoError(t, err)
	require.Contains(t, result.Content, "authorize the next edit")
	require.Contains(t, result.Content, "created.txt#"+util.ComputeFileHash(created))
	// The result carries line hashes only; the written content is not echoed
	// back (the transcript diff card renders the diff from Output).
	require.NotContains(t, result.Content, "alpha")

	replacement := "BETA"
	editArgs, err := json.Marshal(writetool.EditInput{
		Path: "created.txt",
		Hash: util.ComputeFileHash(created),
		Edits: []writetool.FlatEdit{{
			From: testHashlineRef(2, "beta"), To: testHashlineRef(2, "beta"), Content: &replacement,
		}},
	})
	require.NoError(t, err)
	_, err = registry["edit"].Run(t.Context(), editArgs)
	require.NoError(t, err, "a fresh write must authorize a follow-up edit without a read")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "alpha\nBETA\ngamma", string(got))

	// Overwrite: the old revision's grant dies with the old TAG, and the new
	// write mints the capability for its exact revision the same way.
	overwritten := "one\ntwo\nthree"
	overwriteArgs, err := json.Marshal(map[string]any{"path": "created.txt", "content": overwritten})
	require.NoError(t, err)
	_, err = registry["write"].Run(t.Context(), overwriteArgs)
	require.NoError(t, err)
	fix := "II"
	secondEdit, err := json.Marshal(writetool.EditInput{
		Path: "created.txt",
		Hash: util.ComputeFileHash(overwritten),
		Edits: []writetool.FlatEdit{{
			From: testHashlineRef(2, "two"), To: testHashlineRef(2, "two"), Content: &fix,
		}},
	})
	require.NoError(t, err)
	_, err = registry["edit"].Run(t.Context(), secondEdit)
	require.NoError(t, err, "an overwrite re-mints the capability for its exact revision")
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "one\nII\nthree", string(got))
}

func TestDefaultToolsWriteGrantBoundsLargeFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	registry := NewRegistry(DefaultTools())

	lines := make([]string, 600)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %03d", i+1)
	}
	big := strings.Join(lines, "\n")
	writeArgs, err := json.Marshal(map[string]any{"path": "big.txt", "content": big})
	require.NoError(t, err)
	result, err := registry["write"].Run(t.Context(), writeArgs)
	require.NoError(t, err)
	// The grant is bounded: 40 anchors shown, the rest counted, the tail
	// beyond the cap requires a fresh editable read.
	require.Contains(t, result.Content, "+472 more anchors not shown")
	require.Contains(t, result.Content, "the grant covers the first 512 anchor lines")

	head := "top"
	headEdit, err := json.Marshal(writetool.EditInput{
		Path: "big.txt",
		Hash: util.ComputeFileHash(big),
		Edits: []writetool.FlatEdit{{
			From: testHashlineRef(1, "line 001"), To: testHashlineRef(1, "line 001"), Content: &head,
		}},
	})
	require.NoError(t, err)
	_, err = registry["edit"].Run(t.Context(), headEdit)
	require.NoError(t, err, "a line inside the capped grant stays editable")

	tail := "end"
	tailEdit, err := json.Marshal(writetool.EditInput{
		Path: "big.txt",
		Hash: util.ComputeFileHash(big),
		Edits: []writetool.FlatEdit{{
			From: testHashlineRef(600, "line 600"), To: testHashlineRef(600, "line 600"), Content: &tail,
		}},
	})
	require.NoError(t, err)
	_, err = registry["edit"].Run(t.Context(), tailEdit)
	require.ErrorContains(t, err, "[edit:", "a line past the grant cap requires a fresh read")
}

func testHashlineRef(line int, content string) string {
	return fmt.Sprintf("%d#%s", line, util.ComputeLineHash(content))
}
