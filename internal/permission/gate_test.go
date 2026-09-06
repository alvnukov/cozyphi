package permission

import (
	"os"
	"path/filepath"
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
