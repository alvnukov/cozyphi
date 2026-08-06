package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadContextFileFromDirPrefersAGENTS(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "claude rules")
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "agents rules")

	got := loadContextFileFromDir(dir)
	if got == nil {
		t.Fatal("expected a context file")
	}
	if got.Content != "agents rules" {
		t.Fatalf("prefer AGENTS.md, got %q", got.Content)
	}
}

func TestLoadContextFileFromDirFallsBackToCLAUDE(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "claude only")

	got := loadContextFileFromDir(dir)
	if got == nil || got.Content != "claude only" {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadProjectContextFilesOrder(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, "agent")
	mid := filepath.Join(root, "proj")
	cwd := filepath.Join(mid, "nested")
	mustMkdir(t, agentDir)
	mustMkdir(t, cwd)

	mustWrite(t, filepath.Join(agentDir, "AGENTS.md"), "global")
	mustWrite(t, filepath.Join(mid, "AGENTS.md"), "mid")
	mustWrite(t, filepath.Join(cwd, "CLAUDE.md"), "cwd")

	files := loadProjectContextFiles(cwd, agentDir)
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d: %+v", len(files), files)
	}
	want := []string{"global", "mid", "cwd"}
	for i, w := range want {
		if files[i].Content != w {
			t.Fatalf("files[%d]=%q want %q", i, files[i].Content, w)
		}
	}
}

func TestLoadProjectContextFilesDedupesAgentDir(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "once")

	// When cwd == agentDir, only one entry should appear.
	files := loadProjectContextFiles(dir, dir)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}

func TestFormatProjectContext(t *testing.T) {
	got := formatProjectContext([]ContextFile{
		{Path: "/tmp/AGENTS.md", Content: "use gofmt"},
	})
	if !strings.Contains(got, "<project_context>") {
		t.Fatalf("missing wrapper: %q", got)
	}
	if !strings.Contains(got, `<project_instructions path="/tmp/AGENTS.md">`) {
		t.Fatalf("missing path attr: %q", got)
	}
	if !strings.Contains(got, "use gofmt") {
		t.Fatalf("missing body: %q", got)
	}
	if formatProjectContext(nil) != "" {
		t.Fatal("empty files should yield empty string")
	}
}

func TestPromptIncludesContext(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "always use tabs")

	ctx := formatProjectContext(loadProjectContextFiles(dir, t.TempDir()))
	if !strings.Contains(ctx, "always use tabs") {
		t.Fatalf("expected cwd AGENTS.md in context, got %q", ctx)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
