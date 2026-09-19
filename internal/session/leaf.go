package session

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"time"
)

// EntryLeaf is a cursor move. Like the title, it is session metadata and
// never a node in the conversational chain, so it stays out of every context
// the model is shown.
const EntryLeaf = "leaf"

// LeafEntry records where the session cursor went and where it stood before.
// Moving the cursor is how a rewind happens: the log stays append-only, the
// branch the cursor left keeps every entry it had, and reloading the file
// replays the moves in order, so the last one wins.
type LeafEntry struct {
	SessionBaseEntry
	// Target is the entry the cursor moved onto, empty for an empty context.
	Target string `json:"target"`
	// From is where the cursor stood before the move, empty when it stood
	// on an empty context. It is what /rewind back reads.
	From string `json:"from,omitempty"`
}

// GetType implements MessageEntry.
func (LeafEntry) GetType() string { return EntryLeaf }

// GetID implements MessageEntry.
func (l LeafEntry) GetID() string { return l.ID }

// GetParent implements MessageEntry. A cursor move has no parent: it is not
// part of the conversation, so no path ever walks through it.
func (LeafEntry) GetParent() *string { return nil }

// ErrNothingToUndo reports /rewind back with no cursor move behind it.
var ErrNothingToUndo = errors.New("session: nothing to undo: the cursor has not been moved")

// ErrCursorAlreadyThere reports a move that would leave the cursor where it
// already stands.
var ErrCursorAlreadyThere = errors.New("session: the cursor already stands there")

// RewindResult is what a cursor move gives back to its caller: where the
// cursor went, where it came from, and the prompt text the composer gets.
type RewindResult struct {
	Target string
	From   string
	Prompt string
}

// TurnBoundaries lists the places the current context can be cut at, oldest
// first. It is what the /rewind completer offers and what a refused cut is
// measured against.
func (sm *Manager) TurnBoundaries() []TurnBoundary {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return TurnBoundaries(sm.contextPathLocked())
}

// Rewind moves the cursor to the turn boundary anchored at entryID: before a
// prompt the user sent, or after the answer that finished a turn. Anywhere
// else is refused. The entries the cursor leaves behind stay in the file, and
// the next turn is written as a new branch growing from the new cursor.
func (sm *Manager) Rewind(entryID string) (RewindResult, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.closed {
		return RewindResult{}, os.ErrClosed
	}
	boundary, err := TurnBoundaryAt(sm.contextPathLocked(), entryID)
	if err != nil {
		return RewindResult{}, err
	}
	from := sm.leafLocked()
	if boundary.Target == from {
		return RewindResult{}, ErrCursorAlreadyThere
	}
	if err := sm.moveLeafLocked(boundary.Target, from); err != nil {
		return RewindResult{}, err
	}
	return RewindResult{Target: boundary.Target, From: from, Prompt: boundary.Prompt}, nil
}

// UndoRewind sends the cursor back to where the last move started from. It
// records a move of its own, so two of them in a row land where they began:
// the undo is itself something to undo, and the file says so.
func (sm *Manager) UndoRewind() (RewindResult, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.closed {
		return RewindResult{}, os.ErrClosed
	}
	last, ok := sm.lastMoveLocked()
	if !ok {
		return RewindResult{}, ErrNothingToUndo
	}
	from := sm.leafLocked()
	if last.From == from {
		return RewindResult{}, ErrCursorAlreadyThere
	}
	if err := sm.moveLeafLocked(last.From, from); err != nil {
		return RewindResult{}, err
	}
	return RewindResult{Target: last.From, From: from}, nil
}

// leafLocked returns the current cursor as a plain string. Callers hold mu.
func (sm *Manager) leafLocked() string {
	if sm.leafID == nil {
		return ""
	}
	return *sm.leafID
}

// contextPathLocked returns the entries the model would be shown right now.
// Callers hold mu.
func (sm *Manager) contextPathLocked() []MessageEntry {
	if sm.leafID == nil {
		return nil
	}
	return buildSessionContext(sm.entries, *sm.leafID, sm.byIDs)
}

// lastMoveLocked returns the newest cursor move in the log. Callers hold mu.
func (sm *Manager) lastMoveLocked() (LeafEntry, bool) {
	for _, entry := range slices.Backward(sm.entries) {
		if move, ok := entry.(LeafEntry); ok {
			return move, true
		}
	}
	return LeafEntry{}, false
}

// moveLeafLocked records the move and then applies it, so a cursor the file
// never received never moves in memory either. Callers hold mu.
func (sm *Manager) moveLeafLocked(target, from string) error {
	entry := LeafEntry{
		SessionBaseEntry: SessionBaseEntry{
			Type:      EntryLeaf,
			ID:        sm.generateID(),
			Timestamp: time.Now(),
		},
		Target: target,
		From:   from,
	}
	previousLen := len(sm.entries)
	sm.entries = append(sm.entries, entry)
	sm.byIDs[entry.ID] = entry
	if sm.config.shouldFlush {
		if err := sm.flush(entry); err != nil {
			sm.entries = sm.entries[:previousLen]
			delete(sm.byIDs, entry.ID)
			return fmt.Errorf("session: persist cursor move: %w", err)
		}
	}
	setLeaf(&sm.leafID, target)
	return nil
}

// setLeaf points the cursor at target, or clears it for an empty context.
func setLeaf(leafID **string, target string) {
	if target == "" {
		*leafID = nil
		return
	}
	id := target
	*leafID = &id
}
