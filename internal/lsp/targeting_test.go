package lsp

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ambiguousSyms declares two symbols named f: selections at wire line 2 char 5
// and wire line 3 char 2.
const ambiguousSyms = `[{"name":"f","kind":12,"range":{"start":{"line":2,"character":0},"end":{"line":2,"character":9}},"selectionRange":{"start":{"line":2,"character":5},"end":{"line":2,"character":6}}},{"name":"f","kind":12,"range":{"start":{"line":3,"character":1},"end":{"line":3,"character":4}},"selectionRange":{"start":{"line":3,"character":2},"end":{"line":3,"character":3}}}]`

func TestSymbolWithLinePicksNearestDeclaration(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	params := paramsPath(t)
	mgr := openNav(t, dir,
		"LSP_TEST_PARAMS="+params,
		"LSP_TEST_DOC_SYM_RESULT="+ambiguousSyms,
		"LSP_TEST_DEF_RESULT="+defFixture(uriFromPath(otherFile)),
	)
	// Hint line 4 (wire 3) sits on the second declaration.
	res, err := mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Symbol: "f", Line: 4})
	require.NoError(t, err)
	require.Len(t, res.Locations, 1)
	def := wireParams(t, params, "textDocument/definition")
	assert.Contains(t, def, `"line":3`)
	assert.Contains(t, def, `"character":2`)
}

func TestSymbolWithFullPositionUsesThePosition(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	params := paramsPath(t)
	// No declarations at all: the given position must win as-is.
	mgr := openNav(t, dir,
		"LSP_TEST_PARAMS="+params,
		"LSP_TEST_DEF_RESULT="+defFixture(uriFromPath(otherFile)),
	)
	res, err := mgr.Query(t.Context(), Query{
		Op: OpDefinition, File: mainFile, Symbol: "f", Line: 4, Character: 2,
	})
	require.NoError(t, err)
	require.Len(t, res.Locations, 1)
	def := wireParams(t, params, "textDocument/definition")
	assert.Contains(t, def, `"line":3`)
	assert.Contains(t, def, `"character":1`)
}

func TestUndeclaredSymbolResolvesByOccurrence(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	params := paramsPath(t)
	refs := fmt.Sprintf(
		`[{"uri":%q,"range":{"start":{"line":3,"character":1},"end":{"line":3,"character":2}}}]`,
		uriFromPath(mainFile),
	)
	mgr := openNav(t, dir, "LSP_TEST_PARAMS="+params, "LSP_TEST_REF_RESULT="+refs)
	// f is only called in main.go, never declared there: the occurrence on
	// line 4 ("\tf()") must become the target position.
	res, err := mgr.Query(t.Context(), Query{Op: OpReferences, File: mainFile, Symbol: "f"})
	require.NoError(t, err)
	require.Len(t, res.Locations, 1)
	req := wireParams(t, params, "textDocument/references")
	assert.Contains(t, req, `"line":3`)
	assert.Contains(t, req, `"character":1`)
}

func TestAbsentSymbolAnswersWithWarning(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	params := paramsPath(t)
	mgr := openNav(t, dir, "LSP_TEST_PARAMS="+params)
	res, err := mgr.Query(t.Context(), Query{Op: OpReferences, File: mainFile, Symbol: "zzz"})
	require.NoError(t, err)
	assert.Empty(t, res.Locations)
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], `"zzz" does not appear in main.go`)
	assert.Empty(t, wireParams(t, params, "textDocument/references"))
}

func TestQualifiedContainerNameMatches(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	params := paramsPath(t)
	syms := `[{"name":"T","kind":23,"range":{"start":{"line":0,"character":0},"end":{"line":4,"character":1}},"selectionRange":{"start":{"line":0,"character":5},"end":{"line":0,"character":6}},"children":[{"name":"m","kind":6,"range":{"start":{"line":2,"character":0},"end":{"line":3,"character":1}},"selectionRange":{"start":{"line":2,"character":7},"end":{"line":2,"character":8}}}]}]`
	mgr := openNav(t, dir,
		"LSP_TEST_PARAMS="+params,
		"LSP_TEST_DOC_SYM_RESULT="+syms,
		"LSP_TEST_DEF_RESULT="+defFixture(uriFromPath(otherFile)),
	)
	_, err := mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Symbol: "T.m"})
	require.NoError(t, err)
	def := wireParams(t, params, "textDocument/definition")
	assert.Contains(t, def, `"line":2`)
	assert.Contains(t, def, `"character":7`)
}

