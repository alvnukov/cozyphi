package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openNav opens a Manager on the fake server with extra env fixtures and
// closes it with the test.
func openNav(t *testing.T, dir string, env ...string) *Manager {
	t.Helper()
	mgr, err := Open(t.Context(), dir, fakeConfig(env...))
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })
	return mgr
}

// paramsPath returns a unique wire-params log path for this test.
func paramsPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "params")
}

// wireParams returns the params JSON of the last recorded call to method.
func wireParams(t *testing.T, path, method string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	found := ""
	for line := range strings.SplitSeq(string(raw), "\n") {
		name, params, ok := strings.Cut(line, "\t")
		if ok && name == method {
			found = params
		}
	}
	return found
}

func TestDefinitionBySymbolResolvesUniqueTarget(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	params := paramsPath(t)
	syms := `[{"name":"main","kind":12,"range":{"start":{"line":2,"character":0},"end":{"line":4,"character":1}},"selectionRange":{"start":{"line":2,"character":5},"end":{"line":2,"character":9}},"children":[{"name":"f","kind":12,"range":{"start":{"line":3,"character":1},"end":{"line":3,"character":4}},"selectionRange":{"start":{"line":3,"character":2},"end":{"line":3,"character":3}}}]}]`
	mgr := openNav(t, dir,
		"LSP_TEST_PARAMS="+params,
		"LSP_TEST_DOC_SYM_RESULT="+syms,
		"LSP_TEST_DEF_RESULT="+defFixture(uriFromPath(otherFile)),
	)
	res, err := mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Symbol: "f"})
	require.NoError(t, err)
	require.Len(t, res.Locations, 1)
	assert.Equal(t, "other.go", res.Locations[0].File)
	// The definition request targets the child symbol's selection start.
	def := wireParams(t, params, "textDocument/definition")
	assert.Contains(t, def, `"line":3`)
	assert.Contains(t, def, `"character":2`)
}

func TestDefinitionBySymbolAmbiguityReturnsCandidates(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	params := paramsPath(t)
	syms := `[{"name":"f","kind":12,"range":{"start":{"line":2,"character":0},"end":{"line":2,"character":9}},"selectionRange":{"start":{"line":2,"character":5},"end":{"line":2,"character":6}}},{"name":"f","kind":12,"range":{"start":{"line":3,"character":1},"end":{"line":3,"character":4}},"selectionRange":{"start":{"line":3,"character":2},"end":{"line":3,"character":3}}}]`
	mgr := openNav(t, dir, "LSP_TEST_PARAMS="+params, "LSP_TEST_DOC_SYM_RESULT="+syms)
	res, err := mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Symbol: "f"})
	require.NoError(t, err)
	assert.Len(t, res.Locations, 2)
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], "ambiguous")
	// No definition request follows an ambiguous target.
	assert.Empty(t, wireParams(t, params, "textDocument/definition"))
}

func TestReferencesDeclarationFlagAndDedup(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	refs := fmt.Sprintf(
		`[{"uri":%q,"range":{"start":{"line":3,"character":1},"end":{"line":3,"character":2}}},{"uri":%q,"range":{"start":{"line":3,"character":1},"end":{"line":3,"character":2}}},{"uri":%q,"range":{"start":{"line":2,"character":5},"end":{"line":2,"character":6}}}]`,
		uriFromPath(mainFile),
		uriFromPath(mainFile),
		uriFromPath(otherFile),
	)
	params := paramsPath(t)
	mgr := openNav(t, dir, "LSP_TEST_PARAMS="+params, "LSP_TEST_REF_RESULT="+refs)
	res, err := mgr.Query(t.Context(), Query{
		Op: OpReferences, File: mainFile, Line: 4, Character: 2, IncludeDeclaration: true,
	})
	require.NoError(t, err)
	require.Len(t, res.Locations, 2) // the duplicate main.go entry collapses
	assert.Equal(t, "main.go", res.Locations[0].File)
	assert.Equal(t, "other.go", res.Locations[1].File)
	assert.Contains(t, wireParams(t, params, "textDocument/references"), `"includeDeclaration":true`)

	params2 := paramsPath(t)
	mgr2 := openNav(t, dir, "LSP_TEST_PARAMS="+params2, "LSP_TEST_REF_RESULT="+refs)
	_, err = mgr2.Query(t.Context(), Query{
		Op: OpReferences, File: mainFile, Line: 4, Character: 2, IncludeDeclaration: false,
	})
	require.NoError(t, err)
	assert.Contains(t, wireParams(t, params2, "textDocument/references"), `"includeDeclaration":false`)
}

