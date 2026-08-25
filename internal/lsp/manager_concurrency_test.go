package lsp

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func countMethod(hist []string, method string) int {
	n := 0
	for _, m := range hist {
		if m == method {
			n++
		}
	}
	return n
}

// TestConcurrentQueriesRouteResponsesByID drives two concurrent definition
// queries against one manager. The fake server answers them in reverse arrival
// order; each caller must still receive the result for its own request, and the
// two requests must share exactly one process generation.
func TestConcurrentQueriesRouteResponsesByID(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	mgr, err := Open(t.Context(), dir, fakeConfig(
		"LSP_TEST_HISTORY="+hist,
		"LSP_TEST_DEF_BATCH=2",
		"LSP_TEST_MAIN_URI="+uriFromPath(mainFile),
		"LSP_TEST_OTHER_URI="+uriFromPath(otherFile),
	))
	require.NoError(t, err)
	defer mgr.Close(t.Context())

	var wg sync.WaitGroup
	var aRes, bRes Result
	var aErr, bErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		aRes, aErr = mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
	}()
	go func() {
		defer wg.Done()
		bRes, bErr = mgr.Query(t.Context(), Query{Op: OpDefinition, File: otherFile, Line: 3, Character: 6})
	}()
	wg.Wait()

	require.NoError(t, aErr)
	require.NoError(t, bErr)
	require.Len(t, aRes.Locations, 1)
	require.Len(t, bRes.Locations, 1)
	if aRes.Locations[0].File != "other.go" {
		t.Fatalf("query on main.go must resolve to other.go, got %+v", aRes.Locations[0])
	}
	if bRes.Locations[0].File != "main.go" {
		t.Fatalf("query on other.go must resolve to main.go, got %+v", bRes.Locations[0])
	}

	got := history(t, hist)
	if n := countMethod(got, "initialize"); n != 1 {
		t.Fatalf("initialize count = %d, want 1 (%v)", n, got)
	}
}

// TestCoalescedInitSurvivesFirstCallerCancel proves the singleflight task uses
// the Manager lifetime, not the first caller's context: canceling that caller
// during initialize neither aborts startup nor spawns a second process.
func TestCoalescedInitSurvivesFirstCallerCancel(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	gate := filepath.Join(t.TempDir(), "init-gate")
	mgr, err := Open(t.Context(), dir, fakeConfig(
		"LSP_TEST_HISTORY="+hist,
		"LSP_TEST_INIT_GATE="+gate,
		"LSP_TEST_DEF_RESULT=null",
	))
	require.NoError(t, err)
	defer mgr.Close(t.Context())

	ctxA, cancelA := context.WithCancel(t.Context())
	var aErr error
	aDone := make(chan struct{})
	go func() {
		defer close(aDone)
		_, aErr = mgr.Query(ctxA, Query{Op: OpDefinition, File: mainFile, Line: 1, Character: 1})
	}()

	// Wait until the fake server has received initialize, then cancel the
	// first caller while startup is still in flight.
	waitForFile(gate)
	cancelA()

	var bRes Result
	var bErr error
	bDone := make(chan struct{})
	go func() {
		defer close(bDone)
		bRes, bErr = mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 1, Character: 1})
	}()

	require.NoError(t, os.WriteFile(gate+".go", []byte("go"), 0o600))
	<-bDone
	<-aDone

	require.NoError(t, bErr)
	require.Empty(t, bRes.Locations)
	require.ErrorIs(t, aErr, context.Canceled)
	if n := countMethod(history(t, hist), "initialize"); n != 1 {
		t.Fatalf("initialize count = %d, want 1", n)
	}
}

// TestPerQueryCancelDoesNotAffectOtherQuery proves a cancelled child query
// only removes its own pending slot; a concurrent parent query completes with
// its own result and the healthy process survives.
func TestPerQueryCancelDoesNotAffectOtherQuery(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	ready := filepath.Join(t.TempDir(), "def-ready")
	mgr, err := Open(t.Context(), dir, fakeConfig(
		"LSP_TEST_HISTORY="+hist,
		"LSP_TEST_DEF_BATCH=2",
		"LSP_TEST_DEF_READY="+ready,
		"LSP_TEST_MAIN_URI="+uriFromPath(mainFile),
		"LSP_TEST_OTHER_URI="+uriFromPath(otherFile),
	))
	require.NoError(t, err)
	defer mgr.Close(t.Context())

	ctxA, cancelA := context.WithCancel(t.Context())
	var aErr error
	aDone := make(chan struct{})
	go func() {
		defer close(aDone)
		_, aErr = mgr.Query(ctxA, Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
	}()

	var bRes Result
	var bErr error
	bDone := make(chan struct{})
	go func() {
		defer close(bDone)
		bRes, bErr = mgr.Query(t.Context(), Query{Op: OpDefinition, File: otherFile, Line: 3, Character: 6})
	}()

	// Both definition requests are pending in the fake server.
	waitForFile(ready)
	cancelA()
	require.NoError(t, os.WriteFile(ready+".go", []byte("go"), 0o600))

	<-bDone
	<-aDone

	require.ErrorIs(t, aErr, context.Canceled)
	require.NoError(t, bErr)
	require.Len(t, bRes.Locations, 1)
	if bRes.Locations[0].File != "main.go" {
		t.Fatalf("parent query must resolve to main.go, got %+v", bRes.Locations[0])
	}
}

// TestSharedRuntimeSingleGeneration models one primary agent plus four children
// issuing definition queries against the shared Manager. All five must resolve
// against exactly one gopls process generation (one initialize), matching the
// shared-runtime contract for parent + sub-agents.
func TestSharedRuntimeSingleGeneration(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	mgr, err := Open(t.Context(), dir, fakeConfig(
		"LSP_TEST_HISTORY="+hist,
		"LSP_TEST_DEF_BATCH=5",
		"LSP_TEST_MAIN_URI="+uriFromPath(mainFile),
		"LSP_TEST_OTHER_URI="+uriFromPath(otherFile),
	))
	require.NoError(t, err)
	defer mgr.Close(t.Context())

	const agents = 5 // one primary + four children
	var (
		wg      sync.WaitGroup
		results = make([]Result, agents)
		errs    = make([]error, agents)
	)
	for i := range agents {
		wg.Go(func() {
			results[i], errs[i] = mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
		})
	}
	wg.Wait()

	for i := range agents {
		require.NoError(t, errs[i])
		require.Len(t, results[i].Locations, 1)
		if results[i].Locations[0].File != "other.go" {
			t.Fatalf("agent %d must resolve to other.go, got %+v", i, results[i].Locations[0])
		}
	}
	if n := countMethod(history(t, hist), "initialize"); n != 1 {
		t.Fatalf("initialize count = %d, want 1 shared generation", n)
	}
}
