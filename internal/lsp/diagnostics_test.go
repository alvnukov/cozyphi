package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countParams counts recorded wire calls of method in the params log.
func countParams(t *testing.T, path, method string) int {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	n := 0
	for line := range strings.SplitSeq(string(raw), "\n") {
		name, _, ok := strings.Cut(line, "\t")
		if ok && name == method {
			n++
		}
	}
	return n
}

// wireRangeJSON renders a compact range fixture.
func wireRangeJSON(l1, c1, l2, c2 int) string {
	return fmt.Sprintf(`{"start":{"line":%d,"character":%d},"end":{"line":%d,"character":%d}}`, l1, c1, l2, c2)
}

func TestSyncDidOpenOnceAndNoChangeWhenUnchanged(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	params := paramsPath(t)
	mgr := openNav(t, dir, "LSP_TEST_PARAMS="+params, "LSP_TEST_DEF_RESULT=null")
	q := Query{Op: OpDefinition, File: mainFile, Line: 1, Character: 1}
	for range 2 {
		_, err := mgr.Query(t.Context(), q)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, countParams(t, params, "textDocument/didOpen"))
	assert.Equal(t, 0, countParams(t, params, "textDocument/didChange"))
}

func TestSyncDidChangeFullOnDiskEdit(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	params := paramsPath(t)
	mgr := openNav(t, dir, "LSP_TEST_PARAMS="+params, "LSP_TEST_DEF_RESULT=null")
	q := Query{Op: OpDefinition, File: mainFile, Line: 1, Character: 1}
	_, err := mgr.Query(t.Context(), q)
	require.NoError(t, err)

	newText := "package main\n\nfunc main() { g() }\n"
	require.NoError(t, os.WriteFile(mainFile, []byte(newText), 0o600))
	_, err = mgr.Query(t.Context(), q)
	require.NoError(t, err)

	dc := wireParams(t, params, "textDocument/didChange")
	assert.Contains(t, dc, `"version":2`)
	want, errJSON := json.Marshal(newText)
	require.NoError(t, errJSON)
	assert.Contains(t, dc, `"contentChanges":[{"text":`+string(want)+`}]`)
	assert.NotContains(t, dc, `"range"`, "full sync sends no replacement range")
}

func TestSyncDidChangeIncrementalUTF16(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	v1 := "package main\n\n// \U0001F600x" // no trailing newline: emoji lands in the end column
	require.NoError(t, os.WriteFile(mainFile, []byte(v1), 0o600))
	params := paramsPath(t)
	mgr := openNav(t, dir,
		"LSP_TEST_SYNC_KIND=2",
		"LSP_TEST_PARAMS="+params,
		"LSP_TEST_DEF_RESULT=null",
	)
	q := Query{Op: OpDefinition, File: mainFile, Line: 1, Character: 1}
	_, err := mgr.Query(t.Context(), q)
	require.NoError(t, err)

	v2 := v1 + "y"
	require.NoError(t, os.WriteFile(mainFile, []byte(v2), 0o600))
	_, err = mgr.Query(t.Context(), q)
	require.NoError(t, err)

	dc := wireParams(t, params, "textDocument/didChange")
	assert.Contains(t, dc, `"version":2`)
	assert.Contains(t, dc, `"start":{"line":0,"character":0}`)
	// "// " is 3 UTF-16 units, the emoji is 2, and "x" is 1: end column 6.
	assert.Contains(t, dc, `"end":{"line":2,"character":6}`)
	want, errJSON := json.Marshal(v2)
	require.NoError(t, errJSON)
	assert.Contains(t, dc, `"text":`+string(want))
}

func TestSyncLRUEvictionSendsDidClose(t *testing.T) {
	dir, _, _ := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	mgr := openNav(t, dir, "LSP_TEST_HISTORY="+hist, "LSP_TEST_DEF_RESULT=null")

	// 33 documents of 1.1 MB each: the 32 MiB text cap holds 30 documents,
	// so opening them all evicts the three least recently used.
	const files = 33
	const size = 1_100_000
	paths := make([]string, files)
	for i := range files {
		b := append([]byte("package main\n// pad\n"), strings.Repeat("x", size)...)[:size]
		paths[i] = filepath.Join(dir, fmt.Sprintf("f%02d.go", i))
		require.NoError(t, os.WriteFile(paths[i], b, 0o600))
	}
	for _, p := range paths {
		_, err := mgr.Query(t.Context(), Query{Op: OpDefinition, File: p, Line: 1, Character: 1})
		require.NoError(t, err)
	}
	got := history(t, hist)
	assert.Equal(t, files, countMethod(got, "textDocument/didOpen"))
	assert.Equal(t, 3, countMethod(got, "textDocument/didClose"))

	// Revisiting the first file reopens it and evicts the next LRU victim.
	_, err := mgr.Query(t.Context(), Query{Op: OpDefinition, File: paths[0], Line: 1, Character: 1})
	require.NoError(t, err)
	got = history(t, hist)
	assert.Equal(t, files+1, countMethod(got, "textDocument/didOpen"))
	assert.Equal(t, 4, countMethod(got, "textDocument/didClose"))
}

