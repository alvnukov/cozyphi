package session

import (
	"fmt"
	"strings"
	"time"
)

// ItemKind classifies a projected transcript row.
type ItemKind int

// ItemKind values for projected transcript rows.
const (
	ItemUser ItemKind = iota
	ItemThinking
	ItemAssistant
	ItemTool
	ItemCompaction
)

// Item is one list row projected from Snapshot.
type Item struct {
	ID   string
	Kind ItemKind

	Text  string
	State State
	// Queued marks a user row waiting behind the in-flight turn.
	Queued bool

	// Summary is the compaction summarize body (ItemCompaction only).
	Summary string

	Thinking    string
	Streaming   bool
	Interrupted bool
	// ThinkingDuration is the reasoning wall-clock span of the (possibly
	// coalesced) thinking segments; 0 when untimed.
	ThinkingDuration time.Duration

	ToolName  string
	ToolInput string
	ToolUseID string
	ToolRun   ToolRun

	// TurnMeta is end-of-round metadata (model, duration, usage), set on the
	// tail text row of a terminal assistant round. Zero Model means no row.
	TurnMeta TurnMeta
}

// TurnMeta carries per-round assistant metadata for the turn row.
type TurnMeta struct {
	Model    string
	Duration time.Duration
	Usage    TokenUsage
	// Truncated marks a round the provider cut off at the output-token
	// limit; the turn footer appends a "hit max tokens" warning.
	Truncated bool
}

// Project flattens Snapshot into list items in content order:
// user → thinking / assistant text / tool rows. Consecutive thinking rows
// coalesce into one so a run of reasoning blocks shows a single block.
func Project(s Snapshot) []Item {
	var items []Item
	for _, m := range s.Messages {
		switch m.Role {
		case RoleUser:
			if strings.TrimSpace(m.Text) != "" {
				items = append(items, Item{
					ID:     m.ID,
					Kind:   ItemUser,
					Text:   m.Text,
					Queued: m.Queued,
				})
			}
		case RoleAssistant:
			items = append(items, projectAssistant(m, s.Tools)...)
		case RoleCompaction:
			items = append(items, Item{
				ID:      m.ID,
				Kind:    ItemCompaction,
				Text:    m.Text,
				Summary: m.Summary,
			})
		case RoleWatch:
			// A watch renders as a local tool row: it is output that arrived
			// without anyone asking, which is what a local "!cmd" row already
			// looks like. Reusing it keeps the transcript one widget lighter.
			run := ToolRun{ToolUseID: m.ID, Name: "watch", Status: ToolDone, Detail: m.Text, Local: true}
			if tr, ok := s.Tools[m.ID]; ok {
				run = tr
				if run.Detail == "" {
					run.Detail = m.Text
				}
				run.Local = true
			}
			items = append(items, Item{
				ID:        "watch-" + m.ID,
				Kind:      ItemTool,
				ToolUseID: m.ID,
				ToolName:  "watch",
				ToolInput: m.Text,
				ToolRun:   run,
			})
		case RolePlan:
			// A plan automation renders like the watch row: local output nobody
			// asked for in the transcript, which the existing tool row already
			// shows — one widget fewer than a dedicated plan block.
			run := ToolRun{ToolUseID: m.ID, Name: planActionToolName, Status: ToolDone, Detail: m.Text, Local: true}
			if tr, ok := s.Tools[m.ID]; ok {
				run = tr
				if run.Detail == "" {
					run.Detail = m.Text
				}
				run.Local = true
			}
			items = append(items, Item{
				ID:        "plan-" + m.ID,
				Kind:      ItemTool,
				ToolUseID: m.ID,
				ToolName:  planActionToolName,
				ToolInput: m.Text,
				ToolRun:   run,
			})
		case RoleLocalBash:
			run := ToolRun{ToolUseID: m.ID, Name: "bash", Status: ToolInProgress, Detail: m.Text, Local: true}
			if s.Tools != nil {
				if tr, ok := s.Tools[m.ID]; ok {
					run = tr
					if run.Detail == "" {
						run.Detail = m.Text
					}
					run.Local = true
				}
			}
			items = append(items, Item{
				ID:        "bash-" + m.ID,
				Kind:      ItemTool,
				ToolUseID: m.ID,
				ToolName:  "bash",
				ToolInput: m.Text,
				ToolRun:   run,
			})
		}
	}
	return coalesceThinking(items)
}

