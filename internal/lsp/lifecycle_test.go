package lsp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shortenDeadline swaps a package-level deadline for the test and restores it
// afterwards, keeping the frozen public surface intact.
func shortenDeadline(t *testing.T, d *time.Duration, v time.Duration) {
	t.Helper()
	old := *d
	*d = v
	t.Cleanup(func() { *d = old })
}

// TestInitializeTimesOut proves a server that never answers initialize fails
// the query with a typed unavailable error inside the handshake deadline, not
// the caller's patience.
func TestInitializeTimesOut(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	gate := filepath.Join(t.TempDir(), "init-gate")
	shortenDeadline(t, &initializeTimeout, 300*time.Millisecond)
	mgr := openNav(t, dir,
		"LSP_TEST_HISTORY="+hist,
		"LSP_TEST_INIT_GATE="+gate,
	)

	start := time.Now()
	_, err := mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second, "handshake deadline must bound the wait")
	var le *Error
	require.ErrorAs(t, err, &le, "want typed error")
	assert.Equal(t, ErrUnavailable, le.Kind)
	assert.Contains(t, le.Message, "initialize")

	// Release the fake so teardown reaps it promptly instead of waiting out
	// the kill grace.
	require.NoError(t, os.WriteFile(gate+".go", []byte("go"), 0o600))
	assert.Equal(t, 1, countMethod(history(t, hist), "initialize"), "one real spawn happened")
}

// waitForCount polls the fake server history until method appears want times.
// The client write and the fake's read loop run in different processes, so a
// just-sent $/cancelRequest may not be recorded the instant Query returns.
func waitForCount(t *testing.T, hist, method string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for countMethod(history(t, hist), method) < want {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d %s", want, method)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestQueryTimesOutSendsCancel proves an ordinary query that never gets its
// response fails at the frozen deadline, sends $/cancelRequest, discards the
// late response, and leaves the shared generation alive.
func TestQueryTimesOutSendsCancel(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	shortenDeadline(t, &queryTimeout, 300*time.Millisecond)
	// DEF_BATCH=2 holds the response until a second definition arrives.
	mgr := openNav(t, dir,
		"LSP_TEST_HISTORY="+hist,
		"LSP_TEST_DEF_BATCH=2",
		"LSP_TEST_DEF_RESULT=null",
	)

	_, err := mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	waitForCount(t, hist, "$/cancelRequest", 1)

	// The partner definition completes the batch: this query succeeds on the
	// same generation and the abandoned response is dropped silently.
	_, err = mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
	require.NoError(t, err)
	assert.Equal(t, 1, countMethod(history(t, hist), "initialize"), "timeout must not restart gopls")
}

// TestCircuitBreakerRefusesFourthStart proves a crashing server consumes
// start quota: three real spawns, then a typed refusal with a retry hint that
// starts nothing.
func TestCircuitBreakerRefusesFourthStart(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	mgr := openNav(t, dir,
		"LSP_TEST_HISTORY="+hist,
		"LSP_TEST_DIE_ON=initialize",
	)
	q := Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2}
	for range maxStartAttempts {
		_, err := mgr.Query(t.Context(), q)
		require.Error(t, err, "a crashed initialize must fail the query")
	}

	_, err := mgr.Query(t.Context(), q)
	require.Error(t, err)
	var le *Error
	require.ErrorAs(t, err, &le, "want typed error")
	assert.Equal(t, ErrUnavailable, le.Kind)
	assert.Contains(t, le.Message, "retry_after_seconds")
	assert.Equal(t, maxStartAttempts, countMethod(history(t, hist), "initialize"),
		"the refusal must not start a process")
}

// TestCloseCancelsPendingAndClosesDocuments pins the exact close order:
// pending requests get $/cancelRequest, open documents get didClose, then
// shutdown, exit, and the bounded reap. New queries are rejected afterwards.
func TestCloseCancelsPendingAndClosesDocuments(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	ready := filepath.Join(t.TempDir(), "def-ready")
	mgr := openNav(t, dir,
		"LSP_TEST_HISTORY="+hist,
		"LSP_TEST_DEF_BATCH=2",
		"LSP_TEST_DEF_READY="+ready,
		"LSP_TEST_DEF_RESULT=null",
	)

	// Sync the document so the close path owes the server a didClose.
	_, err := mgr.Query(t.Context(), Query{Op: OpDiagnostics, File: mainFile})
	require.NoError(t, err)
	require.Equal(t, 1, countMethod(history(t, hist), "textDocument/didOpen"))

	qErr := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
			qErr <- err
		}()
	}
	waitForFile(ready) // both definitions are pending inside the fake

	closeDone := make(chan error, 1)
	go func() { closeDone <- mgr.Close(t.Context()) }()
	// Release the fake only after both $/cancelRequest have reached its
	// history: a logged cancel proves Close already dropped the pending
	// slots, so a drained batch can no longer complete a query. Releasing
	// earlier raced Close's cancel step — under a coverage-instrumented
	// build the slow manager let the fast fake answer first, and one
	// in-flight query returned success instead of its generation error.
	waitForCount(t, hist, "$/cancelRequest", 2)
	require.NoError(t, os.WriteFile(ready+".go", []byte("go"), 0o600))

	require.Error(t, <-qErr, "the in-flight query must fail with its generation")
	require.Error(t, <-qErr, "both in-flight queries must fail with the generation")
	require.NoError(t, <-closeDone)

	got := history(t, hist)
	require.Equal(t, 2, countMethod(got, "$/cancelRequest"))
	require.Equal(t, 1, countMethod(got, "textDocument/didClose"))
	require.Equal(t, 1, countMethod(got, "shutdown"))
	assert.Less(t, slices.Index(got, "textDocument/didClose"), slices.Index(got, "shutdown"),
		"didClose must precede shutdown")
	assert.Equal(t, "exit", got[len(got)-1], "exit must be the final message")

	_, err = mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
	var le *Error
	require.ErrorAs(t, err, &le)
	assert.Equal(t, ErrClosed, le.Kind)
}

