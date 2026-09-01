package keys_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

func TestCatalogIsWellFormed(t *testing.T) {
	seenScope := map[keys.Scope]bool{}
	for _, g := range keys.Groups() {
		require.NotEmpty(t, g.Title, "scope %q has no title", g.Scope)
		require.NotEmpty(t, g.Bindings, "scope %q has no bindings", g.Scope)
		require.False(t, seenScope[g.Scope], "scope %q listed twice", g.Scope)
		seenScope[g.Scope] = true

		seenLabel := map[string]bool{}
		for _, b := range g.Bindings {
			require.False(t, b.Hint == "" && b.Desc == "",
				"scope %q: binding %q says nothing", g.Scope, b.Label())
			require.False(t, b.Label() == "" && b.Hint == "",
				"scope %q: a keyless binding needs a hint", g.Scope)
			if label := b.Label(); label != "" {
				require.False(t, seenLabel[label], "scope %q: %q bound twice", g.Scope, label)
				seenLabel[label] = true
			}
		}
	}
}

func TestGroupsIsACopy(t *testing.T) {
	got := keys.Groups()
	require.NotEmpty(t, got)
	got[0].Title = "mutated"
	assert.NotEqual(t, "mutated", keys.Groups()[0].Title, "Groups must hand out a copy")
}

func TestFind(t *testing.T) {
	g, ok := keys.Find(keys.ScopeSettings)
	require.True(t, ok)
	assert.Equal(t, keys.ScopeSettings, g.Scope)

	_, ok = keys.Find(keys.Scope("nope"))
	assert.False(t, ok)
}

// The hint rows are the contract the panes render. Pinning them here is what
// makes the catalog worth having: a reworded binding shows up as a diff on the
// row a user actually reads.
func TestHints(t *testing.T) {
	cases := map[keys.Scope]string{
		keys.ScopePlan:       "↑↓/j/k select · Enter open · / jump · . menu · Ctrl+S apply · Esc close",
		keys.ScopePlanDetail: "Enter edit/action · / jump · . menu · Esc back",
		keys.ScopePlanText:   "Enter/Ctrl+S save · Shift/Ctrl+Enter newline · Esc cancel",
		keys.ScopeJump:       "↑↓ cycle · Enter keep · Esc back",
		keys.ScopeMenu:       "↑↓/j/k move · Enter run · Esc back",
		keys.ScopePlanChoice: "↑↓/j/k move · Enter choose · Esc back",
		keys.ScopeSidebar:    "Alt+P plan · Ctrl+O hide",
		keys.ScopePlanFocus:  "Enter/m model · Esc back",
		keys.ScopePlanPicker: "Enter pick · Esc back",
		keys.ScopeSettings:   "/ jump · . menu · Ctrl+S apply · Esc discard",
		keys.ScopeContext: "↑↓/j/k move · Shift+↑↓ select · Enter view · Del delete · " +
			"t trim · c compact · r refresh · Esc close",
		keys.ScopeContextRaw: "j/k scroll · Enter close",
		keys.ScopeAsk:        "↑↓ move · Enter select · Esc deny",
		keys.ScopeContinue:   "↑↓ move · Enter select · Esc stop",
		keys.ScopeQuestion:   "Tab next · ↑↓ select · Enter confirm · Esc dismiss",
		keys.ScopeAnswer:     "Enter send · Esc cancel",
		keys.ScopeConnect:    "Type to filter · ↑↓ navigate · Enter select · Esc cancel",
		keys.ScopeConnectKey: "Paste or type key · Enter save · Esc cancel",
	}
	for scope, want := range cases {
		assert.Equal(t, want, keys.Hints(scope), "hints for %q", scope)
	}
	assert.Empty(t, keys.Hints(keys.Scope("nope")))
}

func TestFooterPads(t *testing.T) {
	assert.Equal(t, " "+keys.Hints(keys.ScopeSettings)+" ", keys.Footer(keys.ScopeSettings))
	assert.Empty(t, keys.Footer(keys.Scope("nope")), "no group, no padding")
}

// Scopes that exist only to document keys (the chat view itself) have no hint
// row; every binding there must still describe itself for the help screen.
func TestDocumentationOnlyScopes(t *testing.T) {
	docOnly := []keys.Scope{
		keys.ScopeGlobal, keys.ScopeComposer, keys.ScopeTranscript,
	}
	for _, scope := range docOnly {
		assert.Empty(t, keys.Hints(scope), "scope %q has no footer row to render", scope)
	}
	for _, scope := range docOnly {
		g, ok := keys.Find(scope)
		require.True(t, ok)
		for _, b := range g.Bindings {
			assert.NotEmpty(t, b.Desc, "scope %q: %q needs a description", scope, b.Label())
		}
	}
}

func TestHelpScopeDocumentsItsOwnKeys(t *testing.T) {
	hints := keys.Hints(keys.ScopeHelp)
	for _, want := range []string{"scroll", "page", "close"} {
		assert.Contains(t, hints, want, "help row %q lacks %q", hints, want)
	}
}
