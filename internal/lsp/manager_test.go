package lsp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupWorkspace(t *testing.T) (dir, mainFile, otherFile string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mainFile = filepath.Join(dir, "main.go")
	otherFile = filepath.Join(dir, "other.go")
	if err := os.WriteFile(mainFile, []byte("package main\n\nfunc main() {\n\tf()\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(otherFile, []byte("package main\n\nfunc f() {\n\t// body\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, mainFile, otherFile
}

func TestOpenDisabledReturnsNil(t *testing.T) {
	mgr, err := Open(t.Context(), t.TempDir(), Config{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if mgr != nil {
		t.Fatal("disabled config must return a nil manager")
	}
}

func TestDefinitionEndToEnd(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	fixture := defFixture(uriFromPath(otherFile))
	mgr, err := Open(t.Context(), dir, fakeConfig(
		"LSP_TEST_HISTORY="+hist,
		"LSP_TEST_DEF_RESULT="+fixture,
	))
	if err != nil {
		t.Fatal(err)
	}
	res, err := mgr.Query(t.Context(), Query{
		Op: OpDefinition, File: mainFile, Line: 5, Character: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Locations) != 1 {
		t.Fatalf("locations: %+v", res.Locations)
	}
	loc := res.Locations[0]
	if loc.File != "other.go" || loc.Line != 3 || loc.Character != 6 || loc.EndLine != 3 || loc.EndCharacter != 7 {
		t.Fatalf("location: %+v", loc)
	}

	if err := mgr.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := []string{"initialize", "initialized", "textDocument/didOpen", "textDocument/definition", "shutdown", "exit"}
	got := history(t, hist)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("history = %v, want %v", got, want)
	}
}

func TestDefinitionLocationLinkAndNull(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)

	hist := filepath.Join(t.TempDir(), "history")
	link := fmt.Sprintf(
		`[{"targetUri":%q,"targetSelectionRange":{"start":{"line":1,"character":0},"end":{"line":1,"character":3}}}]`,
		uriFromPath(otherFile),
	)
	mgr, err := Open(t.Context(), dir, fakeConfig(
		"LSP_TEST_HISTORY="+hist,
		"LSP_TEST_DEF_RESULT="+link,
	))
	if err != nil {
		t.Fatal(err)
	}
	res, err := mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 1, Character: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Locations) != 1 || res.Locations[0].File != "other.go" || res.Locations[0].Line != 2 {
		t.Fatalf("link locations: %+v", res.Locations)
	}
	_ = mgr.Close(t.Context())

	hist2 := filepath.Join(t.TempDir(), "history")
	mgr2, err := Open(t.Context(), dir, fakeConfig(
		"LSP_TEST_HISTORY="+hist2,
		"LSP_TEST_DEF_RESULT=null",
	))
	if err != nil {
		t.Fatal(err)
	}
	res2, err := mgr2.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 1, Character: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Locations) != 0 {
		t.Fatalf("null should be empty: %+v", res2.Locations)
	}
	_ = mgr2.Close(t.Context())
}

func TestDefinitionEscapingURIFails(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	outside := filepath.Join(t.TempDir(), "escape.go")
	fixture := defFixture(uriFromPath(outside))
	mgr, err := Open(t.Context(), dir, fakeConfig(
		"LSP_TEST_HISTORY="+hist,
		"LSP_TEST_DEF_RESULT="+fixture,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close(t.Context())
	_, err = mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 1, Character: 1})
	var lerr *Error
	if !errors.As(err, &lerr) || lerr.Kind != ErrProtocol {
		t.Fatalf("want protocol error, got %v", err)
	}
}

func TestInvalidAndUnsupportedQueries(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	mgr, err := Open(t.Context(), dir, fakeConfig("LSP_TEST_HISTORY="+hist))
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close(t.Context())

	if _, err := mgr.Query(
		t.Context(),
		Query{Op: OpDefinition, File: mainFile, Line: 0, Character: 1},
	); errKind(
		err,
	) != ErrInvalid {
		t.Fatalf("want invalid, got %v", err)
	}
	if _, err := mgr.Query(t.Context(), Query{Op: OpLanguages}); err != nil {
		t.Fatalf("languages is supported and must not start a process: %v", err)
	}
	if got := history(t, hist); len(got) != 0 {
		t.Fatalf("no process should have started, history: %v", got)
	}
}

func TestCloseIdempotentAndRejectsNew(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	mgr, err := Open(t.Context(), dir, fakeConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Query(
		t.Context(),
		Query{Op: OpDefinition, File: mainFile, Line: 1, Character: 1},
	); errKind(
		err,
	) != ErrClosed {
		t.Fatalf("want closed, got %v", err)
	}
}
