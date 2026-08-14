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
	outside := filepath.Join(os.TempDir(), "phi-perm-test-outside")
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
	cfgPath := filepath.Join(home, ".phi", "config.yaml")
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
	if dec != Allow {
		t.Fatalf("go test: want Allow, got %v", dec)
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

func TestCheckFetchHostAllowlist(t *testing.T) {
	p := DefaultPolicy()
	p.FetchAllowedHosts = []string{"docs.github.com"}
	g, err := NewGate(p, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	dec, _ := g.Check(ctx, Request{Action: ActionFetch, Tool: "fetch", URL: "https://docs.github.com/en"})
	if dec != Allow {
		t.Fatalf("allowed host: want Allow, got %v", dec)
	}
	dec, _ = g.Check(ctx, Request{Action: ActionFetch, Tool: "fetch", URL: "https://evil.example"})
	if dec != Ask {
		t.Fatalf("other host: want Ask, got %v", dec)
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
		Action: ActionFetch,
		Tool:   "fetch",
		URL:    "https://example.com",
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

func TestHostOfURL(t *testing.T) {
	cases := map[string]string{
		"https://docs.github.com/en":   "docs.github.com",
		"http://USER:pw@Ex.com:8080/x": "ex.com",
		"docs.github.com/path":         "docs.github.com",
	}
	for in, want := range cases {
		if got := HostOfURL(in); got != want {
			t.Errorf("HostOfURL(%q)=%q want %q", in, got, want)
		}
	}
}