// TestConcurrentQueryCancelClose races queries, per-query cancels, and two
// concurrent Closes: nothing may panic, hang, or survive as a pending call.
func TestConcurrentQueryCancelClose(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	// DEF_BATCH=2 + DEF_RESULT=null makes definitions answer in pairs with a
	// valid null payload, so queries genuinely complete or cancel.
	mgr := openNav(t, dir,
		"LSP_TEST_DEF_BATCH=2",
		"LSP_TEST_DEF_RESULT=null",
	)

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			ctx, cancel := context.WithCancel(t.Context())
			go func() {
				time.Sleep(time.Duration(i) * 5 * time.Millisecond)
				cancel()
			}()
			_, _ = mgr.Query(ctx, Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
		})
	}
	wg.Go(func() { _ = mgr.Close(t.Context()) })
	wg.Go(func() { _ = mgr.Close(t.Context()) })
	wg.Wait()

	_, err := mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
	var le *Error
	require.ErrorAs(t, err, &le)
	assert.Equal(t, ErrClosed, le.Kind)
}

// TestLeakStartCrashRestartClose repeats start/crash/restart cycles and proves
// goroutines settle back near the baseline while the breaker finally refuses
// the fourth generation.
func TestLeakStartCrashRestartClose(t *testing.T) {
	base := runtime.NumGoroutine()
	dir, mainFile, _ := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	mgr := openNav(t, dir,
		"LSP_TEST_HISTORY="+hist,
		"LSP_TEST_DIE_ON=textDocument/hover",
		"LSP_TEST_DEF_RESULT=null",
	)
	def := Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2}
	hov := Query{Op: OpHover, File: mainFile, Line: 5, Character: 2}

	for range maxStartAttempts {
		_, err := mgr.Query(t.Context(), def)
		require.NoError(t, err)
		_, err = mgr.Query(t.Context(), hov)
		require.Error(t, err, "hover must crash the generation")
	}
	_, err := mgr.Query(t.Context(), def)
	var le *Error
	require.ErrorAs(t, err, &le)
	assert.Equal(t, ErrUnavailable, le.Kind)

	require.NoError(t, mgr.Close(t.Context()))
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > base+3 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	assert.LessOrEqual(t, runtime.NumGoroutine(), base+3, "goroutines must settle after close")
	assert.Equal(t, maxStartAttempts, countMethod(history(t, hist), "initialize"))
}
