package permission

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestExtractBash(t *testing.T) {
	req, err := Extract("bash", json.RawMessage(`{"command":"git status"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Action != ActionBash || req.Command != "git status" {
		t.Fatalf("got %+v", req)
	}
}

func TestExtractWritePath(t *testing.T) {
	req, err := Extract("write", json.RawMessage(`{"path":"out.txt","content":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Action != ActionWrite || len(req.Paths) != 1 {
		t.Fatalf("got %+v", req)
	}
	if !filepath.IsAbs(req.Paths[0]) {
		t.Fatalf("path not abs: %s", req.Paths[0])
	}
}

func TestExtractAtUsesExplicitCwd(t *testing.T) {
	root := t.TempDir()
	req, err := ExtractAt("write", json.RawMessage(`{"path":"out.txt","content":"x"}`), root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "out.txt")
	if len(req.Paths) != 1 || req.Paths[0] != want {
		t.Fatalf("got %v, want %s", req.Paths, want)
	}
}

func TestExtractEditFilePath(t *testing.T) {
	req, err := Extract("edit", json.RawMessage(`{"file_path":"a.go","edits":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Action != ActionEdit || len(req.Paths) != 1 {
		t.Fatalf("got %+v", req)
	}
}
