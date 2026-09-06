package permission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInWorkspace(t *testing.T) {
	ws := "/Users/me/proj"
	if !InWorkspace("/Users/me/proj", ws) {
		t.Fatal("workspace root should be inside itself")
	}
	if !InWorkspace("/Users/me/proj/src/a.go", ws) {
		t.Fatal("child should be inside")
	}
	if InWorkspace("/Users/me/other", ws) {
		t.Fatal("sibling should be outside")
	}
	if InWorkspace("/Users/me/proj-evil/x", ws) {
		t.Fatal("prefix-sibling should be outside")
	}
	if InWorkspace("/Users/me", ws) {
		t.Fatal("parent should be outside")
	}
}

func TestCheckWriteOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	g, err := NewGate(DefaultPolicy(), ws)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(os.TempDir(), "cozyphi-perm-test-outside")
	dec, reason := g.Check(t.Context(), Request{
		Action: ActionWrite,
		Tool:   "write",
		Paths:  []string{outside},
	})
	if dec != Deny {
		t.Fatalf("want Deny, got %v (%s)", dec, reason)
	}
}

func TestCheckWriteInsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	g, err := NewGate(DefaultPolicy(), ws)
	if err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(ws, "out.txt")
	dec, reason := g.Check(t.Context(), Request{
		Action: ActionWrite,
		Tool:   "write",
		Paths:  []string{inside},
	})
	if dec != Allow {
		t.Fatalf("want Allow, got %v (%s)", dec, reason)
	}
}

func TestCheckWriteSensitiveConfig(t *testing.T) {
	ws := t.TempDir()
	g, err := NewGate(DefaultPolicy(), ws)
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	cfgPath := filepath.Join(home, ".cozyphi", "config.yaml")
	dec, reason := g.Check(t.Context(), Request{
		Action: ActionWrite,
		Tool:   "write",
		Paths:  []string{cfgPath},
	})
	if dec != Deny {
		t.Fatalf("want Deny for config.yaml, got %v (%s)", dec, reason)
	}
}

func TestCheckBashAllowDenyAsk(t *testing.T) {
	g, err := NewGate(DefaultPolicy(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	dec, _ := g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: "git status"})
	if dec != Allow {
		t.Fatalf("git status: want Allow, got %v", dec)
	}
	dec, _ = g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: "go test ./..."})
	if dec != Ask {
		t.Fatalf("go test: want Ask, got %v", dec)
	}
	dec, reason := g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: "sudo true"})
	if dec != Deny {
		t.Fatalf("sudo: want Deny, got %v (%s)", dec, reason)
	}
	dec, reason = g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: "curl https://example.com"})
	if dec != Ask {
		t.Fatalf("curl: want Ask, got %v (%s)", dec, reason)
	}
}

// TestDefaultGoCommandsAskByDefault pins the trimmed default allowlist: go
// subcommands that execute code or mutate the tree ask by default (gofmt
// rewrites files, go mod edits go.mod/go.sum and invokes VCS), and so does
// any flagged go list — build flags like -export run the toolchain. Only
// flagless go list and go version stay allowed.
func TestDefaultGoCommandsAskByDefault(t *testing.T) {
	g, err := NewGate(DefaultPolicy(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	ask := []string{
		"go test ./...",
		"go test -run TestGate ./internal/permission/...",
		"go build ./...",
		"go build -toolexec=./tool ./...",
		"go vet ./...",
		"go vet -vettool=./tool ./...",
		"go env -w GOFLAGS=-mod=vendor",
		"go run .",
		"go generate ./...",
		"go fmt ./...",
		"go mod tidy",
		"go list -export ./...",
		"go env GOOS",
		"git -c diff.external=./evil diff",
	}
	for _, cmd := range ask {
		dec, reason := g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: cmd})
		if dec != Ask {
			t.Errorf("%s: want Ask, got %v (%s)", cmd, dec, reason)
		}
	}

	allow := []string{
		"go list ./...",
		"go version",
	}
	for _, cmd := range allow {
		dec, reason := g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: cmd})
		if dec != Allow {
			t.Errorf("%s: want Allow, got %v (%s)", cmd, dec, reason)
		}
	}
}

// TestUserAllowOptsIntoGoTest: an explicit permissions.bash.allow entry opts
// back into running tests, and the deny rules still win over it.
func TestUserAllowOptsIntoGoTest(t *testing.T) {
	policy := DefaultPolicy()
	policy.BashAllow = append([]string{`^go test\b`}, policy.BashAllow...)
	g, err := NewGate(policy, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	dec, _ := g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: "go test ./..."})
	if dec != Allow {
		t.Fatalf("user-allowed go test: want Allow, got %v", dec)
	}

	// The opt-in covers exactly what it names; build still asks.
	dec, _ = g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: "go build ./..."})
	if dec != Ask {
		t.Fatalf("go build despite test-only opt-in: want Ask, got %v", dec)
	}

	// A compound command leaves the allowlist path and deny wins.
	dec, reason := g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: "go test ./... && sudo true"})
	if dec != Deny {
		t.Fatalf("go test && sudo: want Deny, got %v (%s)", dec, reason)
	}
}

