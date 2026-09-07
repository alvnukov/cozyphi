package permission

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSensitivePathsAreAbsoluteOnEveryPlatform(t *testing.T) {
	t.Parallel()
	unix := sensitivePathsFor("linux", "/home/x")
	if !slices.Contains(unix, "/etc/shadow") || !slices.Contains(unix, filepath.Join("/home/x", ".ssh")) {
		t.Fatalf("linux list lost an entry: %v", unix)
	}
	if mac := sensitivePathsFor("darwin", "/Users/x"); !slices.Contains(mac, "/etc/shadow") {
		t.Fatalf("darwin list lost /etc/shadow: %v", mac)
	}
	home := `C:\Users\x`
	win := sensitivePathsFor("windows", home)
	if len(win) != 4 {
		t.Fatalf("windows list = %v, want the four home entries", win)
	}
	for _, p := range win {
		if strings.HasPrefix(p, "/") || !strings.HasPrefix(p, home) {
			t.Fatalf("windows entry %q is not rooted at the home directory", p)
		}
	}
	// A Windows host with no home has nothing absolute to name: unix
	// fallbacks would make NewGate fail closed and leave the session gateless.
	if got := sensitivePathsFor("windows", ""); len(got) != 0 {
		t.Fatalf("windows without home = %v, want none", got)
	}
	if got := sensitivePathsFor("linux", ""); !slices.Contains(got, "/etc/shadow") || !slices.Contains(got, "/.ssh") {
		t.Fatalf("linux without home = %v, want the rooted fallbacks", got)
	}
}

func TestPrefixMatchFoldsCaseOnlyWhereTheFilesystemDoes(t *testing.T) {
	prev := caseInsensitivePaths
	t.Cleanup(func() { caseInsensitivePaths = prev })
	prefix := filepath.Join(string(filepath.Separator), "Users", "x", ".ssh")
	other := filepath.Join(string(filepath.Separator), "users", "X", ".SSH", "id_rsa")
	sibling := filepath.Join(string(filepath.Separator), "users", "X", ".sshd", "config")

	caseInsensitivePaths = false
	if matchesPrefix(other, []string{prefix}) {
		t.Fatal("case-sensitive filesystem matched a differently-cased path")
	}
	caseInsensitivePaths = true
	if !matchesPrefix(other, []string{prefix}) {
		t.Fatal("case-insensitive filesystem missed a differently-cased path under the prefix")
	}
	if matchesPrefix(sibling, []string{prefix}) {
		t.Fatal("a sibling sharing the spelling of the prefix matched")
	}
	if !matchesPrefix(strings.ToUpper(prefix), []string{prefix}) {
		t.Fatal("the prefix itself in another case did not match")
	}
}