// coalesceThinking merges consecutive thinking items — several reasoning
// blocks in one message, or thinking-only messages in a row — into a single
// row. The merged row keeps the first id (stable across streaming syncs) and
// the trailing row's streaming flag; a blank line separates the parts.
func coalesceThinking(items []Item) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if it.Kind == ItemThinking && len(out) > 0 && out[len(out)-1].Kind == ItemThinking {
			prev := &out[len(out)-1]
			if prev.Thinking != "" && it.Thinking != "" {
				prev.Thinking += "\n\n"
			}
			prev.Thinking += it.Thinking
			prev.Streaming = it.Streaming
			prev.Interrupted = prev.Interrupted || it.Interrupted
			prev.ThinkingDuration += it.ThinkingDuration
			continue
		}
		out = append(out, it)
	}
	return out
}

func projectAssistant(m Message, tools map[string]ToolRun) []Item {
	var items []Item
	textSeg := 0

	emitText := func(text string, streamingTail bool) {
		if text == "" && !streamingTail {
			return
		}
		st := m.State
		if !streamingTail && m.State == StateStreaming {
			st = StateComplete
		}
		if m.State == StateCancelled {
			st = StateCancelled
		}
		items = append(items, Item{
			ID:    fmt.Sprintf("%s-text-%d", m.ID, textSeg),
			Kind:  ItemAssistant,
			Text:  text,
			State: st,
		})
		textSeg++
	}

	var textBuf strings.Builder
	for i, b := range m.Content {
		switch b.Type {
		case BlockThinking:
			if strings.TrimSpace(b.Text) == "" && m.State != StateStreaming {
				continue
			}
			thinkStreaming := m.State == StateStreaming && isTrailingThinking(m.Content, i)
			items = append(items, Item{
				ID:               fmt.Sprintf("%s-thinking-%d", m.ID, i),
				Kind:             ItemThinking,
				Thinking:         b.Text,
				Streaming:        thinkStreaming,
				Interrupted:      m.State == StateCancelled,
				ThinkingDuration: m.ThinkingDuration,
			})
		case BlockText:
			textBuf.WriteString(b.Text)
		case BlockToolUse:
			emitText(textBuf.String(), false)
			textBuf.Reset()
			run := ToolRun{ToolUseID: b.ID, Name: b.Name, Status: ToolInProgress, Detail: b.Input}
			if tools != nil {
				if tr, ok := tools[b.ID]; ok {
					run = tr
					if run.Detail == "" {
						run.Detail = b.Input
					}
				}
			}
			items = append(items, Item{
				ID:        "tool-" + b.ID,
				Kind:      ItemTool,
				ToolUseID: b.ID,
				ToolName:  b.Name,
				ToolInput: b.Input,
				ToolRun:   run,
			})
		}
	}

	tail := textBuf.String()
	emptyWait := m.State == StateStreaming && len(items) == 0 && tail == ""
	emitText(tail, emptyWait || (m.State == StateStreaming && tail != ""))

	// A round cut off by the output-token limit must never render as
	// silence: when reasoning ate the whole budget and no text row exists,
	// synthesize a warning row so the transcript says what happened.
	if m.State != StateStreaming && m.StopReason == StopMaxTokens &&
		(len(items) == 0 || items[len(items)-1].Kind != ItemAssistant) {
		items = append(items, Item{
			ID:    m.ID + "-truncated",
			Kind:  ItemAssistant,
			Text:  truncationWarningText,
			State: m.State,
		})
	}

	// End-of-round metadata rides the tail text row of terminal rounds only:
	// streaming shows a spinner, and a round ending on tool calls keeps its
	// tool rows last, so neither gets a dangling metadata line.
	if m.State != StateStreaming && len(items) > 0 && items[len(items)-1].Kind == ItemAssistant {
		items[len(items)-1].TurnMeta = TurnMeta{
			Model:     m.Model,
			Duration:  m.TurnDuration(),
			Usage:     m.Usage,
			Truncated: m.StopReason == StopMaxTokens,
		}
	}

	if m.State == StateCancelled {
		for i := range items {
			if items[i].Kind == ItemThinking {
				items[i].Streaming = false
				items[i].Interrupted = true
			}
			if items[i].Kind == ItemAssistant {
				items[i].State = StateCancelled
			}
		}
	}
	return items
}

// truncationWarningText is the transcript row shown when a round hit the
// provider's output-token limit before producing any visible text.
const truncationWarningText = "stopped at the model's output-token limit before producing a reply — raise `max_output_tokens` for this model in config.yaml"

func isTrailingThinking(blocks []ContentBlock, i int) bool {
	for j := i + 1; j < len(blocks); j++ {
		switch blocks[j].Type {
		case BlockThinking:
			return false
		case BlockText:
			if blocks[j].Text != "" {
				return false
			}
		case BlockToolUse:
			return false
		}
	}
	return true
}
