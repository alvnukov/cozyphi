package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAutonomousAndUnknownTurnsPreserveWebTaint(t *testing.T) {
	for _, origin := range []TurnOrigin{TurnAutonomous, ""} {
		t.Run(string(origin), func(t *testing.T) {
			eng := testEngine(t, EngineOpts{})
			eng.turnWeb.mark()
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			for _, err := range eng.Loop(ctx, "nonempty background notification", LoopOpts{Origin: origin}) {
				if err != nil {
					assert.ErrorIs(t, err, context.Canceled)
				}
			}
			assert.True(t, eng.turnWeb.Tainted(), "an event cannot masquerade as new human permission")
		})
	}
}

func TestActualUserOriginResetsTaintRegardlessOfPromptText(t *testing.T) {
	eng := testEngine(t, EngineOpts{})
	eng.turnWeb.mark()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, err := range eng.Loop(ctx, "", LoopOpts{Origin: TurnUserInput}) {
		if err != nil {
			assert.ErrorIs(t, err, context.Canceled)
		}
	}
	assert.False(t, eng.turnWeb.Tainted(), "origin is factual dispatch metadata, not a text heuristic")
}
