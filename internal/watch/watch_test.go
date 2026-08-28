package watch_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/watch"
)

// scriptedShell streams the given chunks and then exits with code. It stops
// early once the context is cancelled, the way a real process does.
func scriptedShell(chunks []string, code int) watch.ShellFunc {
	return func(ctx context.Context, _ string, onChunk func(string)) (watch.ShellResult, error) {
		for _, chunk := range chunks {
			if ctx.Err() != nil {
				return watch.ShellResult{Canceled: true}, nil
			}
			onChunk(chunk)
		}
		return watch.ShellResult{ExitCode: code}, nil
	}
}

// blockingShell runs until the watch is stopped, like tail -f.
func blockingShell() watch.ShellFunc {
	return func(ctx context.Context, _ string, _ func(string)) (watch.ShellResult, error) {
		<-ctx.Done()
		return watch.ShellResult{Canceled: true}, nil
	}
}

// collect reads n events or fails; a watch that never fires is the bug this
// catches, so the timeout is the assertion.
func collect(t *testing.T, ch <-chan watch.Event, n int) []watch.Event {
	t.Helper()
	out := make([]watch.Event, 0, n)
	deadline := time.After(5 * time.Second)
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("event channel closed after %d of %d events", len(out), n)
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("timed out after %d of %d events: %+v", len(out), n, out)
		}
	}
	return out
}

func TestStreamEmitsEveryMatchingLine(t *testing.T) {
	mgr := watch.New(watch.Options{
		// Chunks split mid-line: the splitter, not the shell, decides what a
		// line is.
		Shell: scriptedShell([]string{"ERROR boom\nfine\n", "ERR", "OR again\nquiet\n"}, 0),
	})
	t.Cleanup(mgr.Close)
	events, cancel := mgr.Subscribe()
	t.Cleanup(cancel)

	w, err := mgr.Start(watch.Spec{Label: "errors", Command: "tail -f app.log", Match: "ERROR"})
	require.NoError(t, err)
	assert.Equal(t, "w1", w.ID)

	got := collect(t, events, 2)
	assert.Equal(t, "ERROR boom", got[0].Text)
	assert.Equal(t, "ERROR again", got[1].Text)
	assert.Equal(t, "errors", got[0].Label)
	assert.False(t, got[0].Final)
}

func TestExitTriggerReportsOnceWithTheTail(t *testing.T) {
	mgr := watch.New(watch.Options{
		Shell: scriptedShell([]string{"building\n", "FAIL: auth_test.go:31\n"}, 2),
	})
	t.Cleanup(mgr.Close)
	events, cancel := mgr.Subscribe()
	t.Cleanup(cancel)

	_, err := mgr.Start(watch.Spec{Label: "tests", Command: "go test ./...", On: watch.OnExit})
	require.NoError(t, err)

	got := collect(t, events, 2)
	assert.Contains(t, got[0].Text, "failed with exit 2")
	assert.Contains(t, got[0].Text, "FAIL: auth_test.go:31")
	assert.Contains(t, got[0].Text, "building", "the tail explains the verdict")
	assert.True(t, got[1].Final, "a finished command says nothing more is coming")
}

// TestFloodStopsTheWatch pins the budget that keeps a bad filter from eating
// the model's context: past the cap the watch stops itself and says why.
func TestFloodStopsTheWatch(t *testing.T) {
	chunks := make([]string, 0, 64)
	for i := range 64 {
		chunks = append(chunks, fmt.Sprintf("line %d\n", i))
	}
	mgr := watch.New(watch.Options{Shell: scriptedShell(chunks, 0)})
	t.Cleanup(mgr.Close)
	events, cancel := mgr.Subscribe()
	t.Cleanup(cancel)

	_, err := mgr.Start(watch.Spec{Label: "everything", Command: "tail -f /var/log/all"})
	require.NoError(t, err)

	var final watch.Event
	seen := 0
	deadline := time.After(5 * time.Second)
	for !final.Final {
		select {
		case ev := <-events:
			if ev.Final {
				final = ev
				continue
			}
			seen++
		case <-deadline:
			t.Fatal("flood never produced a final event")
		}
	}
	assert.LessOrEqual(t, seen, 20, "the cap bounds what reaches a subscriber")
	assert.Contains(t, final.Text, "stopped itself")
	assert.Contains(t, final.Text, "narrow the filter")

	list := mgr.List()
	require.Len(t, list, 1)
	assert.False(t, list[0].Live)
	assert.Contains(t, list[0].Err, "events a minute")
}

