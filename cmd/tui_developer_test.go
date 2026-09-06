package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUIDeveloperModeComesFromTheCommandLine(t *testing.T) {
	opts, err := parseTUIArgs([]string{"--developer-mode"})
	require.NoError(t, err)
	assert.True(t, opts.developerMode)

	// `cozyphi --developer-mode --resume ID` is one process: the capability
	// and the session to open are independent choices.
	opts, err = parseTUIArgs([]string{"--developer-mode", "--resume", "abc123"})
	require.NoError(t, err)
	assert.True(t, opts.developerMode)
	assert.Equal(t, "abc123", opts.resume)

	opts, err = parseTUIArgs([]string{"-c"})
	require.NoError(t, err)
	assert.False(t, opts.developerMode)
}

func TestTUIDeveloperModeIsNotSpelledLoosely(t *testing.T) {
	// The flag is a bare switch, as it is for `run`. A typo fails the start
	// instead of quietly not granting what the user asked for.
	for _, args := range [][]string{
		{"--developer-mode=true"},
		{"--developer_mode"},
		{"--developer"},
		{"--developer-modes"},
		{"-developer-mode"},
	} {
		_, err := parseTUIArgs(args)
		assert.Error(t, err, "args %v", args)
	}
}

func TestTUIDeveloperModeIgnoresTheEnvironment(t *testing.T) {
	for _, name := range []string{
		"COZYPHI_DEVELOPER_MODE", "COZYPHI_DEVELOPER", "COZYPHI_DEBUG", "DEVELOPER_MODE",
	} {
		t.Setenv(name, "1")
	}

	opts, err := parseTUIArgs(nil)
	require.NoError(t, err)
	assert.False(t, opts.developerMode, "no environment variable grants the capability")
}

func TestTUIUsageAnnouncesDeveloperMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.txt")
	file, err := os.Create(path)
	require.NoError(t, err)
	printTUIUsage(file)
	require.NoError(t, file.Close())

	usage, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(usage), "--developer-mode")
	assert.Contains(t, string(usage), "read-only")
}
