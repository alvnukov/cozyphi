package session

import (
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/memory"
)

// TurnBoundaryKind says what a cut at the anchor does to the context.
type TurnBoundaryKind int

const (
	// BoundaryPrompt cuts before a prompt the user sent: the prompt leaves
	// the context and its text goes back to the composer.
	BoundaryPrompt TurnBoundaryKind = iota + 1
	// BoundaryAnswer cuts after the answer that finished a turn: the answer
	// stays in the context.
	BoundaryAnswer
)

// TurnBoundary is one place the session context can be cut at. Rewind and
// fork share this list, because both of them may only start a new branch
// where a turn started or ended: anywhere inside a turn the model would find
// a tool call without its result, or a question without its answer.
type TurnBoundary struct {
	// EntryID is the message the boundary is anchored at.
	EntryID string
	// Kind tells whether the anchor is cut before or after.
	Kind TurnBoundaryKind
	// Target is the entry the cursor stands on after the cut. It is empty
	// when the cut empties the context, which is what a rewind to the very
	// first prompt of a session does.
	Target string
	// Prompt is the text a cut here gives back to the composer, empty for
	// an answer anchor.
	Prompt string
	// Preview is one short line naming the boundary for a picker.
	Preview string
}

// NotTurnBoundaryError reports a cut asked for somewhere inside a turn.
type NotTurnBoundaryError struct{ EntryID string }

func (e *NotTurnBoundaryError) Error() string {
	return "session: " + e.EntryID + " is not a turn boundary: cut before a prompt you sent" +
		" or after the answer that finished a turn"
}

// TurnBoundaries lists the places the given context path can be cut at,
// oldest first. The path is what BuildContext returns, so a boundary is
// always a row the user can see.
func TurnBoundaries(path []MessageEntry) []TurnBoundary {
	boundaries := make([]TurnBoundary, 0, len(path))
	for _, entry := range path {
		if boundary, ok := turnBoundary(entry); ok {
			boundaries = append(boundaries, boundary)
		}
	}
	return boundaries
}

// TurnBoundaryAt returns the boundary anchored at entryID within path.
func TurnBoundaryAt(path []MessageEntry, entryID string) (TurnBoundary, error) {
	for _, entry := range path {
		if entry.GetID() != entryID {
			continue
		}
		if boundary, ok := turnBoundary(entry); ok {
			return boundary, nil
		}
		break
	}
	return TurnBoundary{}, &NotTurnBoundaryError{EntryID: entryID}
}

func turnBoundary(entry MessageEntry) (TurnBoundary, bool) {
	message, ok := entry.(SessionMessageEntry)
	if !ok {
		return TurnBoundary{}, false
	}
	if prompt, isPrompt := userPrompt(message); isPrompt {
		// The cut lands before the prompt, so the cursor goes to whatever
		// the prompt was appended after. A first prompt has no parent, and
		// an empty target is an empty context.
		target := ""
		if parent := message.GetParent(); parent != nil {
			target = *parent
		}
		return TurnBoundary{
			EntryID: message.ID,
			Kind:    BoundaryPrompt,
			Target:  target,
			Prompt:  prompt,
			Preview: "before " + displayText(prompt, 48),
		}, true
	}
	if !finishedAnswer(message) {
		return TurnBoundary{}, false
	}
	return TurnBoundary{
		EntryID: message.ID,
		Kind:    BoundaryAnswer,
		Target:  message.ID,
		Preview: "after " + displayText(message.Message.Content, 48),
	}, true
}

// userPrompt returns what the user typed, and false for anything else the
// user role carries: a tool result, a background delivery, or a message that
// is harness scaffolding and nothing more.
func userPrompt(entry SessionMessageEntry) (string, bool) {
	if entry.Message.Role != llm.RoleUser || entry.DeliveryID != "" || entry.Message.ToolCallID != "" {
		return "", false
	}
	text := stripReminders(entry.Message.Content)
	return text, text != ""
}

// finishedAnswer reports an assistant message that ended its turn: it asked
// for no tool, so nothing was left open behind it.
func finishedAnswer(entry SessionMessageEntry) bool {
	return entry.Message.Role == llm.RoleAssistant &&
		len(entry.Message.ToolCalls) == 0 &&
		strings.TrimSpace(entry.Message.Content) != ""
}

// stripReminders drops the harness reminder blocks a prompt was sent with and
// leaves what the user typed. It is the transcript's own rule, borrowed whole:
// a row the feed shows as the user's words is a row a rewind must be able to
// cut at, and two spellings of "what the user typed" would disagree about
// which rows those are.
func stripReminders(content string) string {
	return strings.TrimSpace(memory.StripReminders(content))
}
