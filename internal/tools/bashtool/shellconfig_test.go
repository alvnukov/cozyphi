package bashtool

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
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
	sep := string(os.PathListSeparator)
	got := prependPathEntry([]string{"PATH=/usr/bin" + sep + "/bin"}, "/x/bin")
	want := "PATH=/x/bin" + sep + "/usr/bin" + sep + "/bin"
	if got[0] != want {
		t.Fatalf("got %q, want %q", got[0], want)
	}

	// Already present → unchanged.
	existing := "PATH=/usr/bin" + sep + "/x/bin"
	got = prependPathEntry([]string{existing}, "/x/bin")
	if len(got) != 1 || got[0] != existing {
		t.Fatalf("expected unchanged, got %v", got)
	}

	// Windows-style key casing is matched case-insensitively.
	got = prependPathEntry([]string{"Path=C:\\Windows"}, `C:\CozyPhi\bin`)
	if !strings.HasPrefix(got[0], "Path=") || !strings.Contains(got[0], `C:\CozyPhi\bin`) {
		t.Fatalf("got %q", got[0])
	}

	// No PATH entry → appended.
	got = prependPathEntry([]string{"HOME=/home/x"}, "/x/bin")
	if len(got) != 2 || got[1] != "PATH=/x/bin" {
		t.Fatalf("got %v", got)
	}
}

func TestBuildShellSpec(t *testing.T) {
	spec, err := buildShellSpec(t.Context(), "echo hi")
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Argv) < 3 || spec.Argv[len(spec.Argv)-1] != "echo hi" {
		t.Fatalf("argv=%v", spec.Argv)
	}
	if spec.Stdin != "" {
		t.Fatalf("unexpected stdin transport: %q", spec.Stdin)
	}
	if len(spec.Env) == 0 {
		t.Fatal("expected enriched env")
	}
}

func TestBuildShellSpecUsesContextCwd(t *testing.T) {
	dir := t.TempDir()
	spec, err := buildShellSpec(tooldef.WithCwd(t.Context(), dir), "echo hi")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Dir != dir {
		t.Fatalf("Dir=%q, want %q", spec.Dir, dir)
	}
}

func TestShellSpecStdinMode(t *testing.T) {
	spec := shellSpec(shellConfig{shell: "/bin/bash", args: []string{"-s"}, stdinMode: true}, "", "echo hi")
	if spec.Stdin != "echo hi" {
		t.Fatalf("Stdin=%q", spec.Stdin)
	}
	if len(spec.Argv) != 2 || spec.Argv[0] != "/bin/bash" || spec.Argv[1] != "-s" {
		t.Fatalf("argv=%v", spec.Argv)
	}
}

func TestShellSpecCommandMode(t *testing.T) {
	spec := shellSpec(shellConfig{shell: "/bin/bash", args: []string{"-c"}}, "", "echo hi")
	if spec.Stdin != "" {
		t.Fatalf("Stdin=%q", spec.Stdin)
	}
	if len(spec.Argv) != 3 || spec.Argv[0] != "/bin/bash" || spec.Argv[1] != "-c" || spec.Argv[2] != "echo hi" {
		t.Fatalf("argv=%v", spec.Argv)
	}
}
