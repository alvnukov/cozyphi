package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// ErrForkNotPersisted refuses a fork of a session that is not written to disk.
// A fork is a second file beside the first one, and an in-memory session has
// no first one.
var ErrForkNotPersisted = errors.New("session: this session is not written to disk, so it cannot be forked")

// ErrNothingToFork refuses a fork of a session that holds no whole turn yet,
// an empty one included. There is no place to stop the copy at, and the copy
// would be an empty session, which is what a new tab already is.
var ErrNothingToFork = errors.New("session: nothing to fork yet: no turn of this session has finished")

// ForkResult is what a fork gives back to the shell: the session that was
// written and the prompt the tab showing it starts with.
type ForkResult struct {
	// SessionID is the id of the session that was written.
	SessionID string
	// File is the JSONL the fork was written to.
	File string
	// Anchor is the entry the copy stops at, as the log names it.
	Anchor string
	// Prompt is the text a fork cut before a prompt hands to the composer,
	// empty when the copy stops after an answer.
	Prompt string
}

// Fork writes the branch up to anchorID as a new session file in the same
// directory, and returns the manager that owns it. An empty anchorID forks
// from the current cursor, which copies the whole conversation as it stands.
//
// The anchor is a turn boundary, the same one a rewind is allowed at: the
// copy stops either before a prompt the user sent, whose text the caller
// hands to the composer, or after the answer that finished a turn. Entries
// keep the ids they have in the source, so replay and anything else holding
// an id still names the same message in the copy.
//
// The source is only read. Its file is left exactly as it was, and its
// cursor stays where it stands.
func Fork(src *Manager, anchorID string) (*Manager, error) {
	if src == nil {
		return nil, errors.New("session: no session to fork")
	}
	plan, err := src.forkPlan(anchorID)
	if err != nil {
		return nil, err
	}
	// Out of the source's lock from here on. Writing the copy is a whole
	// file with an fsync and a rename behind it, and everything asking the
	// source a question would be waiting on that disk.
	dst, err := NewSessionManager(plan.cwd,
		WithSessionDir(plan.dir),
		WithShouldFlush(true),
		WithParent(plan.parent),
		WithForkAnchor(plan.anchor),
		WithModel(plan.model),
	)
	if err != nil {
		return nil, fmt.Errorf("session: start the fork: %w", err)
	}
	if err := dst.adoptBranch(plan.branch); err != nil {
		return nil, errors.Join(err, dst.Close())
	}
	return dst, nil
}

// forkPlan is everything the copy needs from the session it is taken from,
// read out as values so that writing the copy needs nothing of the source.
type forkPlan struct {
	branch []MessageEntry
	cwd    string
	dir    string
	parent string
	anchor string
	model  string
}

// forkPlan reads the source under its lock and lets go of it. The branch it
// carries is a slice of its own, and the entries in it are values, so the
// source may grow a turn while the copy is still being written.
func (sm *Manager) forkPlan(anchorID string) (forkPlan, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.closed {
		return forkPlan{}, os.ErrClosed
	}
	if !sm.config.shouldFlush || sm.sessionFile == "" {
		return forkPlan{}, ErrForkNotPersisted
	}
	boundary, err := sm.forkBoundaryLocked(anchorID)
	if err != nil {
		return forkPlan{}, err
	}
	// The branch is taken from the raw chain rather than from the built
	// context, so the copy carries what the context is built out of:
	// compaction entries with their summaries and their drop masks, and the
	// messages behind them. Copying the built context instead would hand the
	// fork a transcript that starts at the last compaction.
	return forkPlan{
		branch: walkPath(sm.entries, boundary.Target, sm.byIDs),
		cwd:    sm.cwd,
		dir:    filepath.Dir(sm.sessionFile),
		parent: sm.sessionID,
		anchor: boundary.EntryID,
		model:  sm.modelLocked(),
	}, nil
}

// ForkBoundary resolves the anchor a fork would be taken at: the entry named,
// or the newest boundary of the path when the name is empty. It is what a
// caller asks before forking, to learn the prompt the copy hands over without
// working the anchor out a second time.
func (sm *Manager) ForkBoundary(anchorID string) (TurnBoundary, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.forkBoundaryLocked(anchorID)
}

// ForkBoundaries lists every place a fork may be taken, oldest first. Unlike
// the rewind list it keeps the boundary the cursor already stands on: a copy
// of the whole conversation is a reasonable thing to ask for, and it is what
// a fork with no anchor takes. Callers hold no lock.
func (sm *Manager) ForkBoundaries() []TurnBoundary {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return TurnBoundaries(sm.contextPathLocked())
}

// forkBoundaryLocked works out where a fork would stop. Callers hold mu.
//
// An anchor nobody named is the newest boundary of the path rather than the
// entry the cursor stands on. The two are usually the same entry, and where
// they are not, the cursor is inside a turn: a stream cut in half leaves an
// assistant message with a tool call and no result behind it. Answering that
// with the cursor would refuse a fork nobody aimed anywhere, naming an id the
// user never typed. The newest boundary is what "copy the conversation as it
// stands" means when the last thing in it is half a turn.
//
// A named anchor is answered as named. The user chose that entry, so a
// refusal that names it back is about the row they clicked.
func (sm *Manager) forkBoundaryLocked(anchorID string) (TurnBoundary, error) {
	path := sm.contextPathLocked()
	if anchorID != "" {
		return TurnBoundaryAt(path, anchorID)
	}
	boundaries := TurnBoundaries(path)
	if len(boundaries) == 0 {
		return TurnBoundary{}, ErrNothingToFork
	}
	return boundaries[len(boundaries)-1], nil
}

// modelLocked returns the model the session last ran with, the same answer
// Model gives. Callers hold mu.
func (sm *Manager) modelLocked() string {
	if msg, ok := sm.lastAssistantModelEntry(); ok {
		return msg.Model
	}
	return sm.model
}

// adoptBranch takes a branch copied from another session as this session's
// own, and writes the whole file at once. The entries keep their ids and
// their parents, so the chain they formed in the source is the chain they
// form here.
//
// The file is written before the manager is handed back, rather than waiting
// for the first answer the way a fresh session does: the fork exists to be
// opened by something else, and an empty path is nothing to open.
func (sm *Manager) adoptBranch(branch []MessageEntry) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for _, entry := range branch {
		sm.entries = append(sm.entries, entry)
		sm.byIDs[entry.GetID()] = entry
		if msg, ok := entry.(SessionMessageEntry); ok && msg.Message.Role == llm.RoleAssistant {
			sm.hasAssistantMsg = true
		}
	}
	if len(branch) > 0 {
		setLeaf(&sm.leafID, branch[len(branch)-1].GetID())
	}
	if err := sm.flushAllEntries(); err != nil {
		return err
	}
	sm.flushed = true
	return nil
}
