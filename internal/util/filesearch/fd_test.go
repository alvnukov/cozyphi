package filesearch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveFD(t *testing.T) {
	ResetResolveFDForTest()
	bin, err := ResolveFD()
	if err != nil {
		t.Skip("fd not installed:", err)
	}
	if bin == "" {
		t.Fatal("empty fd path")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("fd path %q not usable: %v", bin, err)
	}
}

func TestSearch(t *testing.T) {
	ResetResolveFDForTest()
	if _, err := ResolveFD(); err != nil {
		t.Skip("fd not installed:", err)
	}

	dir := t.TempDir()
	mustWrite := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("go.mod", "module x\n")
	mustWrite("internal/session/manager.go", "package session\n")
	mustWrite("internal/session/manager_test.go", "package session\n")
	mustWrite("README.md", "# x\n")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	all, err := Search(ctx, dir, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 4 {
		t.Fatalf("expected >=4 files, got %v", all)
	}

	hits, err := Search(ctx, dir, "manager", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected manager hits")
	}
	for _, h := range hits {
		if !strings.Contains(h, "manager") {
			t.Fatalf("unexpected hit %q", h)
		}
		if strings.Contains(h, "\\") {
			t.Fatalf("path should use slashes: %q", h)
		}
	}

	limited, err := Search(ctx, dir, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) > 2 {
		t.Fatalf("limit exceeded: %v", limited)
	}

	none, err := Search(ctx, dir, "zzz-no-such-file-xyz", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("expected empty, got %v", none)
	}
}

func TestSearchMissingFD(t *testing.T) {
	ResetResolveFDForTest()
	if _, err := ResolveFD(); err == nil {
		t.Skip("fd is installed")
	}
	_, err := Search(context.Background(), t.TempDir(), "", 5)
	if err == nil {
		t.Fatal("expected error when fd missing")
	}
}