func TestDiagnosticsPushClassesAndFreshness(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	mainURI := uriFromPath(mainFile)
	r := wireRangeJSON(0, 0, 0, 1)
	diag := func(msg string, sev int) string {
		return fmt.Sprintf(`{"message":%q,"severity":%d,"range":%s}`, msg, sev, r)
	}
	pubs := fmt.Sprintf(`[
		{"on":"textDocument/didOpen","uri":%q,"matchDocVersion":true,"diagnostics":[%s]},
		{"on":"textDocument/didChange","docVersion":2,"version":1,"diagnostics":[%s]},
		{"on":"textDocument/didChange","docVersion":2,"diagnostics":[%s]},
		{"on":"textDocument/didChange","docVersion":3,"matchDocVersion":true,"diagnostics":[]},
		{"on":"textDocument/didChange","docVersion":4,"diagnostics":[%s]},
		{"on":"textDocument/didChange","docVersion":5,"diagnostics":[]}
	]`,
		mainURI, diag("A", 1),
		diag("STALE", 1),
		diag("B", 2),
		diag("B2", 2),
	)
	// The pending path must stay fast; the five-second value is frozen
	// elsewhere by not overriding it.
	origWait := diagnosticsWait
	diagnosticsWait = 500 * time.Millisecond
	t.Cleanup(func() { diagnosticsWait = origWait })
	mgr := openNav(t, dir, "LSP_TEST_PUBLISH="+pubs)
	q := Query{Op: OpDiagnostics, File: mainFile}

	// q1: matching-version push published on didOpen is fresh.
	res, err := mgr.Query(t.Context(), q)
	require.NoError(t, err)
	assert.Equal(t, StatusFresh, res.Status)
	require.Len(t, res.Diagnostics, 1)
	assert.Equal(t, "A", res.Diagnostics[0].Message)

	// q2: after a disk edit the stale version-1 publication is ignored and
	// the unversioned one can never be fresh.
	require.NoError(t, os.WriteFile(mainFile, []byte("package main\n// one\n"), 0o600))
	res, err = mgr.Query(t.Context(), q)
	require.NoError(t, err)
	assert.Equal(t, StatusUnconfirmed, res.Status)
	require.Len(t, res.Diagnostics, 1)
	assert.Equal(t, "B", res.Diagnostics[0].Message)
	require.NotEmpty(t, res.Warnings)
	assert.Contains(t, res.Warnings[0], "unconfirmed")

	// q3: an empty matching-version publication clears the push class and is
	// a confirmed empty result; the unversioned follow-up cannot pollute it.
	require.NoError(t, os.WriteFile(mainFile, []byte("package main\n// two\n"), 0o600))
	res, err = mgr.Query(t.Context(), q)
	require.NoError(t, err)
	assert.Equal(t, StatusFresh, res.Status)
	assert.Empty(t, res.Diagnostics)

	// q4: a later unversioned publication lands as unconfirmed again.
	require.NoError(t, os.WriteFile(mainFile, []byte("package main\n// three\n"), 0o600))
	res, err = mgr.Query(t.Context(), q)
	require.NoError(t, err)
	assert.Equal(t, StatusUnconfirmed, res.Status)
	require.Len(t, res.Diagnostics, 1)
	assert.Equal(t, "B2", res.Diagnostics[0].Message)

	// q5: an empty unversioned publication clears only the unconfirmed class.
	require.NoError(t, os.WriteFile(mainFile, []byte("package main\n// four\n"), 0o600))
	res, err = mgr.Query(t.Context(), q)
	require.NoError(t, err)
	assert.Equal(t, StatusUnconfirmed, res.Status)
	assert.Empty(t, res.Diagnostics)
}

