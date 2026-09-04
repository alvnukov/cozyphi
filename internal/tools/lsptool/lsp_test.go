package lsptool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

func TestParseRejectsUnknownFields(t *testing.T) {
	_, err := parse(json.RawMessage(`{"op":"definition","file":"a.go","line":1,"character":1,"bogus":1}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

func TestParseAcceptsPlanStep(t *testing.T) {
	in, err := parse(json.RawMessage(`{"op":"languages","plan_step":1}`))
	require.NoError(t, err)
	assert.Equal(t, "languages", in.Op)

	// The v2 form is the stable step id string; both must decode.
	_, err = parse(json.RawMessage(`{"op":"languages","plan_step":"wire-schema"}`))
	require.NoError(t, err)
}

func TestBuildValidationMatrix(t *testing.T) {
	ctx := tooldef.WithCwd(t.Context(), t.TempDir())

	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{"position without file", `{"op":"definition","line":1,"character":1}`, "definition: line requires file"},
		{
			"definition missing position",
			`{"op":"definition","file":"a.go"}`,
			"definition requires symbol or line+character",
		},
		{
			"no target at all",
			`{"op":"references"}`,
			"references requires symbol or file with line+character",
		},
		{
			"line alone without symbol",
			`{"op":"definition","file":"a.go","line":3}`,
			"definition with line alone needs character or symbol",
		},
		{
			"character requires line",
			`{"op":"hover","file":"a.go","character":4}`,
			"hover: character requires line",
		},
		{
			"symbols blank everywhere",
			`{"op":"symbols","file":"","query":"  ","symbol":"x"}`,
			"symbols requires file or query",
		},
		{"diagnostics blank file", `{"op":"diagnostics","file":"  "}`, "diagnostics requires file"},
		{
			"synthetic position without target",
			`{"op":"definition","file":"","line":1,"character":1}`,
			"definition: line requires file",
		},
		{
			"calls rejects a bogus direction",
			`{"op":"calls","file":"a.go","line":1,"character":1,"direction":"sideways"}`,
			"calls requires direction incoming|outgoing",
		},
		{"unknown operation", `{"op":"nope"}`, "unknown operation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, err := parse(json.RawMessage(tt.raw))
			require.NoError(t, err)
			_, err = build(ctx, in)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestBuildValidDefinition(t *testing.T) {
	cwd := t.TempDir()
	ctx := tooldef.WithCwd(t.Context(), cwd)
	in, err := parse(json.RawMessage(`{"op":"definition","file":"a.go","line":3,"character":7,"limit":20}`))
	require.NoError(t, err)
	q, err := build(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, lsp.OpDefinition, q.Op)
	assert.Equal(t, 3, q.Line)
	assert.Equal(t, 7, q.Character)
	assert.Equal(t, 20, q.Limit)
	assert.True(t, strings.HasSuffix(q.File, "a.go"))
}

func TestBuildNavigationMatrix(t *testing.T) {
	ctx := tooldef.WithCwd(t.Context(), t.TempDir())

	in, err := parse(json.RawMessage(`{"op":"definition","file":"a.go","symbol":"F"}`))
	require.NoError(t, err)
	q, err := build(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, "F", q.Symbol)

	in, err = parse(json.RawMessage(`{"op":"references","file":"a.go","line":2,"character":3}`))
	require.NoError(t, err)
	q, err = build(ctx, in)
	require.NoError(t, err)
	assert.True(t, q.IncludeDeclaration, "references defaults include_declaration to true")

	in, err = parse(
		json.RawMessage(`{"op":"references","file":"a.go","line":2,"character":3,"include_declaration":false}`),
	)
	require.NoError(t, err)
	q, err = build(ctx, in)
	require.NoError(t, err)
	assert.False(t, q.IncludeDeclaration)

	in, err = parse(json.RawMessage(`{"op":"symbols","query":"Fun"}`))
	require.NoError(t, err)
	q, err = build(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, "Fun", q.Query)

	in, err = parse(json.RawMessage(`{"op":"hover","file":"a.go"}`))
	require.NoError(t, err)
	_, err = build(ctx, in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hover requires symbol or line+character")
}

func TestBuildTolerantTargeting(t *testing.T) {
	ctx := tooldef.WithCwd(t.Context(), t.TempDir())

	// A symbol plus a full position is over-specification, not a conflict.
	in, err := parse(json.RawMessage(`{"op":"definition","file":"a.go","symbol":"F","line":3,"character":7}`))
	require.NoError(t, err)
	q, err := build(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, "F", q.Symbol)
	assert.Equal(t, 3, q.Line)
	assert.Equal(t, 7, q.Character)

	// A symbol plus a bare hint line disambiguates declarations.
	in, err = parse(json.RawMessage(`{"op":"references","file":"a.go","symbol":"F","line":3}`))
	require.NoError(t, err)
	q, err = build(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, 3, q.Line)
	assert.Zero(t, q.Character)

	// A symbol alone needs no file: it resolves workspace-wide.
	in, err = parse(json.RawMessage(`{"op":"implementations","symbol":"Handler"}`))
	require.NoError(t, err)
	q, err = build(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, lsp.OpImplementations, q.Op)
	assert.Empty(t, q.File)

	// calls defaults its direction to incoming.
	in, err = parse(json.RawMessage(`{"op":"calls","file":"a.go","symbol":"F"}`))
	require.NoError(t, err)
	q, err = build(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, lsp.DirectionIncoming, q.Direction)

	// symbols with file and query filters the outline instead of erroring.
	in, err = parse(json.RawMessage(`{"op":"symbols","file":"a.go","query":"F"}`))
	require.NoError(t, err)
	q, err = build(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, "F", q.Query)
	assert.True(t, strings.HasSuffix(q.File, "a.go"))
}

func TestBuildNeutralWireValues(t *testing.T) {
	ctx := tooldef.WithCwd(t.Context(), t.TempDir())

	tests := []struct {
		name string
		raw  string
		want func(t *testing.T, q lsp.Query)
	}{
		{
			// Exact wire shape from a provider binding: every schema property
			// filled, most with neutral filler values.
			name: "symbols with every property filled",
			raw:  `{"op":"symbols","file":"","symbol":"","line":1,"character":1,"query":"build","direction":"incoming","include_declaration":true,"limit":50}`,
			want: func(t *testing.T, q lsp.Query) {
				assert.Equal(t, lsp.OpSymbols, q.Op)
				assert.Equal(t, "build", q.Query)
				assert.Empty(t, q.File)
				assert.Zero(t, q.Line)
				assert.Zero(t, q.Character)
				assert.False(t, q.IncludeDeclaration, "include_declaration is ignored outside references")
				assert.Empty(t, q.Direction)
				assert.Equal(t, 50, q.Limit)
			},
		},
		{
			name: "include_declaration false outside references",
			raw:  `{"op":"definition","symbol":"build","include_declaration":false}`,
			want: func(t *testing.T, q lsp.Query) {
				assert.Equal(t, "build", q.Symbol)
				assert.False(t, q.IncludeDeclaration)
			},
		},
		{
			name: "symbol with synthetic coordinates and blank file",
			raw:  `{"op":"definition","symbol":"build","file":"","line":1,"character":1}`,
			want: func(t *testing.T, q lsp.Query) {
				assert.Equal(t, "build", q.Symbol)
				assert.Empty(t, q.File)
				assert.Zero(t, q.Line, "synthetic coordinates must not become a target")
				assert.Zero(t, q.Character)
			},
		},
		{
			name: "languages ignores every target field",
			raw:  `{"op":"languages","file":"","symbol":"","query":"","line":1,"character":1,"direction":"incoming","include_declaration":false}`,
			want: func(t *testing.T, q lsp.Query) {
				assert.Equal(t, lsp.OpLanguages, q.Op)
				assert.Empty(t, q.File)
				assert.Empty(t, q.Symbol)
				assert.Zero(t, q.Line)
				assert.Zero(t, q.Character)
			},
		},
		{
			name: "calls with blank direction defaults to incoming",
			raw:  `{"op":"calls","symbol":"build","direction":"  "}`,
			want: func(t *testing.T, q lsp.Query) {
				assert.Equal(t, "build", q.Symbol)
				assert.Equal(t, lsp.DirectionIncoming, q.Direction)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, err := parse(json.RawMessage(tt.raw))
			require.NoError(t, err)
			q, err := build(ctx, in)
			require.NoError(t, err)
			tt.want(t, q)
		})
	}
}

// TestToolRunTranscriptPayload replays the exact failed call from session
// 2026-09-04T18-29-35 through the executor path: decode, build, query.
func TestToolRunTranscriptPayload(t *testing.T) {
	ctx := tooldef.WithCwd(t.Context(), t.TempDir())
	var got lsp.Query
	query := func(_ context.Context, q lsp.Query) (lsp.Result, error) {
		got = q
		return lsp.Result{Symbols: []lsp.Symbol{{Name: "build", Kind: "function"}}}, nil
	}
	tool := Tool(query)
	raw := `{"op":"symbols","file":"","symbol":"","line":1,"character":1,"query":"build","direction":"incoming","include_declaration":true,"limit":50}`
	_, err := tool.Run(ctx, json.RawMessage(raw))
	require.NoError(t, err)
	assert.Equal(t, lsp.OpSymbols, got.Op)
	assert.Equal(t, "build", got.Query)
	assert.Zero(t, got.Line)
	assert.Zero(t, got.Character)
	assert.False(t, got.IncludeDeclaration)
	assert.Empty(t, got.Direction)
	assert.Equal(t, 50, got.Limit)
}

// The provider-facing schema must stay optional-everything-but-op: a
// binding that fills every property is valid, so nothing else may be
// required.
func TestSchemaRequiresOnlyOp(t *testing.T) {
	def := Tool(nil).Definition
	require.NotNil(t, def.Params)
	blob, err := json.Marshal(def.Params)
	require.NoError(t, err)
	var schema struct {
		Type       string         `json:"type"`
		Required   []string       `json:"required"`
		Properties map[string]any `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(blob, &schema))
	assert.Equal(t, []string{"op"}, schema.Required)
	assert.Len(t, schema.Properties, 9)
}

func TestToolRunEndToEnd(t *testing.T) {
	cwd := t.TempDir()
	ctx := tooldef.WithCwd(t.Context(), cwd)
	called := false
	query := func(_ context.Context, q lsp.Query) (lsp.Result, error) {
		called = true
		assert.Equal(t, lsp.OpDefinition, q.Op)
		return lsp.Result{Locations: []lsp.Location{
			{File: "other.go", Line: 3, Character: 6, EndLine: 3, EndCharacter: 7},
		}}, nil
	}
	tool := Tool(query)
	require.Equal(t, "lsp", tool.Definition.Name)

	res, err := tool.Run(ctx, json.RawMessage(`{"op":"definition","file":"a.go","line":3,"character":7}`))
	require.NoError(t, err)
	assert.True(t, called)
	assert.Contains(t, res.Content, "definition: 1 location(s)")
	assert.Contains(t, res.Content, "other.go:3:6-3:7")
	assert.Contains(t, res.Detail, "definition ")
	assert.Contains(t, res.Detail, "a.go")
}

func TestRenderDiagnosticsStatus(t *testing.T) {
	out := render(lsp.OpDiagnostics, lsp.Result{Status: lsp.StatusPending})
	assert.Contains(t, out, "diagnostics: none (pending)")

	res := lsp.Result{
		Status: lsp.StatusFresh,
		Diagnostics: []lsp.Diagnostic{{
			Severity: "error", Message: "boom", File: "a.go", Line: 2, Character: 3,
		}},
	}
	out = render(lsp.OpDiagnostics, res)
	assert.Contains(t, out, "diagnostics: 1 result(s) (fresh)")
	assert.Contains(t, out, "error: boom @ a.go:2:3")
}

func TestRenderLanguages(t *testing.T) {
	res := lsp.Result{Languages: []lsp.Language{{
		Language: "go", Server: "gopls", Configured: true,
		Operations:  []string{"definition", "hover"},
		InstallHint: "go install golang.org/x/tools/gopls@latest",
	}}}
	out := render(lsp.OpLanguages, res)
	assert.Contains(t, out, "go/gopls configured=true installed=false running=false roots=0")
	assert.Contains(t, out, "operations: definition,hover")
	assert.Contains(t, out, "install: go install golang.org/x/tools/gopls@latest")

	running := lsp.Result{Languages: []lsp.Language{{
		Language: "go", Server: "gopls", Configured: true, Installed: true,
		Running: true, ActiveRoots: 2, Error: "boom",
	}}}
	out = render(lsp.OpLanguages, running)
	assert.Contains(t, out, "go/gopls configured=true installed=true running=true roots=2")
	assert.Contains(t, out, "error: boom")
}

func TestRenderLocationSnippets(t *testing.T) {
	res := lsp.Result{Locations: []lsp.Location{
		{File: "a.go", Line: 3, Character: 1, EndLine: 3, EndCharacter: 2, Snippet: "func f() {"},
	}}
	out := render(lsp.OpImplementations, res)
	assert.Contains(t, out, "implementations: 1 location(s)")
	assert.Contains(t, out, "a.go:3:1-3:2\tfunc f() {")
}

func TestRenderLocationOpFallsBackToSymbols(t *testing.T) {
	// A workspace-wide resolution can answer a location op with candidate
	// symbols; they must render qualified so the model can requalify.
	res := lsp.Result{
		Symbols: []lsp.Symbol{{
			Name: "f", Kind: "function", Container: "pkg",
			Location: lsp.Location{File: "a.go", Line: 3, Character: 1},
		}},
		Warnings: []string{`symbol "f" has 2 declarations in the workspace; pass file to pick one`},
	}
	out := render(lsp.OpDefinition, res)
	assert.Contains(t, out, "pkg.f (function) @ a.go:3:1")
	assert.Contains(t, out, "warning: symbol")
}

func TestRenderBoundOutput(t *testing.T) {
	res := lsp.Result{Locations: []lsp.Location{
		{File: "a.go", Line: 1, Character: 1, EndLine: 1, EndCharacter: 2},
	}}
	out := render(lsp.OpDefinition, res)
	assert.Contains(t, out, "a.go:1:1-1:2")

	big := lsp.Result{Warnings: make([]string, 1000)}
	for i := range big.Warnings {
		big.Warnings[i] = strings.Repeat("a", 50)
	}
	out = render(lsp.OpDefinition, big)
	assert.LessOrEqual(t, len(out), lsp.MaxOutputBytes+len("\n... truncated"))
}
