package proc

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is re-exec'd as a hermetic child for process tests. It
// never runs as part of the normal suite: without PROC_TEST_HELPER=1 it skips.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("PROC_TEST_HELPER") != "1" {
		t.Skip("helper process")
	}
	switch os.Getenv("PROC_TEST_MODE") {
	case "both":
		_, _ = os.Stdout.WriteString("out-line\n")
		_, _ = os.Stderr.WriteString("err-line\n")
	case "stderrbig":
		_, _ = os.Stderr.WriteString(strings.Repeat("e", 128*1024))
	case "echo":
		_, _ = io.Copy(os.Stdout, os.Stdin)
	case "exit7":
		_, _ = os.Stdout.WriteString("before-exit\n")
		exit(7)
	case "sleep":
		time.Sleep(time.Hour)
	case "printenv":
		_, _ = os.Stdout.WriteString(os.Getenv("PROC_TEST_VAR"))
	}
	exit(0)
}

// exit terminates the re-exec'd helper without unwinding test machinery.
func exit(code int) {
	os.Exit(code) //nolint:revive // re-exec'd helper must exit directly
}

func helperSpec(mode string) Spec {
	env := append(os.Environ(), "PROC_TEST_HELPER=1", "PROC_TEST_MODE="+mode)
	return Spec{
		Argv: []string{os.Args[0], "-test.run=TestHelperProcess", "--"},
		Env:  env,
	}
}

