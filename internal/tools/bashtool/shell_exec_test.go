package bashtool

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExecShellEcho(t *testing.T) {
	res, err := ExecShell(t.Context(), "echo hello", ShellExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Canceled || res.ExitCode != 0 {
		t.Fatalf("result: %+v", res)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Fatalf("output=%q", res.Output)
	}
}

func TestExecShellCapturesBothStreams(t *testing.T) {
	res, err := ExecShell(t.Context(), "printf stdout; printf stderr >&2", ShellExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 || res.Canceled {
		t.Fatalf("result: %+v", res)
	}
	if !strings.Contains(res.Output, "stdout") || !strings.Contains(res.Output, "stderr") {
		t.Fatalf("combined output=%q", res.Output)
	}
}

func TestExecShellCapturesOutputBeforeProcessExit(t *testing.T) {
	const outputSize = 32 * 1024
	const command = "printf '%*s' 32768 '' | tr ' ' x"

	for range 8 {
		res, err := ExecShell(t.Context(), command, ShellExecOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if res.ExitCode != 0 || res.Canceled {
			t.Fatalf("result: %+v", res)
		}
		if len(res.Output) != outputSize || strings.Trim(res.Output, "x") != "" {
			t.Fatalf("captured %d bytes, want %d x bytes", len(res.Output), outputSize)
		}
	}
}

func TestExecShellKeepsOutputTail(t *testing.T) {
	// Regression: output still buffered in the kernel pipe at process exit
	// must not be dropped (writer-mode copying drains to EOF before Run returns).
	command := "seq 1 100000" // ~590KB, under the collection cap
	res, err := ExecShell(t.Context(), command, ShellExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cleanupBashOutputFile(t, res.Output)
	if res.ExitCode != 0 || res.Canceled {
		t.Fatalf("result: %+v", res)
	}
	// The display notice's own line range proves line 100000 was collected.
	if !strings.Contains(res.Output, "Showing lines 99001-100000 of 100000") {
		t.Fatalf("output tail lost, ends with %q", tailLines(res.Output))
	}
	if !strings.Contains(res.Output, "Full output:") || strings.Contains(res.Output, "Retained output:") {
		t.Fatalf("under-cap output mislabeled, ends with %q", tailLines(res.Output))
	}
	if strings.Contains(res.Output, "[output truncated:") {
		t.Fatalf("unexpected collection truncation, ends with %q", tailLines(res.Output))
	}
}

func TestExecShellBoundsCollection(t *testing.T) {
	// Runaway output must not be buffered unboundedly: the newest
	// BashMaxCollectBytes are kept and the collection truncation is reported.
	command := "yes x | head -c 20971520" // 20MB
	res, err := ExecShell(t.Context(), command, ShellExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cleanupBashOutputFile(t, res.Output)
	if res.ExitCode != 0 || res.Canceled {
		t.Fatalf("result: %+v", res)
	}
	if !strings.Contains(res.Output, "[output truncated: only the last 8 MB was kept]") {
		t.Fatalf("want collection truncation notice, ends with %q", tailLines(res.Output))
	}
	if !strings.Contains(res.Output, "Retained output:") || strings.Contains(res.Output, "Full output:") {
		t.Fatalf("collection-truncated output mislabeled, ends with %q", tailLines(res.Output))
	}
}

func cleanupBashOutputFile(t *testing.T, output string) {
	t.Helper()
	for _, marker := range []string{"Full output: ", "Retained output: "} {
		_, rest, found := strings.Cut(output, marker)
		if !found {
			continue
		}
		path := strings.TrimSpace(strings.Split(rest, "]")[0])
		if path == "" {
			return
		}
		t.Cleanup(func() { _ = os.Remove(path) })
		return
	}
}

func tailLines(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return strings.Join(lines, "\n")
}

func TestExecShellCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	res, err := ExecShell(ctx, "sleep 5", ShellExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Canceled {
		t.Fatalf("want canceled, got %+v", res)
	}
}

func TestExecShellExitCode(t *testing.T) {
	res, err := ExecShell(t.Context(), "exit 7", ShellExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 7 {
		t.Fatalf("exit=%d", res.ExitCode)
	}
}
