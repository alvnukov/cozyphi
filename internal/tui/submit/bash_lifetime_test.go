package submit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

type admissionComposer struct {
	stubComposer
	entered chan struct{}
	release chan struct{}
}

func (c *admissionComposer) HideCompleters() {
	close(c.entered)
	<-c.release
}

func shellEvents() (chan session.ToolRun, func(controller.Msg)) {
	events := make(chan session.ToolRun, 128)
	return events, func(msg controller.Msg) {
		if ev, ok := msg.(controller.SessionEventMsg); ok {
			if data, ok := ev.Event.(session.ToolData); ok {
				events <- data.Run
			}
		}
	}
}

func awaitShellResult(t *testing.T, events <-chan session.ToolRun) session.ToolRun {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case run := <-events:
			if run.Status != session.ToolInProgress {
				return run
			}
		case <-deadline:
			t.Fatal("timed out waiting for shell result")
		}
	}
}

func closeShell(t *testing.T, runner *BashRunner) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
	defer cancel()
	if err := runner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if runner.Running() || runner.Cancel() {
		t.Fatal("closed runner still owns running work")
	}
}

func TestBashRunnerInDirRunsInCanonicalDirectory(t *testing.T) {
	dir := t.TempDir()
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "cwd")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	events, publish := shellEvents()
	runner, err := NewBashRunnerInDir(nil, stubComposer{}, nil, publish, link)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeShell(t, runner) })
	// The runner owns the resolved directory, not a mutable symlink or global cwd.
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if !runner.HandleSubmit("!pwd -P") {
		t.Fatal("shell input not consumed")
	}
	run := awaitShellResult(t, events)
	if run.Status != session.ToolDone || strings.TrimSpace(run.Output) != canonical {
		t.Fatalf("pwd result: %+v; want %q", run, canonical)
	}
}

func TestBashRunnerInDirRejectsBadDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, cwd := range []string{"", filepath.Join(dir, "missing"), file} {
		t.Run(cwd, func(t *testing.T) {
			runner, err := NewBashRunnerInDir(nil, stubComposer{}, nil, nil, cwd)
			if err == nil || runner != nil {
				t.Fatalf("bad directory accepted: runner=%v err=%v", runner, err)
			}
		})
	}
}

func TestBashRunnerLegacyCapturesConstructionDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	events, publish := shellEvents()
	runner := NewBashRunner(nil, stubComposer{}, nil, publish)
	t.Cleanup(func() { closeShell(t, runner) })
	t.Chdir(t.TempDir())
	runner.HandleSubmit("!pwd -P")
	run := awaitShellResult(t, events)
	if run.Status != session.ToolDone || strings.TrimSpace(run.Output) != canonical {
		t.Fatalf("legacy pwd result: %+v; want %q", run, canonical)
	}
}

func TestBashRunnerAdmissionCancelDuplicateAndClose(t *testing.T) {
	composer := &admissionComposer{entered: make(chan struct{}), release: make(chan struct{})}
	events, publish := shellEvents()
	runner, err := NewBashRunnerInDir(nil, composer, nil, publish, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var release sync.Once
	submitted := make(chan bool, 1)
	t.Cleanup(func() {
		release.Do(func() { close(composer.release) })
		closeShell(t, runner)
	})
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	go func() { submitted <- runner.HandleSubmit("!touch '" + marker + "'") }()
	select {
	case <-composer.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("admission did not reach composer")
	}
	// No shell goroutine exists yet; admission must already be cancellable.
	if !runner.Running() || !runner.Cancel() {
		t.Fatal("accepted work not immediately visible/cancellable")
	}
	if !runner.HandleSubmit("!echo duplicate") {
		t.Fatal("duplicate shell input not consumed")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := runner.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close before launch = %v, want deadline", err)
	}
	if !runner.Running() || !runner.Cancel() {
		t.Fatal("timed-out Close relinquished ownership")
	}
	release.Do(func() { close(composer.release) })
	if !<-submitted {
		t.Fatal("initial shell input not consumed")
	}
	closeShell(t, runner)
	run := awaitShellResult(t, events)
	if run.Status != session.ToolCancelled {
		t.Fatalf("cancelled admission result: %+v", run)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled command executed: %v", err)
	}
	if !runner.HandleSubmit("!echo after-close") || runner.Running() {
		t.Fatal("terminal runner admitted work")
	}
	select {
	case extra := <-events:
		t.Fatalf("unexpected extra execution: %+v", extra)
	default:
	}
}

func TestBashRunnerCloseWaitsForRunningShell(t *testing.T) {
	events, publish := shellEvents()
	runner, err := NewBashRunnerInDir(nil, stubComposer{}, nil, publish, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeShell(t, runner) })
	runner.HandleSubmit("!printf ready; exec sleep 30")
	select {
	case run := <-events:
		if run.Status != session.ToolInProgress || run.Output != "ready" {
			t.Fatalf("expected running shell: %+v", run)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shell did not start")
	}
	closeShell(t, runner)
	if run := awaitShellResult(t, events); run.Status != session.ToolCancelled {
		t.Fatalf("close result: %+v", run)
	}
}

func TestBashRunnerLegacyGetwdFailureDeniesExecution(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Skipf("cannot remove current directory: %v", err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("platform still resolves removed cwd")
	}
	var warning string
	events, publish := shellEvents()
	runner := NewBashRunner(nil, stubComposer{}, func(msg string, _ toast.ToastKind, _ time.Duration) {
		warning = msg
	}, publish)
	if !runner.HandleSubmit("!echo unsafe") || runner.Running() || warning == "" {
		t.Fatal("legacy initialization error did not deny execution")
	}
	closeShell(t, runner)
	select {
	case event := <-events:
		t.Fatalf("failed runner published shell result: %+v", event)
	default:
	}
}
