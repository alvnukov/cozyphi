package bashtool

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestIsLegacyWslBashPath(t *testing.T) {
	for _, p := range []string{
		`C:\Windows\System32\bash.exe`,
		`c:\windows\system32\bash.exe`,
		`C:/Windows/Sysnative/bash.exe`,
	} {
		if !isLegacyWslBashPath(p) {
			t.Errorf("want WSL shim for %q", p)
		}
	}
	for _, p := range []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files\WSL\bash.exe`,
		`/bin/bash`,
		`C:\Windows\System32\wsl.exe`,
	} {
		if isLegacyWslBashPath(p) {
			t.Errorf("not a WSL shim for %q", p)
		}
	}
}

func TestConfigForShell(t *testing.T) {
	cfg := configForShell(`C:\Program Files\Git\bin\bash.exe`)
	if cfg.stdinMode || len(cfg.args) != 1 || cfg.args[0] != "-c" {
		t.Fatalf("git bash config: %+v", cfg)
	}
	cfg = configForShell(`C:\Windows\System32\bash.exe`)
	if !cfg.stdinMode || len(cfg.args) != 1 || cfg.args[0] != "-s" {
		t.Fatalf("WSL shim must use stdin transport: %+v", cfg)
	}
}

func TestResolveShellConfig(t *testing.T) {
	cfg, err := resolveShellConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.shell == "" {
		t.Fatal("empty shell")
	}
	if runtime.GOOS == "windows" && !cfg.stdinMode && (len(cfg.args) != 1 || cfg.args[0] != "-c") {
		t.Fatalf("windows config: %+v", cfg)
	}
}

func TestPrependPathEntry(t *testing.T) {
	got := prependPathEntry([]string{"PATH=/usr/bin:/bin"}, "/x/bin")
	if got[0] != "PATH=/x/bin:/usr/bin:/bin" {
		t.Fatalf("got %q", got[0])
	}

	// Already present → unchanged.
	got = prependPathEntry([]string{"PATH=/usr/bin:/x/bin"}, "/x/bin")
	if len(got) != 1 || got[0] != "PATH=/usr/bin:/x/bin" {
		t.Fatalf("expected unchanged, got %v", got)
	}

	// Windows-style key casing is matched case-insensitively.
	got = prependPathEntry([]string{"Path=C:\\Windows"}, `C:\Phi\bin`)
	if !strings.HasPrefix(got[0], "Path=") || !strings.Contains(got[0], `C:\Phi\bin`) {
		t.Fatalf("got %q", got[0])
	}

	// No PATH entry → appended.
	got = prependPathEntry([]string{"HOME=/home/x"}, "/x/bin")
	if len(got) != 2 || got[1] != "PATH=/x/bin" {
		t.Fatalf("got %v", got)
	}
}

func TestBuildShellCommand(t *testing.T) {
	cmd, err := buildShellCommand(context.Background(), "echo hi")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.SysProcAttr == nil {
		t.Fatal("expected process-group syscall attr")
	}
	if cmd.Cancel == nil {
		t.Fatal("expected tree-kill cancel")
	}
	if cmd.WaitDelay != shellWaitDelay {
		t.Fatalf("WaitDelay=%v, want %v", cmd.WaitDelay, shellWaitDelay)
	}
	if len(cmd.Env) == 0 {
		t.Fatal("expected enriched env")
	}
}