func TestReferencesNullIsEmptyAndMinimalCapsUnsupported(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	mgr := openNav(t, dir, "LSP_TEST_REF_RESULT=null")
	res, err := mgr.Query(t.Context(), Query{Op: OpReferences, File: mainFile, Line: 1, Character: 1})
	require.NoError(t, err)
	assert.Empty(t, res.Locations)

	mgr2 := openNav(t, dir, "LSP_TEST_CAPS=minimal", "LSP_TEST_REF_RESULT=null")
	_, err = mgr2.Query(t.Context(), Query{Op: OpReferences, File: mainFile, Line: 1, Character: 1})
	var lerr *Error
	require.ErrorAs(t, err, &lerr)
	assert.Equal(t, ErrUnsupported, lerr.Kind)
}

func TestHoverWireShapes(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{"plaintext contents", `{"contents":"hello"}`, "hello"},
		{"marked string array", `{"contents":["one",{"language":"go","value":"x := 1"}]}`, "one\n```go\nx := 1\n```"},
		{"markup content", `{"contents":{"kind":"markdown","value":"**md**"}}`, "**md**"},
		{"language block", `{"contents":{"language":"go","value":"v"}}`, "```go\nv\n```"},
		{"value only object", `{"contents":{"value":"bare"}}`, "bare"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := openNav(t, dir, "LSP_TEST_HOVER_RESULT="+tt.payload)
			res, err := mgr.Query(t.Context(), Query{Op: OpHover, File: mainFile, Line: 4, Character: 2})
			require.NoError(t, err)
			require.NotNil(t, res.Hover)
			assert.Equal(t, tt.want, res.Hover.Text)
		})
	}
}

func TestHoverNullRangeAndTruncation(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	mgr := openNav(t, dir, "LSP_TEST_HOVER_RESULT=null")
	res, err := mgr.Query(t.Context(), Query{Op: OpHover, File: mainFile, Line: 4, Character: 2})
	require.NoError(t, err)
	assert.Nil(t, res.Hover)

	payload := `{"contents":"r","range":{"start":{"line":3,"character":1},"end":{"line":3,"character":4}}}`
	mgr2 := openNav(t, dir, "LSP_TEST_HOVER_RESULT="+payload)
	res2, err := mgr2.Query(t.Context(), Query{Op: OpHover, File: mainFile, Line: 4, Character: 2})
	require.NoError(t, err)
	require.NotNil(t, res2.Hover)
	assert.Equal(t, 4, res2.Hover.Line)
	assert.Equal(t, 2, res2.Hover.Character)
	assert.Equal(t, 4, res2.Hover.EndLine)
	assert.Equal(t, 5, res2.Hover.EndCharacter)

	big := fmt.Sprintf(`{"contents":%q}`, strings.Repeat("x", MaxTextFieldBytes+64))
	mgr3 := openNav(t, dir, "LSP_TEST_HOVER_RESULT="+big)
	res3, err := mgr3.Query(t.Context(), Query{Op: OpHover, File: mainFile, Line: 4, Character: 2})
	require.NoError(t, err)
	require.NotNil(t, res3.Hover)
	assert.True(t, res3.Truncated)
	assert.Len(t, res3.Hover.Text, MaxTextFieldBytes)
}

