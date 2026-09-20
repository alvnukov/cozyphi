package agent

import (
	"errors"

	"github.com/alvnukov/cozyphi/internal/session"
)

// ErrForkWhileTurnRunning refuses a fork while inference or a tool is still
// in flight. The copy is taken from the branch as it stands, and half a round
// is not a branch: the anchor the user clicked is not the end of the
// conversation the model is still writing.
var ErrForkWhileTurnRunning = errors.New("cannot fork the session while a turn is running")

// Fork writes the branch up to the anchor as a new session file beside this
// one, and reports where it went. An empty anchor forks from the cursor,
// which copies the conversation as it stands. Nothing in this session moves.
func (engine *Engine) Fork(anchorID string) (session.ForkResult, error) {
	if engine.TurnRunning() {
		return session.ForkResult{}, ErrForkWhileTurnRunning
	}
	return engine.sessionRef().Fork(anchorID)
}

// ForkBoundaries lists the places a fork may be taken at, oldest first: what
// the /fork completer offers. Unlike the rewind list it keeps the entry the
// cursor stands on, because copying the whole conversation is a fork the
// user may well want.
func (engine *Engine) ForkBoundaries() []session.TurnBoundary {
	return engine.sessionRef().ForkBoundaries()
}
