package tools_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/agent/prompt"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// The edit capability has one lifecycle, and five model-facing surfaces
// describe it: the read, edit, write and grep descriptions the model sees on
// every turn, the assembled system prompt, and doc/context-loading.md. When
// they disagree the model manufactures re-reads and protocol mistakes, so the
// agreement is asserted on the public surfaces rather than on the internals
// that produce them.

// modelFacingSurfaces returns the surfaces by name: the four tool descriptions
// taken from the real tool set's public llm.ToolDefinitions, the rendered
// system prompt, and the design doc read from the repo.
func modelFacingSurfaces(t *testing.T) map[string]string {
	t.Helper()
	surfaces := map[string]string{}
	for _, tool := range tools.DefaultTools() {
		switch tool.Definition.Name {
		case "read", "edit", "write", "grep":
			surfaces[tool.Definition.Name] = tool.Definition.Description
		}
	}
	for _, name := range []string{"read", "edit", "write", "grep"} {
		if surfaces[name] == "" {
			t.Fatalf("tool %q is missing from DefaultTools or carries no description", name)
		}
	}
	// prompt.Build appends whatever AGENTS.md / CLAUDE.md sits above cwd, so
	// the assembled prompt is rendered from an empty directory: this test
	// judges the harness's own wording, not the checkout it happens to run in.
	t.Chdir(t.TempDir())
	// Every toggle on: the prompt must stay consistent in its widest form,
	// not only in the minimal one.
	surfaces["system prompt"] = prompt.Build(prompt.Options{})
	surfaces["system prompt (all capabilities)"] = prompt.Build(prompt.Options{
		Agents: true, LSP: true, Watches: true, Plan: true,
	})
	surfaces["doc/context-loading.md"] = readRepoFile(t, "doc", "context-loading.md")
	return surfaces
}

// readRepoFile reads a repository file through a path relative to this test
// file, so a test that chdir'd elsewhere cannot change what is checked.
func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.Join(parts...)))
	if err != nil {
		t.Fatalf("read %v: %v", parts, err)
	}
	return string(data)
}

func TestModelFacingSurfacesDropContradictedEditLifecycle(t *testing.T) {
	forbidden := []struct {
		phrase string
		guards string
	}{
		// A successful edit does not end the capability: it commits a
		// successor grant and prints its anchors.
		{"ends the authorization", "a successful edit mints a successor grant"},
		// Re-reading after every edit is exactly the round-trip the successor
		// and post-write grants exist to remove.
		{"re-read before editing", "printed anchors authorize the next edit"},
		// A failed attempt releases the claim unchanged; the grant is not
		// spent per attempt.
		{"one edit attempt", "a failed edit keeps the authorization"},
		{"one-shot", "capability consumption is not per attempt"},
		{"Consumption is one-shot", "capability consumption is not per attempt"},
		// The first edit of a file needs an editable observation; later edits
		// ride the anchors the previous result printed.
		{"Before `edit`, `read`", "only the first edit of a file needs a read"},
	}
	for name, surface := range modelFacingSurfaces(t) {
		for _, bad := range forbidden {
			if strings.Contains(surface, bad.phrase) {
				t.Errorf("%s still says %q; the lifecycle says %s", name, bad.phrase, bad.guards)
			}
		}
	}
}

func TestModelFacingSurfacesStateTheEditGrantChain(t *testing.T) {
	required := map[string][]string{
		"edit": {
			"View reads never authorize edits.",
			"A failed edit keeps the authorization: fix the call and retry without re-reading.",
			"A successful edit replaces it, printing the new TAG and live anchors that",
			`any other line needs a fresh read with mode:"edit" first`,
			"never resend the same call unchanged",
		},
		"read": {
			"View reads never authorize edits.",
			"A failed edit keeps the authorization: fix the call and retry without re-reading.",
			"A successful edit or write replaces it with the anchors that result prints",
		},
		"write": {
			"live LINE#HASH anchors that authorize the next edit without re-reading",
			`any other line needs a fresh read with mode:"edit" of that range first`,
		},
		"grep": {
			"authorize an edit of exactly those lines",
			`any other line needs a read with mode:"edit" of that range first`,
		},
		"system prompt": {
			"The first `edit` of a file needs an editable observation of it in this session",
			"view reads never authorize edits",
			"prints the file's new TAG and live `LINE#HASH` anchors that authorize the next edit of that file without re-reading",
			"A failed `edit` keeps the authorization",
			"On an `[edit:<code>]` refusal, do what its message says; never resend the same call unchanged.",
		},
		"doc/context-loading.md": {
			"Failure does not consume it",
			"commits a successor grant in its place",
			"A successful `write` mints the same kind of grant",
			"any other line needs a fresh `read` with `mode:\"edit\"` of that range first",
			"mixed_grants",
		},
	}
	surfaces := modelFacingSurfaces(t)
	for name, phrases := range required {
		surface, ok := surfaces[name]
		if !ok {
			t.Fatalf("surface %q not collected", name)
		}
		for _, phrase := range phrases {
			if !strings.Contains(surface, phrase) {
				t.Errorf("%s must state %q", name, phrase)
			}
		}
	}
}
