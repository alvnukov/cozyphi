// Package contexttool provides the model-facing context tool: a quantitative
// usage report plus an explicit handle to request context compaction, so the
// model can free its own window mid-task instead of dying on overflow.
package contexttool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// Stats is a quantitative snapshot of the model context. Numbers only —
// conversation content never rides through here.
type Stats struct {
	// ContextTokens is the best known token count occupying the window.
	ContextTokens int
	// TokenSource is "provider" (endpoint-reported) or "estimate"
	// (serialized-bytes heuristic, used before the first usage report).
	TokenSource string
	// UsedBytes is the serialized size of the model-view messages.
	UsedBytes int
	// Messages is the number of messages in the model view.
	Messages int
	// ContextWindow is the model context window in tokens; 0 = unknown.
	ContextWindow int
	// ThresholdTokens is the usage at which auto-compaction fires; 0 = unknown.
	ThresholdTokens int
	// CompactionRecommended mirrors the auto-compaction decision.
	CompactionRecommended bool
	// MicroElidedResults is the number of old tool results currently stubbed
	// by provider-view microcompaction; 0 = full fidelity in the window.
	MicroElidedResults int
	// MicroElidedBytes is the content bytes those stubs removed.
	MicroElidedBytes int
}

// Deps wires the tool to the engine. Both funcs are read at call time so
// session swaps (/resume) are picked up without rebuilding the tool.
type Deps struct {
	// Stats reports the current usage snapshot.
	Stats func() Stats
	// RequestCompact asks the engine to compact at the next tool-round
	// boundary. It returns an error the model can act on (nothing to
	// compact, already scheduled).
	RequestCompact func() error
}

const toolDescription = `Report your context window usage and optionally request compaction.

Call with no arguments (action=status) to get the numbers: context tokens, serialized kilobytes, message count, context window, compact threshold, and whether compaction is recommended. The report is quantitative only — it never echoes conversation content.

Call with action=compact to compact: older history is replaced by a generated summary, recent messages are kept verbatim, and the on-disk transcript is never deleted. Compaction runs at the end of the current tool round, so continue working afterwards with the freed space.

Check usage before starting large tasks and compact when usage is high — running out of context window interrupts your work mid-task.`

// Tools returns the context tool.
func Tools(deps Deps) tooldef.Tool {
	if deps.Stats == nil {
		deps.Stats = func() Stats { return Stats{} }
	}
	if deps.RequestCompact == nil {
		deps.RequestCompact = func() error { return errors.New("compaction unavailable on this engine") }
	}
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "context",
			Description: toolDescription,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"action": llm.Object{
						"type":        "string",
						"enum":        []string{"status", "compact"},
						"description": `status (default): report usage numbers. compact: request compaction of older history at the end of this tool round.`,
					},
				},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in struct {
				Action string `json:"action"`
			}
			_ = json.Unmarshal(input, &in)
			if strings.TrimSpace(in.Action) == "compact" {
				return "compact"
			}
			return "status"
		},
		Run: func(_ context.Context, input json.RawMessage) (tooldef.Result, error) {
			var in struct {
				Action string `json:"action"`
			}
			if len(input) > 0 {
				if err := json.Unmarshal(input, &in); err != nil {
					return tooldef.Result{}, fmt.Errorf("context args: %w", err)
				}
			}
			switch strings.TrimSpace(in.Action) {
			case "", "status":
				return statusResult(deps.Stats()), nil
			case "compact":
				if err := deps.RequestCompact(); err != nil {
					return tooldef.Result{}, err
				}
				body := mustJSON(map[string]any{
					"status": "scheduled",
					"note":   "compaction runs at the end of this tool round; older history becomes a summary, recent messages stay verbatim",
				})
				return tooldef.Result{Content: body, Detail: "compact", Output: body}, nil
			default:
				return tooldef.Result{}, fmt.Errorf(
					"unknown context action %q: use \"status\" or \"compact\"",
					in.Action,
				)
			}
		},
	}
}

func statusResult(s Stats) tooldef.Result {
	body := mustJSON(map[string]any{
		"context_tokens":           s.ContextTokens,
		"token_source":             s.TokenSource,
		"context_kb":               kilobytes(s.UsedBytes),
		"messages":                 s.Messages,
		"context_window":           s.ContextWindow,
		"compact_threshold_tokens": s.ThresholdTokens,
		"compaction_recommended":   s.CompactionRecommended,
		"micro_elided_results":     s.MicroElidedResults,
		"micro_elided_bytes":       s.MicroElidedBytes,
		"note":                     s.note(),
	})
	detail := fmt.Sprintf("%s tokens · %.1f KB", s.TokenSource, kilobytes(s.UsedBytes))
	if s.ContextTokens > 0 {
		detail = fmt.Sprintf("%d tokens · %.1f KB", s.ContextTokens, kilobytes(s.UsedBytes))
	}
	if s.MicroElidedResults > 0 {
		detail += fmt.Sprintf(" · %d results microcompacted", s.MicroElidedResults)
	}
	return tooldef.Result{Content: body, Detail: detail, Output: body}
}

func (s Stats) note() string {
	switch {
	case s.ContextWindow <= 0:
		return "context window size unknown: threshold and recommendation unavailable; compact manually if tool results keep growing"
	case s.CompactionRecommended:
		return `usage is above the compact threshold — call {"action":"compact"} now, then continue`
	default:
		return "usage is below the compact threshold"
	}
}

func kilobytes(n int) float64 {
	return math.Round(float64(n)/1024*10) / 10
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
