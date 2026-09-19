package agent

import (
	"errors"
	"strings"

	"github.com/alvnukov/cozyphi/internal/session"
)

// skillReadInstruction opens the paragraph composeUserPrompt puts in front of
// a prompt sent with skills attached. It is harness text rather than the
// user's, and pendingSkillsInstruction builds the paragraph from it, so the
// two cannot drift apart without a test saying so.
//
// No test can cover the other way this breaks. Rewording the constant moves
// both sides at once and goes green, while every prompt already written to a
// log keeps the old wording and stops being recognized. Change the wording
// only by leaving the old one in place to be stripped as well.
const skillReadInstruction = "You MUST read these skill files first with the read tool and follow them:"

// userTypedPrompt takes the harness paragraph back off a recorded prompt, so
// what reaches the composer is what the user wrote. Sent again as it stands,
// the instruction would reach the model a second time.
//
// It handles the paragraph a skill attachment adds. A plan step that preloads
// skill bodies prepends its own block, and that one ends in a skill body of
// arbitrary shape: where the body stops and the user's text starts cannot be
// read back out of the recorded message. Recovering that case needs the
// user's own text kept on the entry, which is a change to the log format.
func userTypedPrompt(prompt string) string {
	if !strings.HasPrefix(prompt, skillReadInstruction) {
		return prompt
	}
	_, rest, found := strings.Cut(prompt, "\n\n")
	if !found {
		// The instruction was the whole message: the turn carried skills and
		// no words of the user's.
		return ""
	}
	return strings.TrimLeft(rest, " \t\n")
}

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