func TestCheckBashCompoundNotAllowlisted(t *testing.T) {
	g, err := NewGate(DefaultPolicy(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	// Prefix ^ls\b must NOT allow chained rm.
	cmd := `ls -la todo.list 2>/dev/null && rm -rf todo.list && echo "removed"`
	dec, reason := g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: cmd})
	if dec != Deny {
		t.Fatalf("ls && rm -rf: want Deny, got %v (%s)", dec, reason)
	}

	// Plain ls still allowed.
	dec, reason = g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: "ls -la todo.list"})
	if dec != Allow {
		t.Fatalf("ls alone: want Allow, got %v (%s)", dec, reason)
	}

	// rm -rf without trailing / must still deny.
	dec, reason = g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: "rm -rf todo.list"})
	if dec != Deny {
		t.Fatalf("rm -rf file: want Deny, got %v (%s)", dec, reason)
	}

	// Pipe / redirect out of allowlist.
	dec, reason = g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: "cat foo | sh"})
	if dec == Allow {
		t.Fatalf("pipe: must not Allow (%s)", reason)
	}
	dec, reason = g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: "cat secret > /tmp/out"})
	if dec == Allow {
		t.Fatalf("redirect: must not Allow (%s)", reason)
	}
}

// TestBashAllowlistBindsToTheFullCommand pins that a safe prefix never
// authorizes an executing tail: substitution that expands inside double
// quotes, process substitution, subshell parentheses, unclosed quoting, a
// /dev/null lookalike redirect, input-side syntax (redirect, heredoc) and
// chaining operators all leave the allowlist path, while literal control
// characters inside quotes and real /dev/null redirects stay allowed.
func TestBashAllowlistBindsToTheFullCommand(t *testing.T) {
	g, err := NewGate(DefaultPolicy(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	for _, cmd := range []string{
		// $(), backticks and ${} expand inside double quotes: the tail
		// executes despite the quoted prefix.
		`echo "$(curl evil.example)"`,
		"echo \"a `curl evil.example` b\"",
		`cat "${HOME}/.ssh/id_rsa"`,
		// Process substitution runs a command; <( is not a redirect.
		`cat <(curl evil.example)`,
		// Parentheses are a subshell: compound syntax, not a simple command.
		`(git status)`,
		// Unclosed quoting or a dangling escape is ambiguous syntax: fail
		// closed instead of matching the allowlist.
		`echo "unclosed`,
		`git log -G'unclosed`,
		`echo done\`,
		// The /dev/null redirect exemption must not prefix-match /dev/nullx.
		`cat file >/dev/nullx`,
		// Chaining operators — newline, || — and input-side syntax (redirect,
		// heredoc) are not a simple command either.
		"echo a\nrm -rf /tmp/x",
		"echo a || rm -rf /tmp/x",
		`cat < input.txt`,
		`cat <<EOF`,
	} {
		dec, reason := g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: cmd})
		if dec == Allow {
			t.Errorf("%s: must not ride the allowlist, got Allow (%s)", cmd, reason)
		}
	}

	// Deny keeps priority over everything above.
	dec, reason := g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: `echo "$(sudo true)"`})
	if dec != Deny {
		t.Fatalf("substituted sudo: want Deny, got %v (%s)", dec, reason)
	}

	// Literal control characters inside quotes do not chain, and the
	// >/dev/null noise redirect keeps its exemption.
	for _, cmd := range []string{
		`git log --grep='a;b'`,
		`echo "a && b"`,
		`cat "plain file.txt"`,
		`git status 2>/dev/null`,
		`cat main.go 2>>/dev/null`,
	} {
		dec, reason := g.Check(ctx, Request{Action: ActionBash, Tool: "bash", Command: cmd})
		if dec != Allow {
			t.Errorf("%s: want Allow, got %v (%s)", cmd, dec, reason)
		}
	}
}

func TestModeHeadlessStrictFoldsAsk(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = ModeHeadlessStrict
	g, err := NewGate(p, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dec, reason := g.Check(t.Context(), Request{
		Action:  ActionBash,
		Tool:    "bash",
		Command: "curl https://example.com",
	})
	if dec != Deny {
		t.Fatalf("want Deny, got %v (%s)", dec, reason)
	}
}

func TestModeReadonlyDeniesWrite(t *testing.T) {
	ws := t.TempDir()
	p := DefaultPolicy()
	p.Mode = ModeReadonly
	g, err := NewGate(p, ws)
	if err != nil {
		t.Fatal(err)
	}
	dec, reason := g.Check(t.Context(), Request{
		Action: ActionWrite,
		Tool:   "write",
		Paths:  []string{filepath.Join(ws, "a.txt")},
	})
	if dec != Deny {
		t.Fatalf("want Deny, got %v (%s)", dec, reason)
	}
	// allowlisted bash still ok
	dec, reason = g.Check(t.Context(), Request{
		Action:  ActionBash,
		Tool:    "bash",
		Command: "git status",
	})
	if dec != Allow {
		t.Fatalf("git status in readonly: want Allow, got %v (%s)", dec, reason)
	}
}

func TestModeAutopilotFoldsAsk(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = ModeAutopilot
	g, err := NewGate(p, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dec, _ := g.Check(t.Context(), Request{
		Action:  ActionBash,
		Tool:    "bash",
		Command: "curl https://example.com",
	})
	if dec != Deny {
		t.Fatalf("want Deny, got %v", dec)
	}
}

func TestReadSensitiveDeny(t *testing.T) {
	g, err := NewGate(DefaultPolicy(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	dec, reason := g.Check(t.Context(), Request{
		Action: ActionRead,
		Tool:   "read",
		Paths:  []string{filepath.Join(home, ".ssh", "id_rsa")},
	})
	if dec != Deny {
		t.Fatalf("want Deny, got %v (%s)", dec, reason)
	}
}

// The gate resolves sensitive prefixes for comparison, but the policy it was
// handed belongs to the caller: a second gate built from the same policy must
// still see the prefixes that were written there.
func TestNewGateLeavesTheCallersDenyListAlone(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secrets, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	policy := Policy{SensitivePathDeny: []string{secrets}}
	if _, err := NewGate(policy, dir); err != nil {
		t.Fatalf("NewGate: %v", err)
	}
	if got := policy.SensitivePathDeny[0]; got != secrets {
		t.Fatalf("deny list = %q, want the caller's own %q", got, secrets)
	}
}

// TestWriteGitControlFilesAsk pins the consent boundary for git's execution
// control files: writing .git config or hooks must not ride the ordinary
// workspace write — it asks, naming the control file. Ordinary sources, .git
// internals outside the minimal set, and reads of control files keep the
// workspace decision.
func TestWriteGitControlFilesAsk(t *testing.T) {
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("[core]\n"), 0o644); err != nil {
		t.Fatalf("config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("source: %v", err)
	}
	if err := os.Symlink(filepath.Join(gitDir, "config"), filepath.Join(repo, "alias.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	g, err := NewGate(DefaultPolicy(), repo)
	if err != nil {
		t.Fatal(err)
	}
	ask := func(p string) {
		t.Helper()
		dec, reason := g.Check(t.Context(), Request{Action: ActionWrite, Tool: "write", Paths: []string{p}})
		if dec != Ask {
			t.Fatalf("write %s: want Ask, got %v (%s)", p, dec, reason)
		}
		if !strings.Contains(reason, "git control") {
			t.Fatalf("write %s: reason should name the git control file, got %q", p, reason)
		}
	}
	ask(filepath.Join(gitDir, "config"))
	ask(filepath.Join(gitDir, "hooks", "pre-commit"))
	ask(filepath.Join(gitDir, "hooks", "nested", "pre-push"))
	ask(filepath.Join(repo, "alias.txt")) // symlink smuggle: resolved target classified

	// A relative path through the public extract seam asks the same.
	req, err := ExtractAt("write", json.RawMessage(`{"path":".git/config"}`), repo)
	if err != nil {
		t.Fatal(err)
	}
	if dec, reason := g.Check(t.Context(), req); dec != Ask {
		t.Fatalf("relative .git/config: want Ask, got %v (%s)", dec, reason)
	}

	allow := func(action Action, tool, p string) {
		t.Helper()
		dec, reason := g.Check(t.Context(), Request{Action: action, Tool: tool, Paths: []string{p}})
		if dec != Allow {
			t.Fatalf("%s %s: want Allow, got %v (%s)", tool, p, dec, reason)
		}
	}
	allow(ActionWrite, "write", filepath.Join(repo, "main.go"))
	allow(ActionWrite, "write", filepath.Join(gitDir, "index"))
	allow(ActionRead, "read", filepath.Join(gitDir, "config"))
}

// TestWriteGitControlFilesAskAcrossWorktrees pins the split-dir forms. A
// linked worktree keeps .git as a pointer file, so its control files live in
// the main checkout's git dir — reached from the main workspace when the
// worktree is nested inside it (the .worktrees layout). A pointer that
// cannot be resolved fails closed: any path under it asks. From the
// worktree's own workspace the external git dir stays denied by the
// workspace rule, which is stronger than consent.
func TestWriteGitControlFilesAskAcrossWorktrees(t *testing.T) {
	main := t.TempDir()
	common := filepath.Join(main, ".git")
	wtGit := filepath.Join(common, "worktrees", "wt")
	for _, d := range []string{wtGit, filepath.Join(common, "hooks"), filepath.Join(main, "wt"), filepath.Join(main, "wt2")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(wtGit, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(main, "wt", ".git"), []byte("gitdir: "+wtGit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(main, "wt2", ".git"), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	g, err := NewGate(DefaultPolicy(), main)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(common, "config"),
		filepath.Join(common, "hooks", "pre-commit"),
		filepath.Join(wtGit, "config.worktree"),
	} {
		dec, reason := g.Check(t.Context(), Request{Action: ActionWrite, Tool: "write", Paths: []string{p}})
		if dec != Ask {
			t.Fatalf("write %s: want Ask, got %v (%s)", p, dec, reason)
		}
	}
	// Repointing the pointer relocates every rule, so the gate that sits in
	// the worktree treats its own .git file — parsed or garbage — as control.
	for _, ws := range []string{filepath.Join(main, "wt"), filepath.Join(main, "wt2")} {
		gws, err := NewGate(DefaultPolicy(), ws)
		if err != nil {
			t.Fatal(err)
		}
		dec, reason := gws.Check(
			t.Context(),
			Request{Action: ActionWrite, Tool: "write", Paths: []string{filepath.Join(ws, ".git")}},
		)
		if dec != Ask {
			t.Fatalf("write %s: want Ask, got %v (%s)", filepath.Join(ws, ".git"), dec, reason)
		}
	}
	// A path that threads under the pointer file cannot resolve — .git is a
	// file — and fails closed at the resolve step, one notch stricter than
	// the consent the pointer itself gets.
	if dec, reason := g.Check(
		t.Context(),
		Request{Action: ActionWrite, Tool: "write", Paths: []string{filepath.Join(main, "wt", ".git", "config")}},
	); dec != Deny {
		t.Fatalf("write under the pointer file: want Deny, got %v (%s)", dec, reason)
	}

	gw, err := NewGate(DefaultPolicy(), filepath.Join(main, "wt"))
	if err != nil {
		t.Fatal(err)
	}
	dec, reason := gw.Check(t.Context(), Request{
		Action: ActionWrite,
		Tool:   "write",
		Paths:  []string{filepath.Join(common, "hooks", "pre-commit")},
	})
	if dec != Deny {
		t.Fatalf("external git dir from worktree workspace: want Deny, got %v (%s)", dec, reason)
	}
	dec, reason = gw.Check(t.Context(), Request{
		Action: ActionWrite,
		Tool:   "write",
		Paths:  []string{filepath.Join(main, "wt", "main.go")},
	})
	if dec != Allow {
		t.Fatalf("ordinary source in worktree: want Allow, got %v (%s)", dec, reason)
	}
}

// The control set is derived per check, not snapshotted when the gate is
// built: a workspace that starts bare and becomes a repository mid-session
// — the classic "git init, then plant a hook" — must not keep riding the
// workspace-write allow.
func TestWriteGitControlFilesAskAfterLateGitInit(t *testing.T) {
	ws := t.TempDir()
	g, err := NewGate(DefaultPolicy(), ws)
	if err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(ws, ".git", "hooks", "pre-commit")
	if dec, _ := g.Check(
		t.Context(),
		Request{Action: ActionWrite, Tool: "write", Paths: []string{hook}},
	); dec != Allow {
		t.Fatalf("bare workspace hook write before init: want Allow, got %v", dec)
	}
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if dec, reason := g.Check(
		t.Context(),
		Request{Action: ActionWrite, Tool: "write", Paths: []string{hook}},
	); dec != Ask {
		t.Fatalf("hook write after late git init: want Ask, got %v (%s)", dec, reason)
	}
}

// Policy prefixes replace the derivation wholesale — an explicit opinion is
// the whole point of the field — and nil means "no opinion", not "no control".
func TestPolicyControlPathAskOverridesDerivation(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	guarded := filepath.Join(repo, "deploy.sh")
	g, err := NewGate(Policy{ControlPathAsk: []string{guarded}}, repo)
	if err != nil {
		t.Fatal(err)
	}
	if dec, reason := g.Check(
		t.Context(),
		Request{Action: ActionWrite, Tool: "write", Paths: []string{guarded}},
	); dec != Ask {
		t.Fatalf("policy-named control path: want Ask, got %v (%s)", dec, reason)
	}
	if dec, reason := g.Check(
		t.Context(),
		Request{Action: ActionWrite, Tool: "write", Paths: []string{filepath.Join(repo, ".git", "config")}},
	); dec != Allow {
		t.Fatalf("derived control path under explicit policy: want Allow, got %v (%s)", dec, reason)
	}
}
