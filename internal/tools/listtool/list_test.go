package listtool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestList_RelativePath(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "pkg")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "main.go"), []byte("package pkg"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(root)

	raw, err := json.Marshal(listInput{Path: "pkg"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := runList(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "main.go") {
		t.Fatalf("expected output to contain main.go, got: %s", out.Content)
	}
	if !strings.HasPrefix(out.Content, "pkg/") {
		t.Fatalf("expected cwd-relative tree root pkg/, got: %s", out.Content)
	}
	if out.Detail != "pkg" {
		t.Fatalf("expected detail pkg, got %q", out.Detail)
	}
}

func TestList_Errors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(listInput{Path: file})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runList(t.Context(), raw)
	if err == nil {
		t.Fatal("expected error for file path, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not a directory") {
		t.Fatalf("expected 'not a directory' error, got: %v", err)
	}
}

func TestList_MaxDepthStopsExpansion(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "lvl1", "lvl2", "lvl3"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lvl1", "lvl2", "lvl3", "deep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(listInput{
		Path:     root,
		MaxDepth: 3,
		Limit:    100,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := runList(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	result := out.Content

	if !strings.Contains(result, "lvl1"+string(os.PathSeparator)) {
		t.Fatalf("expected output to contain lvl1/")
	}
	if !strings.Contains(result, "lvl2"+string(os.PathSeparator)) {
		t.Fatalf("expected output to contain lvl2/")
	}
	if !strings.Contains(result, "lvl3"+string(os.PathSeparator)) {
		t.Fatalf("expected output to contain lvl3/")
	}
	if strings.Contains(result, "deep.txt") {
		t.Fatalf("expected output NOT to contain deep.txt at maxDepth=3")
	}
}

func TestList_LimitTriggersTruncationMessage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(listInput{
		Path:  root,
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := runList(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	result := out.Content

	if !strings.Contains(result, "Tree truncated after 1 files") {
		t.Fatalf("expected truncation message, got: %s", result)
	}
	if !strings.Contains(result, "limit=<n>") {
		t.Fatalf("expected limit=<n> hint, got: %s", result)
	}
}

func TestList_DefaultOptionsApplied(t *testing.T) {
	limit, depth := normalizeOptions(0, 0)
	if limit != defaultMaxFiles {
		t.Fatalf("expected limit=%d, got %d", defaultMaxFiles, limit)
	}
	if depth != defaultMaxDepth {
		t.Fatalf("expected depth=%d, got %d", defaultMaxDepth, depth)
	}
}

func TestList_PlainStringPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pass path as a plain JSON string, not an object.
	raw, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	out, err := runList(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "x.txt") {
		t.Fatalf("expected output to contain x.txt, got: %s", out.Content)
	}
}
