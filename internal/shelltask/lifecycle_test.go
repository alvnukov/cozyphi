package shelltask_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alvnukov/cozyphi/internal/proc"
	"github.com/alvnukov/cozyphi/internal/shelltask"
)

// A stopped process never reached an exit of its own; the kill leaves a zero
// behind it, and a receipt that reported that zero would read as success. The
// snapshot must carry the stop as state, never as an outcome.
func TestStoppedOutcomeCarriesNoExitCodeOrZeroDeadline(t *testing.T) {
	started := make(chan struct{})
	m := newManager(t, func(ctx context.Context, spec proc.Spec, _ proc.Limit) (proc.Result, error) {
		close(started)
		<-ctx.Done()
		spec.Stream("partial output\n")
		return proc.Result{Canceled: true}, nil
	})
	changed, unsubscribe := m.Subscribe()
	defer unsubscribe()
	result, err := m.Run(t.Context(), request("stopme", true))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err := m.Stop(result.Snapshot.ID); err != nil {
		t.Fatal(err)
	}
	outcome := awaitOutcome(t, m, changed)
	if outcome.State != shelltask.Stopped {
		t.Fatalf("state=%s", outcome.State)
	}
	if outcome.ExitCode != nil {
		t.Fatalf("stopped snapshot carries exit code %d", *outcome.ExitCode)
	}
	if outcome.Deadline != nil {
		t.Fatal("no timeout was set, but the receipt carries a deadline")
	}
	if outcome.Finished == nil {
		t.Fatal("a terminal snapshot must say when it finished")
	}
	data, err := json.Marshal(outcome)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "exit_code") || strings.Contains(text, "deadline") {
		t.Fatalf("stopped receipt invents lifecycle facts: %s", text)
	}
	if !strings.Contains(text, `"state":"stopped"`) || !strings.Contains(text, `"finished"`) {
		t.Fatalf("stopped receipt misses its own facts: %s", text)
	}
}

// An explicit timeout stays in the receipt, and a completed process keeps its
// real exit code — including zero, which a pointer keeps visible.
func TestCompletedOutcomeWithTimeoutKeepsDeadlineAndExitCode(t *testing.T) {
	m := newManager(t, func(_ context.Context, _ proc.Spec, _ proc.Limit) (proc.Result, error) {
		return proc.Result{Output: "ok"}, nil
	})
	changed, unsubscribe := m.Subscribe()
	defer unsubscribe()
	req := request("timed", true)
	req.Timeout = time.Minute
	if _, err := m.Run(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	outcome := awaitOutcome(t, m, changed)
	if outcome.State != shelltask.Completed {
		t.Fatalf("state=%s", outcome.State)
	}
	if outcome.Deadline == nil || outcome.Finished == nil || outcome.ExitCode == nil || *outcome.ExitCode != 0 {
		t.Fatalf("completed receipt=%+v", outcome.Snapshot)
	}
	data, err := json.Marshal(outcome)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, fact := range []string{`"state":"completed"`, `"exit_code":0`, `"deadline"`, `"finished"`} {
		if !strings.Contains(text, fact) {
			t.Fatalf("completed receipt misses %s: %s", fact, text)
		}
	}
}