// TestFloodBudgetIsSharedByTheSession pins the documented bound: 20 events a
// minute for the whole session, not per watch. Two watches draining one shared
// pool of lines publish twenty between them and both stop at the budget.
func TestFloodBudgetIsSharedByTheSession(t *testing.T) {
	var mu sync.Mutex
	lines := 30
	shell := func(ctx context.Context, _ string, onChunk func(string)) (watch.ShellResult, error) {
		for {
			mu.Lock()
			if lines == 0 {
				mu.Unlock()
				return watch.ShellResult{ExitCode: 0}, nil
			}
			lines--
			mu.Unlock()
			if ctx.Err() != nil {
				return watch.ShellResult{Canceled: true}, nil
			}
			onChunk("line\n")
		}
	}
	mgr := watch.New(watch.Options{Shell: shell})
	t.Cleanup(mgr.Close)
	events, cancel := mgr.Subscribe()
	t.Cleanup(cancel)

	for _, label := range []string{"a", "b"} {
		_, err := mgr.Start(watch.Spec{Label: label, Command: "drain"})
		require.NoError(t, err)
	}

	finals, published := 0, 0
	deadline := time.After(5 * time.Second)
	for finals < 2 {
		select {
		case ev := <-events:
			if ev.Final {
				finals++
				continue
			}
			published++
		case <-deadline:
			t.Fatalf("only %d finals after %d events", finals, published)
		}
	}
	assert.Equal(t, 20, published, "the budget is one window shared by every watch")

	for _, w := range mgr.List() {
		assert.False(t, w.Live)
		assert.Contains(t, w.Err, "across all watches", "the watch that crossed the shared budget says so")
	}
}

func TestStopEndsAWatchAndListRemembersIt(t *testing.T) {
	mgr := watch.New(watch.Options{Shell: blockingShell()})
	t.Cleanup(mgr.Close)
	events, cancel := mgr.Subscribe()
	t.Cleanup(cancel)

	w, err := mgr.Start(watch.Spec{Label: "deploy log", Command: "tail -f deploy.log"})
	require.NoError(t, err)
	assert.Equal(t, 1, mgr.Live())

	require.NoError(t, mgr.Stop(w.ID))
	final := collect(t, events, 1)[0]
	assert.True(t, final.Final)
	assert.Contains(t, final.Text, "ended")

	assert.Equal(t, 0, mgr.Live())
	list := mgr.List()
	require.Len(t, list, 1)
	assert.False(t, list[0].Live)
	assert.Empty(t, list[0].Err, "a watch the user stopped did not fail")

	assert.ErrorIs(t, mgr.Stop("w99"), watch.ErrNotFound)
}

func TestLiveWatchesAreCapped(t *testing.T) {
	mgr := watch.New(watch.Options{Shell: blockingShell()})
	t.Cleanup(mgr.Close)

	for i := range watch.MaxLive {
		_, err := mgr.Start(watch.Spec{Label: fmt.Sprintf("w%d", i), Command: "sleep 1"})
		require.NoError(t, err)
	}
	_, err := mgr.Start(watch.Spec{Label: "one too many", Command: "sleep 1"})
	assert.ErrorIs(t, err, watch.ErrTooMany)

	// A stopped watch frees its slot; the dead one still lists.
	require.NoError(t, mgr.Stop("w1"))
	require.Eventually(t, func() bool { return mgr.Live() < watch.MaxLive }, 5*time.Second, 10*time.Millisecond)
	_, err = mgr.Start(watch.Spec{Label: "room again", Command: "sleep 1"})
	assert.NoError(t, err)
}

func TestLogKeepsWhatAWatchSaw(t *testing.T) {
	mgr := watch.New(watch.Options{Shell: scriptedShell([]string{"one\ntwo\nthree\n"}, 0)})
	t.Cleanup(mgr.Close)
	events, cancel := mgr.Subscribe()
	t.Cleanup(cancel)

	w, err := mgr.Start(watch.Spec{Label: "three lines", Command: "printf 'one\\ntwo\\nthree\\n'"})
	require.NoError(t, err)
	collect(t, events, 4) // three lines plus the final event

	log, err := mgr.Log(w.ID, 0)
	require.NoError(t, err)
	require.Len(t, log, 4)
	assert.Equal(t, "one", log[0].Text)

	tail, err := mgr.Log(w.ID, 2)
	require.NoError(t, err)
	require.Len(t, tail, 2)
	assert.Equal(t, "three", tail[0].Text)

	_, err = mgr.Log("w99", 0)
	assert.ErrorIs(t, err, watch.ErrNotFound)
}