func TestDiagnosticsPullMergeDedupCachedUnchanged(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	mainURI := uriFromPath(mainFile)
	r := wireRangeJSON(1, 0, 1, 5)
	item := func(msg string, sev int) string {
		return fmt.Sprintf(
			`{"message":%q,"severity":%d,"code":"%s-code","source":"gopls","range":%s}`,
			msg,
			sev,
			msg,
			r,
		)
	}
	report := fmt.Sprintf(`{"kind":"full","resultId":"r1","items":[{"uri":%q,"diagnostics":[%s,%s]}]}`,
		mainURI, item("P1", 1), item("P2", 2))
	pubs := fmt.Sprintf(`[{"on":"textDocument/didOpen","uri":%q,"matchDocVersion":true,"diagnostics":[%s,%s]}]`,
		mainURI, item("A", 2), item("P1", 1))
	params := paramsPath(t)
	mgr := openNav(t, dir,
		"LSP_TEST_PARAMS="+params,
		"LSP_TEST_EXTRA_CAPS={\"diagnosticProvider\":true}",
		"LSP_TEST_DIAG_UNCHANGED=1",
		"LSP_TEST_DIAG_RESULT="+report,
		"LSP_TEST_PUBLISH="+pubs,
	)
	q := Query{Op: OpDiagnostics, File: mainFile}

	// Pull and matching-version push merge; the duplicated P1 collapses.
	res, err := mgr.Query(t.Context(), q)
	require.NoError(t, err)
	assert.Equal(t, StatusFresh, res.Status)
	require.Len(t, res.Diagnostics, 3)
	seen := map[string]int{}
	for _, d := range res.Diagnostics {
		seen[d.Message]++
	}
	assert.Equal(t, map[string]int{"A": 1, "P1": 1, "P2": 1}, seen)
	assert.Equal(t, "error", res.Diagnostics[0].Severity)
	assert.Equal(t, "P1-code", res.Diagnostics[0].Code)
	assert.Equal(t, "gopls", res.Diagnostics[0].Source)
	assert.Equal(t, "main.go", res.Diagnostics[0].File)
	assert.Equal(t, 2, res.Diagnostics[0].Line)

	// An unchanged snapshot reuses the confirmed merged result as cached.
	res2, err := mgr.Query(t.Context(), q)
	require.NoError(t, err)
	assert.Equal(t, StatusCached, res2.Status)
	assert.Equal(t, res.Diagnostics, res2.Diagnostics)
	assert.Equal(t, 1, countParams(t, params, "textDocument/diagnostic"), "cached skips the pull")

	// After an edit the pull sends the previous result id, the unchanged
	// report keeps the pull items, and the stale push drops out.
	require.NoError(t, os.WriteFile(mainFile, []byte("package main\n// edited\n"), 0o600))
	res3, err := mgr.Query(t.Context(), q)
	require.NoError(t, err)
	assert.Equal(t, StatusFresh, res3.Status)
	require.Len(t, res3.Diagnostics, 2)
	assert.Contains(t, wireParams(t, params, "textDocument/diagnostic"), `"previousResultId":"r1"`)
}

func TestDiagnosticsPendingWithoutData(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	origWait := diagnosticsWait
	diagnosticsWait = 150 * time.Millisecond
	t.Cleanup(func() { diagnosticsWait = origWait })
	mgr := openNav(t, dir) // no pull capability, no publications
	res, err := mgr.Query(t.Context(), Query{Op: OpDiagnostics, File: mainFile})
	require.NoError(t, err)
	assert.Equal(t, StatusPending, res.Status)
	assert.Empty(t, res.Diagnostics, "pending must not claim an empty success")
}

func TestDiagnosticsRestartClearsState(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	mainURI := uriFromPath(mainFile)
	r := wireRangeJSON(0, 0, 0, 1)
	pubs := fmt.Sprintf(
		`[{"on":"textDocument/didOpen","uri":%q,"matchDocVersion":true,"echo":true,"diagnostics":[{"message":"placeholder","severity":1,"range":%s}]}]`,
		mainURI,
		r,
	)
	hist := filepath.Join(t.TempDir(), "history")
	params := paramsPath(t)
	mgr := openNav(t, dir,
		"LSP_TEST_HISTORY="+hist,
		"LSP_TEST_PARAMS="+params,
		"LSP_TEST_PUBLISH="+pubs,
		"LSP_TEST_DIE_ON=textDocument/didChange",
	)
	q := Query{Op: OpDiagnostics, File: mainFile}

	res, err := mgr.Query(t.Context(), q)
	require.NoError(t, err)
	assert.Equal(t, StatusFresh, res.Status)
	require.Len(t, res.Diagnostics, 1)
	assert.Equal(t, fmt.Sprintf("len:%d", len("package main\n\nfunc main() {\n\tf()\n}\n")), res.Diagnostics[0].Message)

	// A disk edit crashes the generation: the next query fails instead of
	// serving stale state.
	require.NoError(t, os.WriteFile(mainFile, []byte("package main\n// shorter\n"), 0o600))
	_, err = mgr.Query(t.Context(), q)
	require.Error(t, err)

	// The restart opens a fresh generation: version restarts at 1 and the
	// published diagnostics reflect the new text, never the old generation.
	res3, err := mgr.Query(t.Context(), q)
	require.NoError(t, err)
	assert.Equal(t, StatusFresh, res3.Status)
	require.Len(t, res3.Diagnostics, 1)
	assert.Equal(t, "len:24", res3.Diagnostics[0].Message)
	assert.Contains(t, wireParams(t, params, "textDocument/didOpen"), `"version":1`)
	assert.Equal(t, 2, countMethod(history(t, hist), "initialize"), "a new generation restarted")
}
