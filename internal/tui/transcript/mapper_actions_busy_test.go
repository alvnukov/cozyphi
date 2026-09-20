package transcript

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
)

// settledSnapshot is one finished turn: nothing is streaming, so the snapshot
// on its own would say the strips may act.
func settledSnapshot() session.Snapshot {
	return session.Snapshot{
		Messages: []session.Message{
			{ID: "u1", Role: session.RoleUser, State: session.StateComplete, Text: "hello"},
			{
				ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
				Content: []session.ContentBlock{{Type: session.BlockText, Text: "hi"}},
			},
		},
	}
}

// The last chunk landing in the feed is not the end of the turn: the loop is
// still writing the answer to the log, and a queued prompt may be waiting
// behind it. A strip that lit up there would refuse every click, so it asks
// the shell as well as the snapshot.
func TestTheStripStaysDimWhileTheShellSaysARunIsInFlight(t *testing.T) {
	m := NewMapper(components.DefaultTheme(), nil, nil)
	running := true
	m.SetRunActive(func() bool { return running })

	m.refreshActionContext(settledSnapshot())
	assert.Equal(t, actionsBusyHint, m.actionsBusy,
		"the snapshot is quiet but the shell says the run is not finished")

	running = false
	m.refreshActionContext(settledSnapshot())
	assert.Empty(t, m.actionsBusy, "once the shell is idle too the buttons act")
}

// With nothing wired the strips follow the snapshot, so a mapper built
// outside the shell behaves as it did before.
func TestWithoutTheShellTheStripFollowsTheSnapshot(t *testing.T) {
	m := NewMapper(components.DefaultTheme(), nil, nil)

	m.refreshActionContext(settledSnapshot())
	assert.Empty(t, m.actionsBusy)

	streaming := settledSnapshot()
	streaming.Messages[1].State = session.StateStreaming
	m.refreshActionContext(streaming)
	assert.Equal(t, actionsBusyHint, m.actionsBusy)
}

// Asking which rows may be cut at walks the session path under the lock
// appends hold while they write to disk. The tail pass runs on roughly every
// frame of a streaming turn, and it has no use for a fresh answer: the row it
// patches is the reply still arriving, which is no boundary while it streams.
// So it must not ask.
func TestTheTailPassNeverAsksTheSession(t *testing.T) {
	m := NewMapper(components.DefaultTheme(), nil, nil)
	asked := 0
	m.SetMessageActions(func(string) {}, func(string) {}, func(string) {},
		func() map[string]struct{} {
			asked++
			return map[string]struct{}{"u1": {}, "a1": {}}
		})

	streaming := func(text string) session.Snapshot {
		return session.Snapshot{Messages: []session.Message{
			{ID: "u1", Role: session.RoleUser, State: session.StateComplete, Text: "go"},
			{
				ID: "a1", Role: session.RoleAssistant, State: session.StateStreaming,
				Content: []session.ContentBlock{{Type: session.BlockText, Text: text}},
			},
		}}
	}

	entries, ids, _ := m.Sync(nil, nil, streaming("one"))
	require.Equal(t, 1, asked, "the full pass asks once")

	for _, text := range []string{"one two", "one two three", "one two three four"} {
		_, ok := m.syncTail(entries, ids, streaming(text))
		require.True(t, ok, "the tail pass handled the growing reply")
	}
	assert.Equal(t, 1, asked, "however many chunks arrive, the session is asked no more")

	// The next full pass picks the answer up again, so nothing goes stale.
	_, _, _ = m.Sync(entries, ids, streaming("one two three four"))
	assert.Equal(t, 2, asked)
}
