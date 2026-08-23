package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTUIArgs(t *testing.T) {
	opts, err := parseTUIArgs(nil)
	require.NoError(t, err)
	assert.False(t, opts.continueLast)
	assert.Empty(t, opts.resume)
	assert.False(t, opts.help)

	opts, err = parseTUIArgs([]string{"-c"})
	require.NoError(t, err)
	assert.True(t, opts.continueLast)

	opts, err = parseTUIArgs([]string{"--continue"})
	require.NoError(t, err)
	assert.True(t, opts.continueLast)

	opts, err = parseTUIArgs([]string{"--resume", "abc123"})
	require.NoError(t, err)
	assert.False(t, opts.continueLast)
	assert.Equal(t, "abc123", opts.resume)

	opts, err = parseTUIArgs([]string{"--resume=abc123"})
	require.NoError(t, err)
	assert.Equal(t, "abc123", opts.resume)

	opts, err = parseTUIArgs([]string{"-h"})
	require.NoError(t, err)
	assert.True(t, opts.help)
}

func TestParseTUIArgs_Rejects(t *testing.T) {
	for _, args := range [][]string{
		{"-c", "--resume", "abc"},
		{"--resume", "--continue"},
		{"--resume"},             // missing value
		{"--yolo"},               // run-only flag
		{"extra"},                // positional
		{"-c", "extra"},          // positional after flags
		{"--resume=abc", "xtra"}, // positional after flags
	} {
		_, err := parseTUIArgs(args)
		require.Error(t, err, "args: %v", args)
	}
}

// writeTUISession persists a minimal session file named for its id and returns
// its path. mtime decides --continue ordering (newest first).
func writeTUISession(t *testing.T, dir, id string, mtime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, "s_"+id+".jsonl")
	line := fmt.Sprintf(
		`{"type":"EntrySession","id":%q,"timestamp":"2026-08-23T12:00:00Z","cwd":"/tmp"}`+"\n", id)
	require.NoError(t, os.WriteFile(path, []byte(line), 0o644))
	require.NoError(t, os.Chtimes(path, mtime, mtime))
	return path
}

func TestResolveTUIResumePath(t *testing.T) {
	dir := t.TempDir()
	old := writeTUISession(t, dir, "aaaa1111", time.Now().Add(-2*time.Hour))
	newest := writeTUISession(t, dir, "bbbb2222", time.Now())

	// No target: start a new session.
	path, err := resolveTUIResumePath(tuiOptions{}, dir)
	require.NoError(t, err)
	assert.Empty(t, path)

	// --continue picks the newest session for the directory.
	path, err = resolveTUIResumePath(tuiOptions{continueLast: true}, dir)
	require.NoError(t, err)
	assert.Equal(t, newest, path)
	assert.NotEqual(t, old, path)

	// --resume accepts an exact id and a unique prefix.
	path, err = resolveTUIResumePath(tuiOptions{resume: "aaaa1111"}, dir)
	require.NoError(t, err)
	assert.Equal(t, old, path)

	path, err = resolveTUIResumePath(tuiOptions{resume: "aaaa"}, dir)
	require.NoError(t, err)
	assert.Equal(t, old, path)
}

func TestResolveTUIResumePath_Errors(t *testing.T) {
	dir := t.TempDir()
	writeTUISession(t, dir, "aaaa1111", time.Now())
	writeTUISession(t, dir, "aaaa2222", time.Now())

	_, err := resolveTUIResumePath(tuiOptions{resume: "aaaa"}, dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")

	_, err = resolveTUIResumePath(tuiOptions{resume: "zzzz"}, dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	empty := t.TempDir()
	_, err = resolveTUIResumePath(tuiOptions{continueLast: true}, empty)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no sessions")
	assert.Contains(t, err.Error(), empty)
}