func TestRunCombinedOutput(t *testing.T) {
	res, err := Run(t.Context(), helperSpec("both"), Limit{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 || res.Canceled || res.Truncated {
		t.Fatalf("result: %+v", res)
	}
	if !strings.Contains(res.Output, "out-line") || !strings.Contains(res.Output, "err-line") {
		t.Fatalf("combined output=%q", res.Output)
	}
}

func TestRunBoundsOutput(t *testing.T) {
	res, err := Run(t.Context(), helperSpec("stderrbig"), Limit{Bytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated {
		t.Fatal("expected truncation")
	}
	if len(res.Output) > 1024 {
		t.Fatalf("output len=%d, want <= 1024", len(res.Output))
	}
	if strings.Trim(res.Output, "e") != "" {
		t.Fatalf("output not tail of e's: %q", res.Output[:min(len(res.Output), 16)])
	}
}

func TestRunExitCode(t *testing.T) {
	res, err := Run(t.Context(), helperSpec("exit7"), Limit{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 7 {
		t.Fatalf("exit=%d, want 7", res.ExitCode)
	}
	if !strings.Contains(res.Output, "before-exit") {
		t.Fatalf("output=%q", res.Output)
	}
}

func TestRunCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	res, err := Run(ctx, helperSpec("sleep"), Limit{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Canceled {
		t.Fatalf("want canceled, got %+v", res)
	}
}

func TestRunStdinAndEnv(t *testing.T) {
	spec := helperSpec("echo")
	spec.Stdin = "hello-stdin"
	res, err := Run(t.Context(), spec, Limit{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "hello-stdin" {
		t.Fatalf("stdin echo=%q", res.Output)
	}

	spec = helperSpec("printenv")
	spec.Env = append(spec.Env, "PROC_TEST_VAR=env-value")
	res, err = Run(t.Context(), spec, Limit{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "env-value" {
		t.Fatalf("env output=%q", res.Output)
	}
}

func TestRunStreamsChunks(t *testing.T) {
	spec := helperSpec("both")
	var streamed strings.Builder
	spec.Stream = func(chunk string) { streamed.WriteString(chunk) }
	res, err := Run(t.Context(), spec, Limit{})
	if err != nil {
		t.Fatal(err)
	}
	if streamed.String() != res.Output {
		t.Fatalf("streamed=%q, output=%q", streamed.String(), res.Output)
	}
}

func TestRunRejectsEmptyArgv(t *testing.T) {
	if _, err := Run(t.Context(), Spec{}, Limit{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunRejectsInvalidDir(t *testing.T) {
	spec := helperSpec("both")
	spec.Dir = "/definitely/not/a/real/dir"
	if _, err := Run(t.Context(), spec, Limit{}); err == nil {
		t.Fatal("expected error")
	}
	spec.Dir = "relative/dir"
	if _, err := Run(t.Context(), spec, Limit{}); err == nil {
		t.Fatal("expected error for relative dir")
	}
}

func TestRunSplitSeparatesTheStreams(t *testing.T) {
	res, err := RunSplit(t.Context(), helperSpec("both"), Limit{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 || res.Canceled || res.Truncated {
		t.Fatalf("result: %+v", res)
	}
	if res.Output != "out-line\n" {
		t.Fatalf("stdout=%q, want the stdout line alone", res.Output)
	}
	if res.Stderr != "err-line\n" {
		t.Fatalf("stderr=%q, want the stderr line alone", res.Stderr)
	}
}

func TestRunSplitStreamsStdoutOnly(t *testing.T) {
	spec := helperSpec("both")
	var streamed strings.Builder
	spec.Stream = func(chunk string) { streamed.WriteString(chunk) }
	res, err := RunSplit(t.Context(), spec, Limit{})
	if err != nil {
		t.Fatal(err)
	}
	if streamed.String() != res.Output {
		t.Fatalf("streamed=%q, stdout=%q", streamed.String(), res.Output)
	}
	if strings.Contains(streamed.String(), "err-line") {
		t.Fatalf("stderr reached the stream: %q", streamed.String())
	}
}

func TestRunSplitBoundsTheStderrTail(t *testing.T) {
	res, err := RunSplit(t.Context(), helperSpec("stderrbig"), Limit{Bytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "" {
		t.Fatalf("stdout=%q, want empty", res.Output)
	}
	if res.Truncated {
		t.Fatal("Truncated reports on stdout, which stayed empty")
	}
	if len(res.Stderr) != DefaultStderrLimit {
		t.Fatalf("stderr len=%d, want the %d-byte tail", len(res.Stderr), DefaultStderrLimit)
	}
	if strings.Trim(res.Stderr, "e") != "" {
		t.Fatalf("stderr not tail of e's: %q", res.Stderr[:min(len(res.Stderr), 16)])
	}
}

func TestRunSplitReportsAnExitCode(t *testing.T) {
	res, err := RunSplit(t.Context(), helperSpec("exit7"), Limit{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 7 {
		t.Fatalf("exit=%d, want 7", res.ExitCode)
	}
	if res.Output != "before-exit\n" {
		t.Fatalf("stdout=%q", res.Output)
	}
}

func TestRunLeavesStderrEmpty(t *testing.T) {
	res, err := Run(t.Context(), helperSpec("both"), Limit{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stderr != "" {
		t.Fatalf("Stderr=%q, want empty for the combining Run", res.Stderr)
	}
	if !strings.Contains(res.Output, "out-line") || !strings.Contains(res.Output, "err-line") {
		t.Fatalf("combined output=%q", res.Output)
	}
}

func TestStartEchoRoundtrip(t *testing.T) {
	p, err := Start(t.Context(), helperSpec("echo"), DefaultStderrLimit)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(DefaultGrace)

	if _, err := p.Stdin().Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(p.Stdout()).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if line != "ping\n" {
		t.Fatalf("echo=%q", line)
	}
}

func TestStartStderrTailBounded(t *testing.T) {
	p, err := Start(t.Context(), helperSpec("stderrbig"), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !p.StderrTruncated() {
		t.Fatal("expected stderr truncation")
	}
	if tail := p.StderrTail(); len(tail) > 1024 || strings.Trim(tail, "e") != "" {
		t.Fatalf("stderr tail len=%d", len(tail))
	}
}

func TestStartCloseIdempotent(t *testing.T) {
	p, err := Start(t.Context(), helperSpec("sleep"), DefaultStderrLimit)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Close(100 * time.Millisecond); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := p.Close(100 * time.Millisecond); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if err := p.Wait(); err == nil {
		t.Fatal("expected killed wait error")
	}
}

func TestStartLifetimeCancelKills(t *testing.T) {
	lifetime, cancel := context.WithCancel(t.Context())
	p, err := Start(lifetime, helperSpec("sleep"), DefaultStderrLimit)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("lifetime cancel did not reap process")
	}
}

func TestStartSpawnFailure(t *testing.T) {
	if _, err := Start(
		t.Context(),
		Spec{Argv: []string{"/nonexistent/proc-test-binary"}},
		DefaultStderrLimit,
	); err == nil {
		t.Fatal("expected spawn failure")
	}
}