// TestFinishedWatchLogsAreCapped pins the retention bound: the last
// FinishedLogKeep finished watches keep their history for action=log, older
// ones keep only the final event that says how they ended.
func TestFinishedWatchLogsAreCapped(t *testing.T) {
	mgr := watch.New(watch.Options{
		Shell: scriptedShell([]string{"one\ntwo\n"}, 0),
	})
	t.Cleanup(mgr.Close)

	ids := make([]string, 0, watch.FinishedLogKeep+1)
	for range watch.FinishedLogKeep + 1 {
		w, err := mgr.Start(watch.Spec{Label: "two lines", Command: "printf"})
		require.NoError(t, err)
		ids = append(ids, w.ID)
		// Let each one finish before the next starts, so the finished list
		// holds them in start order and the oldest is the one evicted.
		require.Eventually(t, func() bool {
			for _, l := range mgr.List() {
				if l.ID == w.ID {
					return !l.Live
				}
			}
			return false
		}, 5*time.Second, 10*time.Millisecond)
	}

	log, err := mgr.Log(ids[0], 0)
	require.NoError(t, err)
	require.Len(t, log, 1, "past the cap a finished watch keeps only its final event")
	assert.True(t, log[0].Final)

	fresh, err := mgr.Log(ids[len(ids)-1], 0)
	require.NoError(t, err)
	require.Len(t, fresh, 3, "the newest finished watch keeps its full log")
	assert.Equal(t, "one", fresh[0].Text)
}

func TestInvalidSpecsAreRejected(t *testing.T) {
	mgr := watch.New(watch.Options{Shell: blockingShell()})
	t.Cleanup(mgr.Close)

	cases := map[string]watch.Spec{
		"nothing to run":     {Label: "empty"},
		"timer with no name": {Every: time.Minute},
		"interval too small": {Label: "fast", Command: "date", Every: time.Second},
		"once with no clock": {Label: "soon", Command: "date", Once: true},
		"exit while polling": {Label: "poll", Command: "date", Every: time.Minute, On: watch.OnExit},
		"unknown trigger":    {Label: "odd", Command: "date", On: watch.Trigger("whenever")},
		"broken regexp":      {Label: "bad", Command: "date", Match: "("},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := mgr.Start(spec)
			assert.ErrorIs(t, err, watch.ErrInvalid)
		})
	}
}

// TestCloseEndsEverything pins the shutdown contract: no watch outlives the
// manager, and no subscriber is left waiting on a channel nobody will close.
func TestCloseEndsEverything(t *testing.T) {
	mgr := watch.New(watch.Options{Shell: blockingShell()})
	events, cancel := mgr.Subscribe()
	defer cancel()

	_, err := mgr.Start(watch.Spec{Label: "long", Command: "tail -f x"})
	require.NoError(t, err)

	mgr.Close()
	mgr.Close() // idempotent

	drained := 0
	for range events {
		drained++
	}
	assert.Positive(t, drained, "the final event reaches a subscriber before the channel closes")
	assert.Equal(t, 0, mgr.Live())

	_, err = mgr.Start(watch.Spec{Label: "after close", Command: "date"})
	assert.ErrorIs(t, err, watch.ErrClosed)
}

// TestUnsubscribeStopsDelivery pins that a cancelled subscription neither
// receives nor blocks the watch that keeps firing.
func TestUnsubscribeStopsDelivery(t *testing.T) {
	mgr := watch.New(watch.Options{Shell: scriptedShell([]string{"one\n"}, 0)})
	t.Cleanup(mgr.Close)

	events, cancel := mgr.Subscribe()
	cancel()
	cancel() // idempotent

	_, err := mgr.Start(watch.Spec{Label: "orphan", Command: "echo one"})
	require.NoError(t, err)

	for range events { //nolint:revive // draining a closed channel
	}
	require.Eventually(t, func() bool {
		log, err := mgr.Log("w1", 0)
		return err == nil && len(log) >= 2
	}, 5*time.Second, 10*time.Millisecond, "the watch runs on with nobody listening")
}

func TestLabelDefaultsToTheCommand(t *testing.T) {
	mgr := watch.New(watch.Options{Shell: blockingShell()})
	t.Cleanup(mgr.Close)

	w, err := mgr.Start(watch.Spec{Command: "  tail -f app.log  "})
	require.NoError(t, err)
	assert.Equal(t, "tail -f app.log", w.Label)
	assert.Equal(t, "tail -f app.log", w.Command)
	assert.Equal(t, watch.OnLine, w.On)
}

func TestCwdReachesTheShell(t *testing.T) {
	seen := make(chan string, 1)
	mgr := watch.New(watch.Options{
		Cwd: func() string { return "/tmp/somewhere" },
		Shell: func(ctx context.Context, command string, _ func(string)) (watch.ShellResult, error) {
			seen <- strings.TrimSpace(command)
			<-ctx.Done()
			return watch.ShellResult{Canceled: true}, nil
		},
	})
	t.Cleanup(mgr.Close)

	_, err := mgr.Start(watch.Spec{Label: "here", Command: "pwd"})
	require.NoError(t, err)
	select {
	case cmd := <-seen:
		assert.Equal(t, "pwd", cmd)
	case <-time.After(5 * time.Second):
		t.Fatal("the shell was never called")
	}
}
