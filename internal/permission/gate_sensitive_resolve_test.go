package permission

import (
	"os"
	"path/filepath"
	"testing"
)

// The password file must never be touched, not even for metadata: a deny
// entry outside the home directory is resolved through its parent and the
// leaf is appended unchanged, while an entry under home still follows a
// symlinked leaf so a relocated ~/.ssh stays covered.
func TestSensitivePrefixOutsideHomeKeepsItsLeafUnresolved(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	system := filepath.Join(root, "etc")
	home := filepath.Join(root, "home")
	for _, dir := range []string{system, home, filepath.Join(root, "real-shadow"), filepath.Join(root, "real-ssh")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	shadow := filepath.Join(system, "shadow")
	if err := os.Symlink(filepath.Join(root, "real-shadow"), shadow); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	ssh := filepath.Join(home, ".ssh")
	if err := os.Symlink(filepath.Join(root, "real-ssh"), ssh); err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := ResolveTarget(root)
	if err != nil {
		t.Fatal(err)
	}

	got, err := resolveSensitivePrefix(shadow, home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(resolvedRoot, "etc", "shadow"); got != want {
		t.Fatalf("system prefix = %q, want the parent resolved and the leaf kept: %q", got, want)
	}

	got, err = resolveSensitivePrefix(ssh, home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(resolvedRoot, "real-ssh"); got != want {
		t.Fatalf("home prefix = %q, want the symlinked leaf followed: %q", got, want)
	}

	// A missing leaf under a resolvable parent is fine either way: the parent
	// gives the physical form and the name rides along.
	got, err = resolveSensitivePrefix(filepath.Join(system, "missing"), home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(resolvedRoot, "etc", "missing"); got != want {
		t.Fatalf("missing leaf = %q, want %q", got, want)
	}
}
