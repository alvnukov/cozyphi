package agent

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/session"
)

// TestLoopStampsTurnMetadata: every assistant event carries the model id and
// the round start; the terminal event closes the timing span. Streaming
// deltas never set an end.
func TestLoopStampsTurnMetadata(t *testing.T) {
	server, _ := fakeToolSequenceServer(0)
	defer server.Close()

	var runs atomic.Int32
	engine := newRoundTestEngine(t, server.URL, &runs)

	var streaming, complete session.Message
	for ev, err := range engine.Loop(t.Context(), "go", LoopOpts{}) {
		require.NoError(t, err)
		if update, ok := ev.(session.AssistantMessageUpdate); ok {
			switch update.Message.State {
			case session.StateStreaming:
				streaming = update.Message
			case session.StateComplete:
				complete = update.Message
			}
		}
	}
	require.Equal(t, "fake", complete.Model, "model id on the terminal event")
	require.False(t, complete.Started.IsZero(), "round start stamped")
	require.False(t, complete.Ended.IsZero(), "round end stamped")
	require.False(t, complete.Ended.Before(complete.Started), "end after start")
	require.Equal(t, "fake", streaming.Model, "model id on streaming deltas")
	require.Equal(t, complete.Started, streaming.Started, "streaming deltas share the round start")
	require.True(t, streaming.Ended.IsZero(), "no end while streaming")
}
