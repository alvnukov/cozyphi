package update_test

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/util/update"
)

func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.1.0", "v0.2.0", true},
		{"0.1.0", "v0.2.0", true},
		{"v0.2.0", "v0.1.0", false},
		{"v0.2.0", "v0.2.0", false},
		{"v0.1.0 (abc)", "v0.1.1", true},
	}
	for _, tc := range cases {
		if got := update.VersionLess(tc.a, tc.b); got != tc.want {
			t.Fatalf("VersionLess(%q, %q)=%v want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestIsDevBuild(t *testing.T) {
	if !update.IsDevBuild("dev") || !update.IsDevBuild("") || !update.IsDevBuild("v0.0.0") {
		t.Fatal("expected dev builds")
	}
	if update.IsDevBuild("v0.1.0") {
		t.Fatal("v0.1.0 should not be dev")
	}
}