func TestSymbolsFileFlattensNested(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	syms := `[{"name":"main","kind":12,"detail":"func()","range":{"start":{"line":2,"character":0},"end":{"line":4,"character":1}},"selectionRange":{"start":{"line":2,"character":5},"end":{"line":2,"character":9}},"children":[{"name":"inner","kind":6,"range":{"start":{"line":3,"character":1},"end":{"line":3,"character":6}},"selectionRange":{"start":{"line":3,"character":1},"end":{"line":3,"character":6}}}]}]`
	mgr := openNav(t, dir, "LSP_TEST_DOC_SYM_RESULT="+syms)
	res, err := mgr.Query(t.Context(), Query{Op: OpSymbols, File: mainFile})
	require.NoError(t, err)
	require.Len(t, res.Symbols, 2)
	assert.Equal(t, "main", res.Symbols[0].Name)
	assert.Equal(t, "function", res.Symbols[0].Kind)
	assert.Equal(t, "func()", res.Symbols[0].Detail)
	assert.Empty(t, res.Symbols[0].Container)
	assert.Equal(t, "inner", res.Symbols[1].Name)
	assert.Equal(t, "method", res.Symbols[1].Kind)
	assert.Equal(t, "main", res.Symbols[1].Container)

	res, err = mgr.Query(t.Context(), Query{Op: OpSymbols, File: mainFile, Limit: 1})
	require.NoError(t, err)
	assert.Len(t, res.Symbols, 1)
	assert.Equal(t, 1, res.Omitted)
	assert.True(t, res.Truncated)
}

func TestSymbolsWorkspaceQuery(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	ws := fmt.Sprintf(
		`[{"name":"Query","kind":23,"location":{"uri":%q,"range":{"start":{"line":38,"character":8},"end":{"line":48,"character":1}}},"containerName":"lsp"},{"name":"Ghost","kind":12},{"name":"Odd","kind":99,"location":{"uri":%q,"range":{"start":{"line":2,"character":5},"end":{"line":2,"character":6}}}}]`,
		uriFromPath(mainFile),
		uriFromPath(otherFile),
	)
	params := paramsPath(t)
	mgr := openNav(t, dir, "LSP_TEST_PARAMS="+params, "LSP_TEST_WS_SYM_RESULT="+ws)
	res, err := mgr.Query(t.Context(), Query{Op: OpSymbols, Query: "Que"})
	require.NoError(t, err)
	require.Len(t, res.Symbols, 2)
	assert.Equal(t, "Query", res.Symbols[0].Name)
	assert.Equal(t, "struct", res.Symbols[0].Kind)
	assert.Equal(t, "lsp", res.Symbols[0].Container)
	assert.Equal(t, "main.go", res.Symbols[0].Location.File)
	assert.Equal(t, "unknown:99", res.Symbols[1].Kind)
	assert.Equal(t, 1, res.Omitted) // Ghost carries no location
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], "without a location")
	assert.Contains(t, wireParams(t, params, "workspace/symbol"), `"query":"Que"`)
}

func TestCallsIncomingPreservesOpaqueData(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	prepare := fmt.Sprintf(
		`[{"name":"f","kind":12,"detail":"func()","uri":%q,"range":{"start":{"line":2,"character":5},"end":{"line":4,"character":1}},"selectionRange":{"start":{"line":2,"character":5},"end":{"line":2,"character":6}},"data":{"opaque":42}}]`,
		uriFromPath(otherFile),
	)
	incoming := fmt.Sprintf(
		`[{"from":{"name":"main","kind":12,"uri":%q,"range":{"start":{"line":2,"character":0},"end":{"line":4,"character":1}},"selectionRange":{"start":{"line":2,"character":5},"end":{"line":2,"character":9}}},"fromRanges":[{"start":{"line":3,"character":1},"end":{"line":3,"character":4}}]}]`,
		uriFromPath(mainFile),
	)
	params := paramsPath(t)
	mgr := openNav(t, dir,
		"LSP_TEST_PARAMS="+params,
		"LSP_TEST_CALL_PREPARE_RESULT="+prepare,
		"LSP_TEST_CALL_RESULT="+incoming,
	)
	res, err := mgr.Query(t.Context(), Query{
		Op: OpCalls, File: mainFile, Line: 4, Character: 2, Direction: DirectionIncoming,
	})
	require.NoError(t, err)
	require.Len(t, res.Calls, 1)
	edge := res.Calls[0]
	assert.Equal(t, "main", edge.From.Name)
	assert.Equal(t, "f", edge.To.Name)
	assert.Equal(t, "main.go", edge.Location.File)
	assert.Equal(t, 4, edge.Location.Line)
	assert.Equal(t, 2, edge.Location.Character)
	// The follow-up request replays the server's opaque data verbatim.
	assert.Contains(t, wireParams(t, params, "callHierarchy/incomingCalls"), `"data":{"opaque":42}`)
}

