package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
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
	canonical, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return canonical
}

func TestResolveTUIResumeSession(t *testing.T) {
	dir := t.TempDir()
	old := writeTUISession(t, dir, "aaaa1111", time.Now().Add(-2*time.Hour))
	newest := writeTUISession(t, dir, "bbbb2222", time.Now())

	m, err := resolveTUIResumeSession(tuiOptions{}, dir)
	require.NoError(t, err)
	require.Nil(t, m)

	latest, err := resolveTUIResumeSession(tuiOptions{continueLast: true}, dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, latest.Close()) })
	assert.Equal(t, newest, latest.File())
	_, err = session.OpenSession(newest)
	require.ErrorIs(t, err, session.ErrBusy, "selection must retain ownership")

	older, err := resolveTUIResumeSession(tuiOptions{continueLast: true}, dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, older.Close()) })
	assert.Equal(t, old, older.File(), "continue skips the latest active session")

	m, err = resolveTUIResumeSession(tuiOptions{continueLast: true}, dir)
	require.NoError(t, err)
	require.Nil(t, m, "all busy means start fresh")
	_, err = resolveTUIResumeSession(tuiOptions{resume: "aaaa"}, dir)
	require.ErrorIs(t, err, session.ErrBusy, "explicit busy ID is not a fresh session")

	require.NoError(t, older.Close())
	for _, id := range []string{"aaaa1111", "aaaa"} {
		m, err = resolveTUIResumeSession(tuiOptions{resume: id}, dir)
		require.NoError(t, err)
		assert.Equal(t, old, m.File())
		require.NoError(t, m.Close())
	}
}

func TestResolveTUIResumeSessionErrorsAndNoHistory(t *testing.T) {
	dir := t.TempDir()
	writeTUISession(t, dir, "aaaa1111", time.Now())
	writeTUISession(t, dir, "aaaa2222", time.Now())
	_, err := resolveTUIResumeSession(tuiOptions{resume: "aaaa"}, dir)
	require.ErrorContains(t, err, "ambiguous")
	_, err = resolveTUIResumeSession(tuiOptions{resume: "zzzz"}, dir)
	require.ErrorContains(t, err, "not found")
	for _, dir := range []string{t.TempDir(), filepath.Join(t.TempDir(), "missing")} {
		m, err := resolveTUIResumeSession(tuiOptions{continueLast: true}, dir)
		require.NoError(t, err)
		require.Nil(t, m)
	}
}