func wsSymFixture(uri string) string {
	return fmt.Sprintf(
		`[{"name":"f","kind":12,"location":{"uri":%q,"range":{"start":{"line":2,"character":5},"end":{"line":2,"character":6}}}}]`,
		uri,
	)
}

func TestFilelessSymbolResolvesWorkspaceWide(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	params := paramsPath(t)
	mgr := openNav(t, dir,
		"LSP_TEST_PARAMS="+params,
		"LSP_TEST_WS_SYM_RESULT="+wsSymFixture(uriFromPath(otherFile)),
		"LSP_TEST_DEF_RESULT="+defFixture(uriFromPath(mainFile)),
	)
	res, err := mgr.Query(t.Context(), Query{Op: OpDefinition, Symbol: "f"})
	require.NoError(t, err)
	require.Len(t, res.Locations, 1)
	assert.Contains(t, wireParams(t, params, "workspace/symbol"), `"query":"f"`)
	// The query continued at the declaration inside other.go.
	def := wireParams(t, params, "textDocument/definition")
	assert.Contains(t, def, `"line":2`)
	assert.Contains(t, def, `"character":5`)
}

func TestFilelessAmbiguityReturnsCandidates(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	params := paramsPath(t)
	two := fmt.Sprintf(
		`[{"name":"f","kind":12,"location":{"uri":%q,"range":{"start":{"line":2,"character":5},"end":{"line":2,"character":6}}}},{"name":"f","kind":12,"location":{"uri":%q,"range":{"start":{"line":3,"character":1},"end":{"line":3,"character":2}}}}]`,
		uriFromPath(otherFile),
		uriFromPath(mainFile),
	)
	mgr := openNav(t, dir, "LSP_TEST_PARAMS="+params, "LSP_TEST_WS_SYM_RESULT="+two)

	// definition: the candidate declarations are the answer.
	res, err := mgr.Query(t.Context(), Query{Op: OpDefinition, Symbol: "f"})
	require.NoError(t, err)
	assert.Len(t, res.Locations, 2)
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], "declarations in the workspace")
	assert.Empty(t, wireParams(t, params, "textDocument/definition"))

	// references: candidates come back as symbols to requalify with.
	res, err = mgr.Query(t.Context(), Query{Op: OpReferences, Symbol: "f"})
	require.NoError(t, err)
	assert.Empty(t, res.Locations)
	assert.Len(t, res.Symbols, 2)
	assert.Empty(t, wireParams(t, params, "textDocument/references"))
}

func TestFilelessMissListsNearestSymbols(t *testing.T) {
	dir, _, otherFile := setupWorkspace(t)
	near := fmt.Sprintf(
		`[{"name":"fHelper","kind":12,"location":{"uri":%q,"range":{"start":{"line":2,"character":5},"end":{"line":2,"character":6}}}}]`,
		uriFromPath(otherFile),
	)
	mgr := openNav(t, dir, "LSP_TEST_WS_SYM_RESULT="+near)
	res, err := mgr.Query(t.Context(), Query{Op: OpDefinition, Symbol: "fx"})
	require.NoError(t, err)
	assert.Empty(t, res.Locations)
	require.Len(t, res.Symbols, 1)
	assert.Equal(t, "fHelper", res.Symbols[0].Name)
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], `no declaration named "fx"`)
}