func TestCallsOutgoingMapsEdges(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	prepare := fmt.Sprintf(
		`[{"name":"f","kind":12,"uri":%q,"range":{"start":{"line":2,"character":5},"end":{"line":4,"character":1}},"selectionRange":{"start":{"line":2,"character":5},"end":{"line":2,"character":6}}}]`,
		uriFromPath(otherFile),
	)
	outgoing := fmt.Sprintf(
		`[{"to":{"name":"g","kind":12,"uri":%q,"range":{"start":{"line":1,"character":0},"end":{"line":2,"character":1}},"selectionRange":{"start":{"line":1,"character":5},"end":{"line":1,"character":6}}},"fromRanges":[{"start":{"line":3,"character":1},"end":{"line":3,"character":4}}]}]`,
		uriFromPath(mainFile),
	)
	mgr := openNav(t, dir, "LSP_TEST_CALL_PREPARE_RESULT="+prepare, "LSP_TEST_CALL_RESULT="+outgoing)
	res, err := mgr.Query(t.Context(), Query{
		Op: OpCalls, File: mainFile, Line: 4, Character: 2, Direction: DirectionOutgoing,
	})
	require.NoError(t, err)
	require.Len(t, res.Calls, 1)
	edge := res.Calls[0]
	assert.Equal(t, "f", edge.From.Name) // the prepared item is the caller
	assert.Equal(t, "g", edge.To.Name)
	// Outgoing call sites live in the caller's document.
	assert.Equal(t, "other.go", edge.Location.File)
}

func TestCallsPrepareAmbiguityFails(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	prepare := fmt.Sprintf(
		`[{"name":"a","kind":12,"uri":%q,"range":{"start":{"line":3,"character":1},"end":{"line":3,"character":4}},"selectionRange":{"start":{"line":3,"character":1},"end":{"line":3,"character":2}}},{"name":"b","kind":12,"uri":%q,"range":{"start":{"line":3,"character":1},"end":{"line":3,"character":4}},"selectionRange":{"start":{"line":3,"character":2},"end":{"line":3,"character":3}}}]`,
		uriFromPath(mainFile),
		uriFromPath(mainFile),
	)
	mgr := openNav(t, dir, "LSP_TEST_CALL_PREPARE_RESULT="+prepare)
	_, err := mgr.Query(t.Context(), Query{
		Op: OpCalls, File: mainFile, Line: 4, Character: 2, Direction: DirectionIncoming,
	})
	var lerr *Error
	require.ErrorAs(t, err, &lerr)
	assert.Equal(t, ErrAmbiguous, lerr.Kind)
}

func TestCallsBySymbolAmbiguityFails(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	syms := `[{"name":"f","kind":12,"range":{"start":{"line":2,"character":0},"end":{"line":2,"character":9}},"selectionRange":{"start":{"line":2,"character":5},"end":{"line":2,"character":6}}},{"name":"f","kind":12,"range":{"start":{"line":3,"character":1},"end":{"line":3,"character":4}},"selectionRange":{"start":{"line":3,"character":2},"end":{"line":3,"character":3}}}]`
	mgr := openNav(t, dir, "LSP_TEST_DOC_SYM_RESULT="+syms)
	_, err := mgr.Query(t.Context(), Query{
		Op: OpCalls, File: mainFile, Symbol: "f", Direction: DirectionIncoming,
	})
	var lerr *Error
	require.ErrorAs(t, err, &lerr)
	assert.Equal(t, ErrAmbiguous, lerr.Kind)
}
