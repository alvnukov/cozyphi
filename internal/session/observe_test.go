package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// A persisting session takes its file the moment it opens and writes nothing
// until its first assistant turn. The gap between the two is exactly what a
// reader mistakes for a lost conversation, so the observation keeps them
// apart rather than collapsing both into "on disk".
func TestABoundTranscriptIsReportedBeforeAnythingIsWrittenToIt(t *testing.T) {
	dir := t.TempDir()
	manager, err := newTestSessionManager(t, dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	held := Observe(manager, dir)
	assert.True(t, held.Known)
	assert.True(t, held.Persisting)
	assert.True(t, held.Bound, "the file is taken when the session opens")
	assert.False(t, held.Written)
	assert.True(t, held.Local)
	assert.Equal(t, 1, held.Entries, "the session header is an entry like any other")

	_, err = manager.Append(llm.Message{Role: llm.RoleUser, Content: "hello"})
	require.NoError(t, err)
	assert.False(t, Observe(manager, dir).Written, "a session flushes on its first assistant turn")

	_, err = manager.Append(llm.Message{Role: llm.RoleAssistant, Content: "hi"})
	require.NoError(t, err)

	written := Observe(manager, dir)
	assert.True(t, written.Written)
	assert.Equal(t, 3, written.Entries)
	assert.NotEqual(t, held.Revision, written.Revision, "and the two states are told apart")
}

// A session opened without persistence ends with the process. It is not a
// session whose file is merely empty, and nothing about it may read as one.
func TestAnEphemeralSessionHoldsNoFileAtAll(t *testing.T) {
	dir := t.TempDir()
	manager, err := newTestSessionManager(t, dir, WithSessionDir(dir), WithShouldFlush(false))
	require.NoError(t, err)
	_, err = manager.Append(llm.Message{Role: llm.RoleAssistant, Content: "hi"})
	require.NoError(t, err)

	facts := Observe(manager, dir)
	assert.True(t, facts.Known)
	assert.False(t, facts.Persisting)
	assert.False(t, facts.Bound)
	assert.False(t, facts.Written)
	assert.False(t, facts.Local, "there is no file, so it is in no directory")
	assert.Equal(t, 2, facts.Entries, "the conversation is still real, it is just not kept")
}

// A transcript resumed from a path someone named sits wherever they named,
// and the view must not point at a directory the file is not in. The
// comparison is on the directory below the base: a session file is
// canonicalized when taken, so the prefixes need not agree even for a file
// that is exactly where it belongs.
func TestATranscriptOutsideThisWorkspacesDirectoryIsReportedAsElsewhere(t *testing.T) {
	own, elsewhere := t.TempDir(), t.TempDir()
	manager, err := newTestSessionManager(t, own, WithSessionDir(own), WithShouldFlush(true))
	require.NoError(t, err)

	assert.True(t, Observe(manager, own).Local)
	assert.False(t, Observe(manager, elsewhere).Local,
		"another workspace's directory does not hold this workspace's transcript")

	// The layout's own spelling of the directory, with a symlinked prefix
	// resolved out of the file's, still names one workspace.
	assert.True(t, Observe(manager, own+string(filepath.Separator)).Local)
	assert.False(t, Observe(manager, "").Local, "and no directory at all is not a match")
}

// Nothing that could carry a secret is copied out. A transcript is the one
// place in the process where every message the user typed is held, so this
// is the observation with the most to leak and the least that may.
func TestNoMessageTitleOrPathReachesTheObservation(t *testing.T) {
	const secret = "sk-live-SESSION-SENTINEL"
	dir := t.TempDir()
	manager, err := newTestSessionManager(t, dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, err = manager.Append(llm.Message{Role: llm.RoleUser, Content: "the key is " + secret})
	require.NoError(t, err)
	_, err = manager.Append(llm.Message{Role: llm.RoleAssistant, Content: "noted"})
	require.NoError(t, err)

	facts := Observe(manager, dir)
	rendered := fmt.Sprintf("%+v", facts)
	assert.NotContains(t, rendered, secret)
	assert.NotContains(t, rendered, manager.sessionFile, "not even the path it is written to")
	assert.NotContains(t, rendered, manager.sessionID)
	assert.NotContains(t, rendered, dir)
}

// Reading is reading: no entry is appended, no flush forced, and the file is
// neither read, listed nor stat'd — so a question about the transcript
// cannot change what the next turn writes to it.
func TestObservingATranscriptAppendsNothingAndFlushesNothing(t *testing.T) {
	dir := t.TempDir()
	manager, err := newTestSessionManager(t, dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, err = manager.Append(llm.Message{Role: llm.RoleAssistant, Content: "hi"})
	require.NoError(t, err)

	before, err := os.ReadFile(manager.sessionFile)
	require.NoError(t, err)
	first := Observe(manager, dir)

	for range 3 {
		assert.Equal(t, first, Observe(manager, dir))
	}

	after, err := os.ReadFile(manager.sessionFile)
	require.NoError(t, err)
	assert.Equal(t, before, after, "the transcript is byte for byte what it was")
	assert.Len(t, manager.entries, 2)
}

// A manager nobody built is a wiring gap, and the layer above turns that
// into "not known" rather than into an empty transcript.
func TestObservingNoManagerAtAllKnowsNothing(t *testing.T) {
	facts := Observe(nil, t.TempDir())
	assert.False(t, facts.Known)
	assert.Zero(t, facts.Entries)
	assert.Empty(t, facts.Revision)
}

// The tracker is the manager's own and is never handed out. What leaves is
// the mechanism — this session counts the plan or it does not, and the
// schema is this wide — never a counter: what the counters say is the plan
// category's answer, and repeating it here would be two answers to one
// question.
func TestTelemetryIsReportedAsAMechanismAndNeverAsACounter(t *testing.T) {
	dir := t.TempDir()
	manager, err := newTestSessionManager(t, dir, WithSessionDir(dir))
	require.NoError(t, err)

	facts := ObserveTelemetry(manager)
	assert.True(t, facts.Known)
	assert.True(t, facts.Tracked, "a session built the usual way counts the plan")
	assert.Positive(t, facts.Counters, "and the whole schema is what is being counted")
	assert.NotEmpty(t, facts.Readable, "the counters can be read in one place inside this process")

	manager.telemetry = nil
	assert.False(t, ObserveTelemetry(manager).Tracked,
		"a manager without a tracker is telemetry switched off, not a fault")
}

// A manager nobody built is telemetry switched off on the same terms every
// recording method here already takes a nil manager for.
func TestObservingTelemetryWithNoManagerAtAllIsTelemetryOff(t *testing.T) {
	var absent *Manager
	facts := ObserveTelemetry(absent)
	assert.True(t, facts.Known)
	assert.False(t, facts.Tracked)
	assert.Empty(t, facts.Readable)
}
