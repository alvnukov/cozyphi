package greptool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/util"
)

func TestRunGrep_CwdRelativeHeaders(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\nfunc Hello() {}\n"), 0o644))
	t.Chdir(root)

	raw, err := json.Marshal(grepInput{Pattern: "Hello", Path: "src"})
	require.NoError(t, err)
	out, err := runGrep(t.Context(), raw)
	if err != nil && strings.Contains(err.Error(), "ripgrep") {
		t.Skip(err.Error())
	}
	require.NoError(t, err)
	assert.Contains(t, out.Content, "@file src/main.go#")
	assert.Contains(t, out.Content, "src/main.go:>>")
	assert.NotContains(t, out.Content, "@file main.go#")
}

func TestRunGrep_DefaultPathUsesCwdRelative(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\nfunc Hello() {}\n"), 0o644))
	t.Chdir(root)

	raw, err := json.Marshal(grepInput{Pattern: "Hello"})
	require.NoError(t, err)
	out, err := runGrep(t.Context(), raw)
	if err != nil && strings.Contains(err.Error(), "ripgrep") {
		t.Skip(err.Error())
	}
	require.NoError(t, err)
	assert.Contains(t, out.Content, "@file src/main.go#")
}

func TestGrepDetailNamesPatternAndCounts(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.go"), []byte("Hello\nHello\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.go"), []byte("Hello\n"), 0o644))
	t.Chdir(root)

	raw, err := json.Marshal(grepInput{Pattern: "Hello"})
	require.NoError(t, err)
	out, err := runGrep(t.Context(), raw)
	if err != nil && strings.Contains(err.Error(), "ripgrep") {
		t.Skip(err.Error())
	}
	require.NoError(t, err)
	assert.Equal(t, `"Hello" — 3 matches in 2 files`, out.Detail)

	raw, err = json.Marshal(grepInput{Pattern: "NoSuchThing"})
	require.NoError(t, err)
	out, err = runGrep(t.Context(), raw)
	require.NoError(t, err)
	assert.Equal(t, `"NoSuchThing" — 0 matches`, out.Detail)
}

func TestGrepDetailClipsLongPatterns(t *testing.T) {
	long := strings.Repeat("x", 40)
	got := grepDetail(long, 1, 1)
	assert.Equal(t, `"`+strings.Repeat("x", 31)+`…" — 1 match in 1 file`, got)
}

func TestKeptLineCountFollowsTheOutputCap(t *testing.T) {
	require.Equal(t, 5, keptLineCount("a\nb", false, 5), "untruncated output keeps every rendered line")
	require.Equal(t, 2, keptLineCount("a\nb", true, 5), "a truncated output keeps only what it carries")
	require.Equal(t, 0, keptLineCount("", true, 5))
}

// Anchors the output cap cut must not be authorized: the model never saw them.
func TestReportAnchorsSkipsLinesLostToTruncation(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.go")
	second := filepath.Join(dir, "b.go")
	firstRev, secondRev := util.Revision(0xA1B2), util.Revision(0xC3D4)
	anchors := []outAnchor{
		{},
		{abs: first, rev: firstRev, ref: "1#abc"},
		{abs: first, rev: firstRev, ref: "2#def"},
		{},
		{abs: second, rev: secondRev, ref: "9#ghi"},
	}

	type grant struct {
		path    string
		rev     util.Revision
		anchors []string
	}
	var grants []grant
	sink := func(path string, rev util.Revision, refs []string) {
		grants = append(grants, grant{path: path, rev: rev, anchors: refs})
	}

	reportAnchors(t.Context(), sink, anchors, len(anchors))
	require.Equal(t, []grant{
		{path: first, rev: firstRev, anchors: []string{"1#abc", "2#def"}},
		{path: second, rev: secondRev, anchors: []string{"9#ghi"}},
	}, grants, "one grant per file snapshot, in output order")

	grants = nil
	reportAnchors(t.Context(), sink, anchors, 3)
	require.Equal(t, []grant{{path: first, rev: firstRev, anchors: []string{"1#abc", "2#def"}}}, grants)

	grants = nil
	reportAnchors(t.Context(), nil, anchors, len(anchors))
	require.Empty(t, grants)
}
