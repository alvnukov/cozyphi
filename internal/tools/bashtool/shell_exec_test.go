package bashtool

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecShellEcho(t *testing.T) {
	res, err := ExecShell(context.Background(), "echo hello", ShellExecOptions{})
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

func TestExecShellCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
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
	res, err := ExecShell(context.Background(), "exit 7", ShellExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 7 {
		t.Fatalf("exit=%d", res.ExitCode)
	}
}
