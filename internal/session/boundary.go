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
//
// It reports the shape of the path and nothing else: a boundary whose cut
// would leave the cursor exactly where it stands is in this list like any
// other. That is deliberate, because a copy of the branch taken from the
// current leaf is a whole session and a reasonable thing to ask for. Anything
// offering a cut to a reader wants Manager.TurnBoundaries instead, which
// leaves that one out; taking it up there earns nothing but a refusal.
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

// WithPrompt returns the boundary with its prompt text replaced and its
// preview rebuilt from it. A caller that knows more about what the user
// actually typed than the recorded message does uses it to keep the two in
// step: the line the picker offers and the text the composer receives must
// not be two different answers to the same question.
func (b TurnBoundary) WithPrompt(prompt string) TurnBoundary {
	if b.Kind != BoundaryPrompt {
		return b
	}
	b.Prompt = prompt
	b.Preview = promptPreview(prompt)
	return b
}

// promptPreview is the one line a cut before a prompt is offered under. Both
// the boundary as the log records it and the boundary a caller has corrected
// go through here, so the picker cannot start showing one thing while the
// composer receives another.
func promptPreview(prompt string) string {
	return "before " + displayText(prompt, 48)
}

// MovesCursor reports whether a cut here would actually move the cursor.
// A cut whose target is where the cursor already stands changes nothing, and
// Manager.Rewind refuses it. Everything that offers a cut asks this first, so
// that refusal stays a last resort rather than what the user meets: after a
// finished turn the cursor sits on the very answer at the bottom of the feed,
// which is the first button a reader reaches for.
func (b TurnBoundary) MovesCursor(leaf string) bool {
	return b.Target != leaf
}

// boundaryCut is what a cut at one entry amounts to, before it is dressed up
// for a picker: the message, the entry the cursor would land on, the kind of
// cut, and the prompt text a prompt boundary hands back. Everything that asks
// where a cut leads goes through here, and it stays cheap on purpose, because
// the strips ask it for every row they draw.
type boundaryCut struct {
	message SessionMessageEntry
	target  string
	prompt  string
	kind    TurnBoundaryKind
}

// boundaryCutAt works out the cut at one entry, and reports false when the
// entry is no boundary at all. It carries the message out with it so no
// caller has to assert the type a second time, and reads the prompt out once
// so no caller strips the harness wrapper twice.
func boundaryCutAt(entry MessageEntry) (boundaryCut, bool) {
	message, isMessage := entry.(SessionMessageEntry)
	if !isMessage {
		return boundaryCut{}, false
	}
	if prompt, isPrompt := userPrompt(message); isPrompt {
		// The cut lands before the prompt, so the cursor goes to whatever
		// the prompt was appended after. A first prompt has no parent, and
		// an empty target is an empty context.
		target := ""
		if parent := message.GetParent(); parent != nil {
			target = *parent
		}
		return boundaryCut{message: message, target: target, prompt: prompt, kind: BoundaryPrompt}, true
	}
	if !finishedAnswer(message) {
		return boundaryCut{}, false
	}
	// The cut lands after the answer, so the answer itself is the cursor.
	return boundaryCut{message: message, target: message.ID, kind: BoundaryAnswer}, true
}

func turnBoundary(entry MessageEntry) (TurnBoundary, bool) {
	cut, ok := boundaryCutAt(entry)
	if !ok {
		return TurnBoundary{}, false
	}
	boundary := TurnBoundary{
		EntryID: cut.message.ID,
		Kind:    cut.kind,
		Target:  cut.target,
		Prompt:  cut.prompt,
	}
	if cut.kind == BoundaryPrompt {
		boundary.Preview = promptPreview(cut.prompt)
		return boundary, true
	}
	boundary.Preview = "after " + displayText(cut.message.Message.Content, 48)
	return boundary, true
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
