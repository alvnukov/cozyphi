package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

// streamTurn runs one inference round against the snapshot's client and
// yields the streaming events; everything it reads comes from rt so a setter
// cannot swap state under a running round.
func streamTurn(
	ctx context.Context,
	yield func(session.Event, error) bool,
	messages []llm.Message,
	rt roundRuntime,
) (llm.Message, session.Event, bool, error) {
	id := fmt.Sprintf("assistant-%d", time.Now().UnixNano())
	started := time.Now()
	model := rt.modelName
	var thinking, text string
	var final llm.Message
	var finish string
	var thinkStart, thinkEnd time.Time
	gotDone := false
	// thinkingSpan is the round's reasoning wall time: first reasoning delta
	// to first text delta (or to stream end when reasoning ran to the wire).
	thinkingSpan := func() time.Duration {
		if thinkStart.IsZero() || thinkEnd.IsZero() || thinkEnd.Before(thinkStart) {
			return 0
		}
		return thinkEnd.Sub(thinkStart)
	}

	for event, err := range rt.client.Stream(ctx, messages) {
		if err != nil {
			if thinking != "" || text != "" {
				if !yield(
					emitMessage(
						id,
						session.StateError,
						session.StopNone,
						thinking,
						text,
						nil,
						llm.Usage{},
						model,
						started,
						thinkingSpan(),
					),
					nil,
				) {
					return llm.Message{}, nil, false, nil
				}
			}
			return llm.Message{}, nil, false, err
		}

		switch event.Type {
		case llm.StreamEventTypeError:
			if event.Err != nil {
				// Pass the typed cause through untouched: classification
				// (cancel / 429 / auth) downstream relies on the chain.
				return llm.Message{}, nil, false, event.Err
			}
			return llm.Message{}, nil, false, errors.New("stream error")

		case llm.StreamEventTypeDelta:
			if event.Delta.ReasoningContent != "" {
				if thinkStart.IsZero() {
					thinkStart = time.Now()
				}
				thinking += event.Delta.ReasoningContent
			}
			if event.Delta.Content != "" {
				if !thinkStart.IsZero() && thinkEnd.IsZero() {
					thinkEnd = time.Now()
				}
				text += event.Delta.Content
			}
			if !yield(
				emitMessage(
					id,
					session.StateStreaming,
					session.StopNone,
					thinking,
					text,
					nil,
					llm.Usage{},
					model,
					started,
					thinkingSpan(),
				),
				nil,
			) {
				return llm.Message{}, nil, false, nil
			}

		case llm.StreamEventTypeDone:
			if len(event.Partial.Choices) == 0 {
				return llm.Message{}, nil, false, errors.New("agent: stream finished with no assistant choice")
			}
			final = event.Partial.Choices[0].Message
			final.Usage = event.Partial.Usage
			finish = event.Partial.Choices[0].FinishReason
			gotDone = true
			if !thinkStart.IsZero() && thinkEnd.IsZero() {
				thinkEnd = time.Now()
			}
			// Prefer fully accumulated message for the complete event.
			if final.ReasoningContent != "" {
				thinking = final.ReasoningContent
			}
			if final.Content != "" {
				text = final.Content
			}
		}
	}

	if !gotDone {
		if ctx.Err() != nil {
			_ = yield(
				emitMessage(
					id,
					session.StateCancelled,
					session.StopNone,
					thinking,
					text,
					nil,
					llm.Usage{},
					model,
					started,
					thinkingSpan(),
				),
				nil,
			)
			return llm.Message{}, nil, false, nil
		}
		return llm.Message{}, nil, false, errors.New("agent: stream closed without assistant output")
	}

	blocks := toolCallsToBlocks(rt.executor, final.ToolCalls)
	reason := stopReasonFromFinish(finish, len(blocks) > 0)
	complete := emitMessage(
		id,
		session.StateComplete,
		reason,
		thinking,
		text,
		blocks,
		final.Usage,
		model,
		started,
		thinkingSpan(),
	)
	return final, complete, true, nil
}

// stopReasonFromFinish maps the provider's raw finish signal onto the session
// stop reason: a budget cutoff always wins (the transcript must never render
// a truncated round as a clean end), then tool use, then a normal end turn.
// An empty signal falls back to inferring from the accumulated message.
func stopReasonFromFinish(finish string, hasTools bool) session.StopReason {
	switch finish {
	case "max_tokens", "length":
		return session.StopMaxTokens
	case "tool_use", "tool_calls":
		return session.StopToolUse
	case "end_turn", "stop", "stop_sequence":
		return session.StopEndTurn
	}
	if hasTools {
		return session.StopToolUse
	}
	return session.StopEndTurn
}

// toolCallsToBlocks renders tool calls as transcript blocks, preferring a
// tool's own DetailFromArgs summary over the raw arguments. It runs inside the
// streaming round, so it reads the executor from the round snapshot rather
// than the engine field a setter may be swapping.
func toolCallsToBlocks(exec *Executor, calls []llm.ToolCall) []session.ContentBlock {
	if len(calls) == 0 {
		return nil
	}
	out := make([]session.ContentBlock, 0, len(calls))
	for _, c := range calls {
		input := c.Function.Arguments
		if exec != nil {
			if tool, ok := exec.registry[c.Function.Name]; ok && tool.DetailFromArgs != nil {
				if d := tool.DetailFromArgs(json.RawMessage(c.Function.Arguments)); d != "" {
					input = d
				}
			}
		}
		out = append(out, session.ContentBlock{
			Type:     session.BlockToolUse,
			ID:       c.ID,
			Name:     c.Function.Name,
			Input:    input,
			Complete: true,
		})
	}
	return out
}

func buildContent(thinking, text string, tools []session.ContentBlock) []session.ContentBlock {
	var out []session.ContentBlock
	if thinking != "" {
		out = append(out, session.ContentBlock{Type: session.BlockThinking, Text: thinking})
	}
	if text != "" {
		out = append(out, session.ContentBlock{Type: session.BlockText, Text: text})
	}
	out = append(out, tools...)
	return out
}

func emitMessage(
	id string,
	state session.State,
	reason session.StopReason,
	thinking,
	text string,
	tools []session.ContentBlock,
	usage llm.Usage,
	model string,
	started time.Time,
	thinkDur time.Duration,
) session.Event {
	msg := session.Message{
		ID:         id,
		State:      state,
		StopReason: reason,
		Content:    buildContent(thinking, text, tools),
		Text:       text,
		Usage: session.TokenUsage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			CachedTokens:     usage.CachedTokens(),
			TotalTokens:      usage.TotalTokens,
		},
		Model:            model,
		Started:          started,
		ThinkingDuration: thinkDur,
	}
	if state != session.StateStreaming {
		msg.Ended = time.Now()
	}
	return session.AssistantMessageUpdate{Message: msg}
}
