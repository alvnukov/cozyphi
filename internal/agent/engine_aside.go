package agent

import (
	"context"
	"errors"
	"iter"
	"maps"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/session/compaction"
)

// ErrAsideWhileTurnRunning refuses a side question while inference or a tool
// is still in flight: the context it would be asked about is still being
// written.
var ErrAsideWhileTurnRunning = errors.New("cannot ask a side question while a turn is running")

// asideInstruction goes in front of the question. The request carries the
// same tool list a turn does, so the prompt prefix the provider has cached
// still matches, and some providers refuse a history with tool calls in it
// when no tools are declared. Declared tools can still be called, which is
// why the instruction asks for text and RunAside stops at the first call.
const asideInstruction = "This is a side question about the conversation so far. " +
	"Answer it in plain text. Do not call any tools: none will run. " +
	"Neither the question nor your answer stays in the conversation."

// AsideRequest is a side question checked and resolved against the session,
// ready to be sent. Everything that may refuse the question has already had
// its say by the time one exists.
type AsideRequest struct {
	// ID is the entry id the answer streams and is recorded under.
	ID       string
	Question string
	Anchor   string
	Leaf     string
	// AnchorPreview names the anchor when the question is not about the
	// whole context.
	AnchorPreview string
	messages      []llm.Message
}

// PrepareAside checks a side question and resolves what it is asked about:
// an empty anchor is the current context as it stands, a named one is a
// message of it. Nothing is sent and nothing is written; a refusal here
// leaves no trace.
func (engine *Engine) PrepareAside(question, anchorID string) (AsideRequest, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return AsideRequest{}, session.ErrEmptyAsideQuestion
	}
	if engine.TurnRunning() {
		return AsideRequest{}, ErrAsideWhileTurnRunning
	}
	scope, err := engine.sessionRef().AsideScope(anchorID)
	if err != nil {
		return AsideRequest{}, err
	}
	messages := engine.frozenProjection(contextMessages(scope.Entries))
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: asideInstruction + "\n\n" + question,
	})
	return AsideRequest{
		ID:            session.NewEntryID(),
		Question:      question,
		Anchor:        scope.Anchor,
		Leaf:          scope.Leaf,
		AnchorPreview: scope.AnchorPreview,
		messages:      messages,
	}, nil
}

// AsideAnchors lists the messages a side question may be asked about,
// oldest first: what the /btw completer offers.
func (engine *Engine) AsideAnchors() []session.AsideAnchor {
	return engine.sessionRef().AsideAnchors()
}

// Aside asks a side question and streams the answer: PrepareAside and
// RunAside in one call.
func (engine *Engine) Aside(
	ctx context.Context,
	question, anchorID string,
	yield func(session.Event) bool,
) error {
	req, err := engine.PrepareAside(question, anchorID)
	if err != nil {
		return err
	}
	return engine.RunAside(ctx, req, yield)
}

// RunAside sends a prepared side question and streams the answer as
// AsideUpdate events under the request's id. A finished answer is recorded
// as an aside entry; one cut short by ctx or by an error is not, so the file
// holds only answers that were given in full. Tools never run: the first
// tool call the model makes ends the answer, and the entry says which tool
// it was.
//
// The cursor, the context and the engine's cached view of it are the same
// afterwards as before, and so is everything a turn keeps between rounds.
func (engine *Engine) RunAside(ctx context.Context, req AsideRequest, yield func(session.Event) bool) error {
	if req.ID == "" {
		return errors.New("agent: side question was not prepared")
	}
	if engine.TurnRunning() {
		return ErrAsideWhileTurnRunning
	}
	rt := engine.roundSnapshot()
	if rt.client == nil {
		return errors.New("agent: no model to ask")
	}
	update := session.AsideUpdate{
		ID:            req.ID,
		Anchor:        req.Anchor,
		AnchorPreview: req.AnchorPreview,
		Question:      req.Question,
		State:         session.StateStreaming,
		Model:         rt.modelName,
	}
	events := rt.client.Stream
	if engine.asideStream != nil {
		events = engine.asideStream
	}
	finished, usage, err := streamAside(ctx, events(ctx, req.messages), &update, yield)
	if err != nil {
		update.State = session.StateError
		update.Error = err.Error()
		yield(update)
		return err
	}
	// Esc after the last token still cancels: the row says so, so the file
	// must not keep the answer.
	if !finished || ctx.Err() != nil {
		update.State = session.StateCancelled
		yield(update)
		return ctx.Err()
	}
	if err := engine.sessionRef().AppendAside(session.AsideEntry{
		SessionBaseEntry: session.SessionBaseEntry{ID: req.ID},
		Leaf:             req.Leaf,
		Anchor:           req.Anchor,
		Question:         req.Question,
		Answer:           update.Answer,
		SkippedTool:      update.SkippedTool,
		Model:            rt.modelName,
		Usage:            usage,
	}); err != nil {
		update.State = session.StateError
		update.Error = "The answer was not saved: " + err.Error()
		yield(update)
		return err
	}
	update.State = session.StateComplete
	yield(update)
	return nil
}

// streamAside reads the answer into update, yielding it as it grows. It
// reports false when the answer never finished: ctx ended it, or the
// consumer stopped listening.
func streamAside(
	ctx context.Context,
	events iter.Seq2[llm.StreamEvent, error],
	update *session.AsideUpdate,
	yield func(session.Event) bool,
) (bool, llm.Usage, error) {
	for event, err := range events {
		if err != nil {
			if ctx.Err() != nil {
				return false, llm.Usage{}, nil
			}
			return false, llm.Usage{}, err
		}
		switch event.Type {
		case llm.StreamEventTypeError:
			if ctx.Err() != nil {
				return false, llm.Usage{}, nil
			}
			if event.Err != nil {
				return false, llm.Usage{}, event.Err
			}
			return false, llm.Usage{}, errors.New("stream error")
		case llm.StreamEventTypeDelta:
			if len(event.Delta.ToolCalls) > 0 {
				update.SkippedTool = toolCallName(event.Delta.ToolCalls)
				return true, llm.Usage{}, nil
			}
			if event.Delta.Content == "" {
				continue
			}
			update.Answer += event.Delta.Content
			if !yield(*update) {
				return false, llm.Usage{}, nil
			}
		case llm.StreamEventTypeDone:
			if len(event.Partial.Choices) > 0 {
				final := event.Partial.Choices[0].Message
				if final.Content != "" {
					update.Answer = final.Content
				}
				if len(final.ToolCalls) > 0 {
					update.SkippedTool = toolCallName(final.ToolCalls)
				}
			}
			return true, event.Partial.Usage, nil
		}
	}
	if ctx.Err() == nil {
		return false, llm.Usage{}, errors.New("agent: stream closed without an answer")
	}
	return false, llm.Usage{}, nil
}

func toolCallName(calls []llm.ToolCall) string {
	if name := calls[0].Function.Name; name != "" {
		return name
	}
	return "a tool"
}

// frozenProjection applies the tool-result stubs a turn has already frozen
// and never adds to them, so a side question sends the prefix the last turn
// sent and leaves the stub set exactly as it found it.
func (engine *Engine) frozenProjection(messages []llm.Message) []llm.Message {
	engine.mu.RLock()
	settings := engine.compactionSettings
	frozen := maps.Clone(engine.microStubbed)
	engine.mu.RUnlock()
	projected, _, _ := compaction.Microcompact(messages, compaction.MicroPolicy{
		KeepVerbatim:     keepToolResultVerbatim,
		Advice:           microAdvice,
		KeepRecentTokens: settings.KeepRecentTokens(),
	}, frozen)
	return projected
}
