package transcript

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
