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

func TestBuildValidationMatrix(t *testing.T) {
	ctx := tooldef.WithCwd(t.Context(), t.TempDir())

	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{"definition missing file", `{"op":"definition","line":1,"character":1}`, "definition requires file"},
		{
			"definition missing position",
			`{"op":"definition","file":"a.go"}`,
			"definition requires symbol or line+character",
		},
		{
			"definition symbol plus position",
			`{"op":"definition","file":"a.go","symbol":"F","line":1,"character":1}`,
			"definition requires symbol or line+character, not both",
		},
		{"languages rejects file", `{"op":"languages","file":"a.go"}`, "languages takes no target fields"},
		{
			"include_declaration only references",
			`{"op":"definition","file":"a.go","line":1,"character":1,"include_declaration":true}`,
			"include_declaration applies only to references",
		},
		{
			"calls requires direction",
			`{"op":"calls","file":"a.go","line":1,"character":1}`,
			"calls requires direction incoming|outgoing",
		},
		{
			"symbols rejects file and query",
			`{"op":"symbols","file":"a.go","query":"F"}`,
			"symbols accepts file or query, not both",
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
