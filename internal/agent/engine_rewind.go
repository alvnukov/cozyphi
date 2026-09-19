package agent

import (
	"errors"

	"github.com/alvnukov/cozyphi/internal/session"
)

// ErrTurnRunning refuses a cursor move while inference or a tool is still in
// flight. Moving the leaf then would cut a round in half and hand the model a
// tool call whose result now belongs to a branch it is not on.
var ErrTurnRunning = errors.New("cannot move the session cursor while a turn is running")

// beginTurn and endTurn bracket one run of the loop. The count, not a flag,
// is what makes the answer right when a queued prompt starts before the turn
// before it has finished unwinding.
func (engine *Engine) beginTurn() {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.turnRunning++
}

func (engine *Engine) endTurn() {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.turnRunning--
}

// TurnRunning reports a turn in flight on this engine.
func (engine *Engine) TurnRunning() bool {
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	return engine.turnRunning > 0
}

// Rewind moves the session cursor to the turn boundary anchored at entryID:
// before a prompt the user sent, or after the answer that finished a turn.
// The branch the cursor leaves stays in the file, so the next turn grows a
// new one and nothing is lost. If the branch no longer fits the window, the
// existing ErrCompactionRequired path handles it as it always has.
func (engine *Engine) Rewind(entryID string) (session.RewindResult, error) {
	if engine.TurnRunning() {
		return session.RewindResult{}, ErrTurnRunning
	}
	return engine.sessionRef().Rewind(entryID)
}

// UndoRewind sends the cursor back to where the last move started from.
func (engine *Engine) UndoRewind() (session.RewindResult, error) {
	if engine.TurnRunning() {
		return session.RewindResult{}, ErrTurnRunning
	}
	return engine.sessionRef().UndoRewind()
}

// TurnBoundaries lists the places the current context can be cut at, oldest
// first: what the /rewind completer offers.
func (engine *Engine) TurnBoundaries() []session.TurnBoundary {
	return engine.sessionRef().TurnBoundaries()
}
