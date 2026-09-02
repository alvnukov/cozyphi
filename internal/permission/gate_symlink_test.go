package permission

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// symlinkOrSkip links newname to oldname, skipping the test where the platform
// or filesystem refuses symlinks. Several tests here turn on symlink behavior
// and every one of them needs the same escape hatch.
func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

// symlinkFixture builds ws/outside dirs and a symlink ws/link -> outside.
func symlinkFixture(t *testing.T) (ws, outside string) {
	t.Helper()
	ws = t.TempDir()
	outside = t.TempDir()
	symlinkOrSkip(t, outside, filepath.Join(ws, "link"))
	return ws, outside
}

// mustEval resolves a fixture path to its physical form: on macOS t.TempDir
// lives under /var, which the kernel reports as /private/var, so expectations
// that name a physical target must go through EvalSymlinks first.
func mustEval(t *testing.T, p string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestResolveTargetResolvesLeafSymlink(t *testing.T) {
	outside := t.TempDir()
	target := filepath.Join(outside, "file.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	leafLink := filepath.Join(t.TempDir(), "file-link")
	symlinkOrSkip(t, target, leafLink)
	got, err := ResolveTarget(leafLink)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(mustEval(t, outside), "file.txt"); got != want {
		t.Fatalf("ResolveTarget(link) = %q, want %q", got, want)
	}
}

func TestResolveTargetResolvesAncestorSymlinkKeepsMissingTail(t *testing.T) {
	ws, outside := symlinkFixture(t)
	got, err := ResolveTarget(filepath.Join(ws, "link", "new", "leaf.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(mustEval(t, outside), "new", "leaf.txt")
	if got != want {
		t.Fatalf("ResolveTarget = %q, want %q", got, want)
	}
}

func TestResolveTargetPhysicalPathUnchanged(t *testing.T) {
	base := mustEval(t, t.TempDir())
	p := filepath.Join(base, "a", "b.txt")
	got, err := ResolveTarget(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != p {
		t.Fatalf("ResolveTarget(%q) = %q, want identity", p, got)
	}
}

func TestResolveTargetRejectsRelativePath(t *testing.T) {
	if _, err := ResolveTarget("relative/path.txt"); err == nil {
		t.Fatal("relative path must be rejected")
	}
}

func TestWithinWorkspaceResolvedSymlinkEscape(t *testing.T) {
	ws, _ := symlinkFixture(t)
	if WithinWorkspaceResolved(filepath.Join(ws, "link"), ws) {
		t.Fatal("symlink out of workspace must not count as inside")
	}
	if !WithinWorkspaceResolved(filepath.Join(ws, "missing", "leaf.txt"), ws) {
		t.Fatal("missing leaf inside workspace must stay inside")
	}
}

func TestWithinWorkspaceResolvedSymlinkedWorkspace(t *testing.T) {
	real := t.TempDir()
	parent := t.TempDir()
	wsLink := filepath.Join(parent, "ws-link")
	symlinkOrSkip(t, real, wsLink)
	if !WithinWorkspaceResolved(filepath.Join(real, "f.txt"), wsLink) {
		t.Fatal("path under the real workspace dir must be inside the symlinked workspace")
	}
}

func TestCheckWriteDeniesLeafSymlinkEscape(t *testing.T) {
	ws, outside := symlinkFixture(t)
	g, err := NewGate(DefaultPolicy(), ws)
	if err != nil {
		t.Fatal(err)
	}
	dec, reason := g.Check(t.Context(), Request{
		Action: ActionWrite,
		Tool:   "write",
		Paths:  []string{filepath.Join(ws, "link", "stolen.txt")},
	})
	if dec != Deny {
		t.Fatalf("want Deny, got %v (%s)", dec, reason)
	}
	if !strings.Contains(reason, mustEval(t, outside)) {
		t.Fatalf("reason should name the physical target, got %q", reason)
	}
}

func TestCheckWriteDeniesAncestorSymlinkEscapeMissingLeaf(t *testing.T) {
	ws, _ := symlinkFixture(t)
	g, err := NewGate(DefaultPolicy(), ws)
	if err != nil {
		t.Fatal(err)
	}
	dec, reason := g.Check(t.Context(), Request{
		Action: ActionWrite,
		Tool:   "write",
		Paths:  []string{filepath.Join(ws, "link", "sub", "new.txt")},
	})
	if dec != Deny {
		t.Fatalf("want Deny, got %v (%s)", dec, reason)
	}
}

func TestCheckReadDeniesSymlinkToSensitive(t *testing.T) {
	ws, outside := symlinkFixture(t)
	key := filepath.Join(outside, "key.pem")
	if err := os.WriteFile(key, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := DefaultPolicy()
	p.SensitivePathDeny = []string{outside}
	g, err := NewGate(p, ws)
	if err != nil {
		t.Fatal(err)
	}
	dec, reason := g.Check(t.Context(), Request{
		Action: ActionRead,
		Tool:   "read",
		Paths:  []string{filepath.Join(ws, "link", "key.pem")},
	})
	if dec != Deny {
		t.Fatalf("want Deny, got %v (%s)", dec, reason)
	}
}

func TestCheckWriteAllowsSymlinkInsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	sub := filepath.Join(ws, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, sub, filepath.Join(ws, "alias"))
	g, err := NewGate(DefaultPolicy(), ws)
	if err != nil {
		t.Fatal(err)
	}
	dec, reason := g.Check(t.Context(), Request{
		Action: ActionWrite,
		Tool:   "write",
		Paths:  []string{filepath.Join(ws, "alias", "f.txt")},
	})
	if dec != Allow {
		t.Fatalf("alias inside workspace must stay allowed, got %v (%s)", dec, reason)
	}
}

func TestNewGateSymlinkedWorkspaceContainsRealPaths(t *testing.T) {
	real := t.TempDir()
	parent := t.TempDir()
	wsLink := filepath.Join(parent, "ws-link")
	symlinkOrSkip(t, real, wsLink)
	g, err := NewGate(DefaultPolicy(), wsLink)
	if err != nil {
		t.Fatal(err)
	}
	dec, _ := g.Check(t.Context(), Request{
		Action: ActionWrite,
		Tool:   "write",
		Paths:  []string{filepath.Join(real, "f.txt")},
	})
	if dec != Allow {
		t.Fatal("physical path under the symlinked workspace must stay allowed")
	}
}