func TestImplementationsEndToEndWithSnippet(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	impl := fmt.Sprintf(
		`[{"uri":%q,"range":{"start":{"line":2,"character":5},"end":{"line":2,"character":6}}}]`,
		uriFromPath(otherFile),
	)
	mgr := openNav(t, dir, "LSP_TEST_IMPL_RESULT="+impl)
	res, err := mgr.Query(t.Context(), Query{Op: OpImplementations, File: mainFile, Line: 4, Character: 2})
	require.NoError(t, err)
	require.Len(t, res.Locations, 1)
	assert.Equal(t, "other.go", res.Locations[0].File)
	assert.Equal(t, "func f() {", res.Locations[0].Snippet)
}

func TestTypeDefinitionDecodesSingleObject(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	single := fmt.Sprintf(
		`{"uri":%q,"range":{"start":{"line":2,"character":5},"end":{"line":2,"character":6}}}`,
		uriFromPath(otherFile),
	)
	mgr := openNav(t, dir, "LSP_TEST_TYPEDEF_RESULT="+single)
	res, err := mgr.Query(t.Context(), Query{Op: OpTypeDefinition, File: mainFile, Line: 4, Character: 2})
	require.NoError(t, err)
	require.Len(t, res.Locations, 1)
	assert.Equal(t, "other.go", res.Locations[0].File)
	assert.Equal(t, 3, res.Locations[0].Line)
}

func TestDefinitionCarriesSnippet(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	mgr := openNav(t, dir, "LSP_TEST_DEF_RESULT="+defFixture(uriFromPath(otherFile)))
	res, err := mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 4, Character: 2})
	require.NoError(t, err)
	require.Len(t, res.Locations, 1)
	assert.Equal(t, "func f() {", res.Locations[0].Snippet)
}

func TestSymbolsFileFilteredByQuery(t *testing.T) {
	dir, mainFile, _ := setupWorkspace(t)
	syms := `[{"name":"alpha","kind":12,"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":5}},"selectionRange":{"start":{"line":0,"character":0},"end":{"line":0,"character":5}}},{"name":"beta","kind":12,"range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}},"selectionRange":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}}}]`
	mgr := openNav(t, dir, "LSP_TEST_DOC_SYM_RESULT="+syms)
	res, err := mgr.Query(t.Context(), Query{Op: OpSymbols, File: mainFile, Query: "bet"})
	require.NoError(t, err)
	require.Len(t, res.Symbols, 1)
	assert.Equal(t, "beta", res.Symbols[0].Name)
}

func TestValidateQueryTolerantMatrix(t *testing.T) {
	valid := []Query{
		{Op: OpReferences, File: "/w/a.go", Symbol: "F", Line: 3, Character: 7},
		{Op: OpDefinition, File: "/w/a.go", Symbol: "F", Line: 3},
		{Op: OpImplementations, Symbol: "F"},
		{Op: OpSymbols, File: "/w/a.go", Query: "F"},
	}
	for _, q := range valid {
		assert.NoError(t, validateQuery(q), "%+v", q)
	}
	invalid := []struct {
		q    Query
		want string
	}{
		{Query{Op: OpReferences}, "references requires symbol or file with line+character"},
		{Query{Op: OpReferences, File: "/w/a.go"}, "references requires symbol or line+character"},
		{Query{Op: OpReferences, File: "/w/a.go", Line: 3}, "references with line alone needs character or symbol"},
		{Query{Op: OpReferences, File: "/w/a.go", Character: 3}, "references: character requires line"},
		{Query{Op: OpHover, Symbol: "F", Line: 2}, "hover: line requires file"},
	}
	for _, tt := range invalid {
		err := validateQuery(tt.q)
		require.Error(t, err, "%+v", tt.q)
		assert.Contains(t, err.Error(), tt.want)
	}
}

func TestOccurrenceColumn(t *testing.T) {
	col, ok := occurrenceColumn("\tf()", "f")
	require.True(t, ok)
	assert.Equal(t, 2, col)

	// Embedded in a longer identifier: not an occurrence.
	_, ok = occurrenceColumn("func main() {", "f")
	assert.False(t, ok)

	// Qualified names search for their last segment.
	col, ok = occurrenceColumn("\tt.m()", "T.m")
	require.True(t, ok)
	assert.Equal(t, 4, col)

	_, ok = occurrenceColumn("prefix_f()", "f")
	assert.False(t, ok)
}
