package shelltask_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvnukov/cozyphi/internal/proc"
	"github.com/alvnukov/cozyphi/internal/shelltask"
)

func newManager(t *testing.T, runner shelltask.Runner) *shelltask.Manager {
	t.Helper()
	manager, err := shelltask.New(t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Error(err)
		}
	})
	return manager
}

func request(id string, background bool) shelltask.Request {
	return shelltask.Request{
		ParentSessionID: "parent", ToolUseID: id, Command: "test command",
		Spec: proc.Spec{Argv: []string{"unused"}}, Background: background,
	}
}

func awaitOutcome(t *testing.T, m *shelltask.Manager, changed <-chan struct{}) shelltask.Outcome {
	t.Helper()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		outcomes, err := m.Pending("parent", 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(outcomes) > 0 {
			return outcomes[0]
		}
		select {
		case <-changed:
		case <-timeout.C:
			t.Fatal("terminal receipt was not delivered")
		}
	}
}

func TestBackgroundSurvivesTurnCancellationAndPersistsBeforeHint(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	m := newManager(t, func(ctx context.Context, spec proc.Spec, _ proc.Limit) (proc.Result, error) {
		close(started)
		<-finish
		if err := ctx.Err(); err != nil {
			return proc.Result{}, err
		}
		spec.Stream("stdout and stderr\n")
		return proc.Result{Output: "stdout and stderr\n"}, nil
	})
	changed, unsubscribe := m.Subscribe()
	defer unsubscribe()
	ctx, cancel := context.WithCancel(t.Context())
	result, err := m.Run(ctx, request("one", true))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	close(finish)
	outcome := awaitOutcome(t, m, changed)
	if outcome.State != shelltask.Completed || outcome.ID != result.Snapshot.ID {
		t.Fatalf("outcome=%+v", outcome)
	}
	data, err := os.ReadFile(outcome.OutputFile)
	if err != nil || string(data) != "stdout and stderr\n" {
		t.Fatalf("output=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(outcome.OutputFile), "result.json")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(outcome.OutputFile)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("private output: %v %v", info, err)
	}
	if other, err := m.Pending("other", 4); err != nil || len(other) != 0 {
		t.Fatalf("cross-session outcomes=%v err=%v", other, err)
	}
	if err := m.Acknowledge("other", outcome.EventID); err == nil {
		t.Fatal("cross-session acknowledgement accepted")
	}
	if err := m.Acknowledge("parent", outcome.EventID); err != nil {
		t.Fatal(err)
	}
	if err := m.Acknowledge("parent", outcome.EventID); err != nil {
		t.Fatal(err)
	}
	if pending, err := m.Pending("parent", 4); err != nil || len(pending) != 0 {
		t.Fatalf("ack left pending=%v err=%v", pending, err)
	}
}

func TestPromotionRunsOnceAndPreservesDeadline(t *testing.T) {
	started := make(chan time.Time, 1)
	finish := make(chan struct{})
	runs := 0
	m := newManager(t, func(ctx context.Context, spec proc.Spec, _ proc.Limit) (proc.Result, error) {
		runs++
		deadline, _ := ctx.Deadline()
		started <- deadline
		<-finish
		if err := ctx.Err(); err != nil {
			return proc.Result{}, err
		}
		spec.Stream("completed")
		return proc.Result{Output: "completed"}, nil
	})
	changed, unsubscribe := m.Subscribe()
	defer unsubscribe()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req := request("promote", false)
	req.Timeout = time.Minute
	resultCh := make(chan shelltask.Result, 1)
	errorCh := make(chan error, 1)
	go func() { res, err := m.Run(ctx, req); resultCh <- res; errorCh <- err }()
	deadline := <-started
	snapshots := m.List("parent")
	if len(snapshots) != 1 {
		t.Fatalf("snapshots=%v", snapshots)
	}
	if err := m.Background(snapshots[0].ID); err != nil {
		t.Fatal(err)
	}
	result := <-resultCh
	if err := <-errorCh; err != nil {
		t.Fatal(err)
	}
	if !result.Snapshot.Background || !result.Snapshot.Deadline.Equal(deadline) {
		t.Fatalf("promotion=%+v deadline=%v", result.Snapshot, deadline)
	}
	cancel()
	close(finish)
	outcome := awaitOutcome(t, m, changed)
	if outcome.State != shelltask.Completed || runs != 1 {
		t.Fatalf("state=%s runs=%d", outcome.State, runs)
	}
	if err := m.Background(outcome.ID); !errors.Is(err, shelltask.ErrFinished) {
		t.Fatalf("promote finished: %v", err)
	}
}

func TestForegroundCancellationJoinsBeforeReturning(t *testing.T) {
	started := make(chan struct{})
	m := newManager(t, func(ctx context.Context, _ proc.Spec, _ proc.Limit) (proc.Result, error) {
		close(started)
		<-ctx.Done()
		return proc.Result{Canceled: true}, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	results := make(chan shelltask.Result, 1)
	errs := make(chan error, 1)
	go func() { res, err := m.Run(ctx, request("cancel", false)); results <- res; errs <- err }()
	<-started
	cancel()
	res := <-results
	if err := <-errs; !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if res.Snapshot.State != shelltask.Stopped {
		t.Fatalf("state=%s", res.Snapshot.State)
	}
	if pending, err := m.Pending("parent", 1); err != nil || len(pending) != 0 {
		t.Fatalf("foreground became notification: %v %v", pending, err)
	}
}

func TestStopIsRequestUntilReapedAndIdempotent(t *testing.T) {
	canceled, reap := make(chan struct{}), make(chan struct{})
	m := newManager(t, func(ctx context.Context, _ proc.Spec, _ proc.Limit) (proc.Result, error) {
		<-ctx.Done()
		close(canceled)
		<-reap
		return proc.Result{Canceled: true}, nil
	})
	changed, unsubscribe := m.Subscribe()
	defer unsubscribe()
	res, err := m.Run(t.Context(), request("stop", true))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(res.Snapshot.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(res.Snapshot.ID); err != nil {
		t.Fatal(err)
	}
	<-canceled
	if got := m.List("parent")[0].State; got != shelltask.Running {
		t.Fatalf("unreaped state=%s", got)
	}
	close(reap)
	outcome := awaitOutcome(t, m, changed)
	if outcome.State != shelltask.Stopped {
		t.Fatalf("state=%s", outcome.State)
	}
	if err := m.Stop(outcome.ID); err != nil {
		t.Fatal(err)
	}
}

func TestOutputBoundsAndOwnedReadDoNotFollowReplacementPath(t *testing.T) {
	finish := make(chan struct{})
	payload := strings.Repeat("x", shelltask.OutputFileBytes+100) + "TAIL"
	m := newManager(t, func(_ context.Context, spec proc.Spec, _ proc.Limit) (proc.Result, error) {
		<-finish
		spec.Stream(payload)
		return proc.Result{}, nil
	})
	changed, unsubscribe := m.Subscribe()
	defer unsubscribe()
	res, err := m.Run(t.Context(), request("bounds", true))
	if err != nil {
		t.Fatal(err)
	}
	close(finish)
	outcome := awaitOutcome(t, m, changed)
	info, err := os.Stat(outcome.OutputFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != shelltask.OutputFileBytes || !outcome.Truncated || len(outcome.Output) > shelltask.TailBytes {
		t.Fatalf("bounds: size=%d truncated=%t tail=%d", info.Size(), outcome.Truncated, len(outcome.Output))
	}
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(outcome.OutputFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, outcome.OutputFile); err != nil {
		t.Fatal(err)
	}
	output, err := m.Output(res.Snapshot.ID, 4)
	if err != nil || output != "TAIL" {
		t.Fatalf("owned read=%q err=%v", output, err)
	}
}

func TestDeadlineClassifiedAndCloseStopsBackground(t *testing.T) {
	m := newManager(t, func(ctx context.Context, _ proc.Spec, _ proc.Limit) (proc.Result, error) {
		<-ctx.Done()
		return proc.Result{ExitCode: -1}, nil
	})
	changed, unsubscribe := m.Subscribe()
	defer unsubscribe()
	req := request("deadline", true)
	req.Timeout = 10 * time.Millisecond
	if _, err := m.Run(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	outcome := awaitOutcome(t, m, changed)
	if outcome.State != shelltask.Failed || !strings.Contains(outcome.Error, "timed out") {
		t.Fatalf("outcome=%+v", outcome)
	}
	res, err := m.Run(t.Context(), request("close", true))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range m.List("") {
		if snapshot.ID == res.Snapshot.ID && snapshot.State != shelltask.Stopped {
			t.Fatalf("close state=%s", snapshot.State)
		}
	}
	if _, err := m.Run(t.Context(), request("closed", true)); !errors.Is(err, shelltask.ErrClosed) {
		t.Fatalf("closed accepted work: %v", err)
	}
}

func TestLiveCapAndSilentSubscriberCannotLoseTerminalReceipts(t *testing.T) {
	finish := make(chan struct{})
	m := newManager(t, func(_ context.Context, _ proc.Spec, _ proc.Limit) (proc.Result, error) {
		<-finish
		return proc.Result{}, nil
	})
	changed, unsubscribe := m.Subscribe()
	defer unsubscribe()
	for i := range shelltask.MaxLive {
		if _, err := m.Run(t.Context(), request(string(rune('a'+i)), true)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Run(t.Context(), request("over", true)); err == nil {
		t.Fatal("live cap not enforced")
	}
	close(finish)
	_ = awaitOutcome(t, m, changed)
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	all, err := m.Pending("parent", shelltask.MaxLive)
	if err != nil || len(all) != shelltask.MaxLive {
		t.Fatalf("terminal receipts=%d err=%v", len(all), err)
	}
}
