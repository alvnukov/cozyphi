package permission

import (
	"path/filepath"
	"strings"
	"testing"
)

// memoryGate builds a gate whose memory directory sits outside the workspace,
// the way the real layout does (~/.cozyphi/memory/… beside a project anywhere).
func memoryGate(t *testing.T) (*StaticGate, string) {
	t.Helper()
	ws := t.TempDir()
	memDir := filepath.Join(t.TempDir(), "memory", "--proj--")
	policy := DefaultPolicy()
	policy.WorkspaceOnlyReads = true
	policy.MemoryDir = memDir
	g, err := NewGate(policy, ws)
	if err != nil {
		t.Fatal(err)
	}
	return g, memDir
}

func TestMemoryDirIsWritableOutsideTheWorkspace(t *testing.T) {
	g, memDir := memoryGate(t)

	for _, action := range []Action{ActionWrite, ActionEdit} {
		dec, reason := g.Check(t.Context(), Request{
			Action: action,
			Tool:   string(action),
			Paths:  []string{filepath.Join(memDir, "who-the-user-is.md")},
		})
		if dec != Allow {
			t.Fatalf("%s: want Allow, got %v (%s)", action, dec, reason)
		}
	}

	dec, reason := g.Check(t.Context(), Request{
		Action: ActionRead,
		Tool:   "read",
		Paths:  []string{filepath.Join(memDir, "who-the-user-is.md")},
	})
	if dec != Allow {
		t.Fatalf("read: want Allow, got %v (%s)", dec, reason)
	}
}

func TestMemoryExemptionIsNarrow(t *testing.T) {
	g, memDir := memoryGate(t)

	// A sibling of the memory directory is still outside the workspace.
	dec, reason := g.Check(t.Context(), Request{
		Action: ActionWrite,
		Tool:   "write",
		Paths:  []string{filepath.Join(filepath.Dir(memDir), "--other--", "x.md")},
	})
	if dec != Deny {
		t.Fatalf("want Deny for a sibling project's memory, got %v (%s)", dec, reason)
	}

	// A batch is judged as a whole: one bad path denies it.
	dec, reason = g.Check(t.Context(), Request{
		Action: ActionWrite,
		Tool:   "write",
		Paths:  []string{filepath.Join(memDir, "ok.md"), filepath.Join(filepath.Dir(memDir), "escape.md")},
	})
	if dec != Deny {
		t.Fatalf("want Deny when one path escapes, got %v (%s)", dec, reason)
	}
}

func TestNoMemoryDirKeepsTheWorkspaceRule(t *testing.T) {
	ws := t.TempDir()
	memDir := filepath.Join(t.TempDir(), "memory", "--proj--")
	g, err := NewGate(DefaultPolicy(), ws) // no MemoryDir: what a sub-agent gets
	if err != nil {
		t.Fatal(err)
	}
	dec, reason := g.Check(t.Context(), Request{
		Action: ActionWrite,
		Tool:   "write",
		Paths:  []string{filepath.Join(memDir, "note.md")},
	})
	if dec != Deny {
		t.Fatalf("want Deny without MemoryDir, got %v (%s)", dec, reason)
	}
}

// TestMemoryToolIsGatedOnHavingAMemoryDirectory pins the one decision the gate
// can make about the memory tool: it carries no path — a memory is addressed
// by name — so what is checked is whether this session has a memory directory
// at all. A sub-agent has none, and gets none of the tool.
func TestMemoryToolIsGatedOnHavingAMemoryDirectory(t *testing.T) {
	g, _ := memoryGate(t)

	req, err := ExtractAt("memory", []byte(`{"action":"forget","name":"release-freeze"}`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if req.Action != ActionMemory {
		t.Fatalf("want action %q, got %q", ActionMemory, req.Action)
	}
	if dec, reason := g.Check(t.Context(), req); dec != Allow {
		t.Fatalf("want Allow with a memory directory, got %v (%s)", dec, reason)
	}

	policy := DefaultPolicy()
	child, err := NewGate(policy, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dec, reason := child.Check(t.Context(), req)
	if dec != Deny {
		t.Fatalf("want Deny without a memory directory, got %v (%s)", dec, reason)
	}
	if !strings.Contains(reason, "no memory directory") {
		t.Fatalf("reason should say why: %q", reason)
	}
}
