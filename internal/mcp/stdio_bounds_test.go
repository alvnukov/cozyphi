package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A server that streams an oversized unterminated frame must fail the call
// with the violated limit named, close the transport, and leave the next
// call working over a fresh process — not exhaust memory growing a line.
func TestStdioOversizedFrameFailsClosedAndRecovers(t *testing.T) {
	sess := newFakeStdioSession(t, "10s")
	require.NoError(t, sess.Initialize(t.Context()))

	_, err := sess.CallTool(t.Context(), "bigframe", nil)
	require.ErrorIs(t, err, errTransportDead)
	assert.Contains(t, err.Error(), "frame exceeds")
	assert.Contains(t, err.Error(), `"fake"`, "the error must name the server")
	assert.NotContains(t, err.Error(), "xxxx", "the error must not echo the payload")

	got, err := sess.CallTool(t.Context(), "echo", map[string]any{"message": "after"})
	require.NoError(t, err, "next session must recover over a fresh transport")
	assert.Equal(t, "echo:after", got)
}

// A bounded frame nested far past encoding/json's depth limit is a framing
// violation: the transport dies closed and the next call recovers.
func TestStdioDeeplyNestedFrameFailsClosedAndRecovers(t *testing.T) {
	sess := newFakeStdioSession(t, "10s")
	require.NoError(t, sess.Initialize(t.Context()))

	_, err := sess.CallTool(t.Context(), "deepnest", nil)
	require.ErrorIs(t, err, errTransportDead)
	assert.Contains(t, err.Error(), "parse response")

	got, err := sess.CallTool(t.Context(), "echo", map[string]any{"message": "after"})
	require.NoError(t, err)
	assert.Equal(t, "echo:after", got)
}

// A flood of notifications is skipped within the per-frame bound; the real
// answer still arrives.
func TestStdioNotificationFloodIsSkipped(t *testing.T) {
	sess := newFakeStdioSession(t, "10s")
	require.NoError(t, sess.Initialize(t.Context()))

	got, err := sess.CallTool(t.Context(), "notiflood", nil)
	require.NoError(t, err)
	assert.Equal(t, "correct", got)
}

// The on-disk stderr log must stay bounded across sessions: once it would
// pass the cap it is rewritten with the newest tail only.
func TestWriteBoundedLogKeepsDiskFinite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	tail := strings.Repeat("e", 400*1024) // proc's in-memory tail cap is 64 KiB; use the seam directly

	for range 3 {
		writeBoundedLog(path, tail, 1<<20)
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(data), 1<<20, "the log must never pass the cap")
	assert.Equal(t, tail, string(data), "past the cap only the newest tail survives")
}

func TestWriteBoundedLogEmptyTailNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	writeBoundedLog(path, "", 1<<20)
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "an empty tail must not create the file")
}
