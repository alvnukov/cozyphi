package session

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// EntryAside is a side question and its answer. Like the title and a cursor
// move it is session metadata rather than a node of the conversation: no path
// walks through it and the cursor never lands on it, so no context built for
// the model can contain it.
const EntryAside = "aside"

// AsideEntry records one side question, the answer the model gave, and the
// place in the conversation it was asked about.
type AsideEntry struct {
	SessionBaseEntry
	// Leaf is where the cursor stood when the question was asked.
	Leaf string `json:"leaf,omitempty"`
	// Anchor is the entry the question was asked about. The context the
	// model answered from ends there, the anchor included.
	Anchor   string `json:"anchor"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
	// SkippedTool names the tool the model asked to run when the answer
	// stopped there. Tools never run for a side question, so a call ends it.
	SkippedTool string    `json:"skippedTool,omitempty"`
	Model       string    `json:"model,omitempty"`
	Usage       llm.Usage `json:"usage,omitzero"`
}

// GetType implements MessageEntry.
func (AsideEntry) GetType() string { return EntryAside }

// GetID implements MessageEntry.
func (a AsideEntry) GetID() string { return a.ID }

// GetParent implements MessageEntry. The leaf the question was asked at is
// kept in Leaf rather than here: a parent would make the entry a child in the
// tree, and a turn could then be written onto it.
func (AsideEntry) GetParent() *string { return nil }

// ErrNothingToAsk refuses a side question in a session whose context is empty.
var ErrNothingToAsk = errors.New("session: nothing to ask about yet: the context is empty")

// ErrEmptyAsideQuestion refuses a side question with no words in it.
var ErrEmptyAsideQuestion = errors.New("session: a side question needs a question")

// NotAsideAnchorError refuses an anchor that is not a message of the current
// context.
type NotAsideAnchorError struct{ EntryID string }

func (e *NotAsideAnchorError) Error() string {
	return "session: " + e.EntryID + " is not a message in the current context:" +
		" a side question is asked about a message the feed shows"
}

// AsideScope is what a side question is asked about: the anchor it was
// resolved to, where the cursor stood, and the context the model answers from.
type AsideScope struct {
	Anchor string
	Leaf   string
	// AnchorPreview names the anchor when it is not the end of the context,
	// and is empty when the question is about all of it.
	AnchorPreview string
	// Entries is the current context cut after the anchor, oldest first.
	Entries []MessageEntry
}

// AsideScope resolves the context a side question is asked about. An empty
// anchor is the whole current context and resolves to its last entry, so the
// record names a real place however the question was asked.
//
// A named anchor is any message of the current context, a tool call or its
// result as much as a prompt or an answer. The context is the one the model
// is shown now, cut after the anchor, rather than the raw chain behind it: it
// is made of the same rows the feed shows above the anchor, compaction and
// deleted blocks included. An entry off the current path is refused, the same
// way a rewind and a fork refuse one, because the feed never shows it.
func (sm *Manager) AsideScope(anchorID string) (AsideScope, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.closed {
		return AsideScope{}, os.ErrClosed
	}
	path := sm.contextPathLocked()
	if len(path) == 0 {
		return AsideScope{}, ErrNothingToAsk
	}
	scope := AsideScope{Leaf: sm.leafLocked()}
	if anchorID == "" {
		scope.Anchor = path[len(path)-1].GetID()
		scope.Entries = slices.Clone(path)
		return scope, nil
	}
	for i, entry := range path {
		if entry.GetID() != anchorID {
			continue
		}
		if _, isMessage := entry.(SessionMessageEntry); !isMessage {
			break
		}
		scope.Anchor = anchorID
		scope.Entries = slices.Clone(path[:i+1])
		scope.AnchorPreview = anchorPreview(path, anchorID)
		return scope, nil
	}
	return AsideScope{}, &NotAsideAnchorError{EntryID: anchorID}
}

// AsideAnchor is one message a side question may be asked about.
type AsideAnchor struct {
	EntryID string
	Preview string
}

// AsideAnchors lists the messages of the current context, oldest first: the
// places a side question may be asked about.
func (sm *Manager) AsideAnchors() []AsideAnchor {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	path := sm.contextPathLocked()
	anchors := make([]AsideAnchor, 0, len(path))
	for _, entry := range path {
		if message, ok := entry.(SessionMessageEntry); ok {
			anchors = append(anchors, AsideAnchor{EntryID: message.ID, Preview: asidePreview(message)})
		}
	}
	return anchors
}

// anchorPreview names the anchor when it is not the last entry of context,
// and is empty when it is: a question about the end of the context is a
// question about all of it.
func anchorPreview(context []MessageEntry, anchor string) string {
	if len(context) == 0 || context[len(context)-1].GetID() == anchor {
		return ""
	}
	for _, entry := range context {
		if message, ok := entry.(SessionMessageEntry); ok && message.ID == anchor {
			return asidePreview(message)
		}
	}
	return ""
}

func asidePreview(entry SessionMessageEntry) string {
	msg := entry.Message
	switch {
	case msg.Role == llm.RoleTool:
		return "tool result " + displayText(msg.Content, 40)
	case msg.Role == llm.RoleAssistant && strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) > 0:
		names := make([]string, 0, len(msg.ToolCalls))
		for _, call := range msg.ToolCalls {
			names = append(names, call.Function.Name)
		}
		return "calls " + displayText(strings.Join(names, ", "), 42)
	case msg.Role == llm.RoleAssistant:
		return "answer " + displayText(msg.Content, 41)
	default:
		return "prompt " + displayText(stripReminders(msg.Content), 41)
	}
}

// AppendAside writes a side question and its answer to the log. The cursor
// stays where it stands, and nothing the model is shown changes. The entry
// keeps the id its caller gave it, so the row that streamed the answer and
// the record in the file are one and the same.
func (sm *Manager) AppendAside(entry AsideEntry) error {
	if entry.ID == "" {
		return errors.New("session: a side question needs an entry id")
	}
	if strings.TrimSpace(entry.Question) == "" {
		return ErrEmptyAsideQuestion
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.closed {
		return os.ErrClosed
	}
	if _, taken := sm.byIDs[entry.ID]; taken {
		return fmt.Errorf("session: entry ID %q is already in the log", entry.ID)
	}
	entry.Type = EntryAside
	entry.ParentID = nil
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	previousLen := len(sm.entries)
	sm.entries = append(sm.entries, entry)
	sm.byIDs[entry.ID] = entry
	if sm.config.shouldFlush {
		if err := sm.flush(entry); err != nil {
			sm.entries = sm.entries[:previousLen]
			delete(sm.byIDs, entry.ID)
			return fmt.Errorf("session: persist side question: %w", err)
		}
	}
	return nil
}

// Asides returns the side questions recorded in this session, oldest first.
func (sm *Manager) Asides() []AsideEntry {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	var asides []AsideEntry
	for _, entry := range sm.entries {
		if aside, ok := entry.(AsideEntry); ok {
			asides = append(asides, aside)
		}
	}
	return asides
}

// AsideSkippedToolNote is the line an answer that stopped at a tool call ends
// with, so the stop is not read as the whole answer.
func AsideSkippedToolNote(tool string) string {
	if tool == "" {
		tool = "a tool"
	}
	return "[stopped here: the model asked to run " + tool + ", and tools do not run for a side question]"
}

// asideRow is the feed row of a side question. Its state is set by the
// update carrying its id and by nothing else: the row is no assistant turn,
// so a cancel aimed at the last one never reaches it.
func asideRow(e AsideUpdate) Message {
	return Message{
		ID:    e.ID,
		Role:  RoleAside,
		State: e.State,
		Model: e.Model,
		Aside: AsideRow{
			Question:      e.Question,
			AnchorPreview: e.AnchorPreview,
			Answer:        e.Answer,
			SkippedTool:   e.SkippedTool,
			Error:         e.Error,
		},
	}
}
