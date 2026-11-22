package tools_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tools"
)

// Consume the same JSON a Code Mode cell sees. An edit must accept real
// successors, and a view read must not leak a grant.
func TestStructuredReadEditAndSuccessor(t *testing.T) {
	t.Chdir(t.TempDir())
	registry := tools.NewRegistry(tools.DefaultTools())
	call := func(name string, args, dst any) {
		t.Helper()
		raw, err := json.Marshal(args)
		require.NoError(t, err)
		result, err := registry[name].Run(t.Context(), raw)
		require.NoError(t, err)
		data, err := json.Marshal(result.Structured)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, dst))
	}
	type mutation struct {
		Path    string   `json:"path"`
		Hash    string   `json:"hash"`
		Anchors []string `json:"anchors"`
	}
	var wrote mutation
	call("write", map[string]any{"path": "a.txt", "content": "one\ntwo\n"}, &wrote)
	require.NotEmpty(t, wrote.Anchors)
	require.NotEmpty(t, wrote.Hash)
	var edited mutation
	call("edit", map[string]any{
		"path": wrote.Path, "hash": wrote.Hash,
		"edits": []map[string]string{{"from": wrote.Anchors[0], "to": wrote.Anchors[0], "content": "new"}},
	}, &edited)
	require.NotEmpty(t, edited.Anchors)
	assert.NotEqual(t, wrote.Hash, edited.Hash)
	var page struct {
		Hash  string
		Lines []struct {
			Number int
			Anchor string
			Text   string
		}
		NextOffset int
	}
	call("read", map[string]any{"path": "a.txt", "limit": 1}, &page)
	require.Len(t, page.Lines, 1)
	assert.Equal(t, 1, page.Lines[0].Number)
	assert.Equal(t, "new", page.Lines[0].Text)
	assert.Equal(t, 2, page.NextOffset)
	assert.Empty(t, page.Hash)
	assert.Empty(t, page.Lines[0].Anchor)
	call("read", map[string]any{"path": "a.txt", "mode": "edit", "limit": 1}, &page)
	assert.Equal(t, edited.Anchors[0], page.Lines[0].Anchor)
	assert.Equal(t, edited.Hash, page.Hash)
}

func TestStructuredGrepRespectsVisibleAnchors(t *testing.T) {
	t.Chdir(t.TempDir())
	line := strings.Repeat("x", 500)
	require.NoError(t, os.WriteFile("many.txt", []byte(strings.Repeat(line+"\n", 200)), 0o600))
	result, err := tools.NewRegistry(tools.DefaultTools())["grep"].Run(
		t.Context(),
		json.RawMessage(`{"pattern":"xx","path":"many.txt","limit":200}`),
	)
	if err != nil && strings.Contains(err.Error(), "ripgrep (rg) is not available") {
		t.Skip(err)
	}
	require.NoError(t, err)
	raw, err := json.Marshal(result.Structured)
	require.NoError(t, err)
	var data struct {
		Matches []struct {
			Number             int
			Anchor, Hash, Text string
		}
		Truncated bool
	}
	require.NoError(t, json.Unmarshal(raw, &data))
	require.True(t, data.Truncated)
	require.NotEmpty(t, data.Matches)
	require.Less(t, len(data.Matches), 200)
	for _, m := range data.Matches {
		assert.Contains(t, result.Content, ">>"+m.Anchor+"|")
		assert.Equal(t, line, m.Text)
	}
	assert.Equal(t, strings.Count(result.Content, ">>"), len(data.Matches))
}
