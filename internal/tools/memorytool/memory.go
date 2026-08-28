// Package memorytool exposes the agent's memory directory as one read-only
// tool. The system prompt carries what fits — the standing facts and a bounded
// list of the rest — and this is how the agent reaches everything else: the
// full catalog, and any memory in full by name.
//
// Writing stays with `write`. Remembering is writing a file, and a memory the
// agent can rewrite through a tool of its own is a memory the permission gate
// never sees.
package memorytool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/memory"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

const (
	// defaultLimit is a comfortable page of the catalog.
	defaultLimit = 40
	// maxLimit bounds what one call can spend of the context it is called from.
	maxLimit = 200
	// maxFactRunes bounds one memory. A file past this is not a fact any more,
	// and the model can still read the rest with `read`.
	maxFactRunes = 20000
)

const description = `Read and prune the Claude Code auto memory shared by both agents. Memories
are written with ` + "`write`" + `, one topic file per fact; this tool never writes one.

Actions:
- list: what is stored — name, kind and description, one per line. Pass
  query to rank the list against those words instead of listing in order;
  the system prompt shows only what fits in it, so this is how you see the
  rest.
- read: one memory in full, by name, including its frontmatter and links.
- overlaps: pairs of memories that say much the same thing. Merge a pair by
  writing one file that covers both, then forgetting the leftover.
- forget: move one memory to forgotten/. It leaves the prompt, retrieval and
  the index; the file stays on disk, so a wrong call is undone with a move.
  A pinned memory is refused — remove ` + "`pin: true`" + ` from it first.

Use list before saving a fact, to find the memory that already covers it,
and read whenever an index row looks like it applies to the work at hand.`

// Tool binds a memory store into the model-facing tool. A nil store yields a
// tool that reports an empty memory rather than failing — sub-agents carry no
// store, and neither does a session whose memory directory could not be
// opened.
func Tool(store *memory.Store) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "memory",
			Description: description,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"action": llm.Object{
						"type": "string",
						"enum": []string{"list", "read", "overlaps", "forget"},
						"description": "list (default): what is stored. read: one memory in full. " +
							"overlaps: merge candidates. forget: archive one memory.",
					},
					"name": llm.Object{
						"type":        "string",
						"description": "Memory name, as the index names it (read and forget actions).",
					},
					"query": llm.Object{
						"type":        "string",
						"description": "Rank the list against these words instead of listing in order (list action).",
					},
					"limit": llm.Object{
						"type":        "integer",
						"minimum":     1,
						"description": fmt.Sprintf("Rows to list; default %d, hard max %d.", defaultLimit, maxLimit),
					},
				},
			},
			Readable: true,
		},
		DetailFromArgs: detail,
		Run:            run(store),
	}
}

type input struct {
	Action string `json:"action"`
	Name   string `json:"name"`
	Query  string `json:"query"`
	Limit  *int   `json:"limit"`
	// PlanStep is injected by the plan gate and consumed before this tool runs;
	// it is accepted here so strict decoding never rejects a gate-valid call.
	PlanStep *int `json:"plan_step"`
}

func run(store *memory.Store) tooldef.Handler {
	return func(_ context.Context, raw json.RawMessage) (tooldef.Result, error) {
		in, err := parse(raw)
		if err != nil {
			return tooldef.Result{}, err
		}
		switch in.Action {
		case "read":
			return read(store, in.Name)
		case "overlaps":
			return overlaps(store, limitOf(in.Limit))
		case "forget":
			return forget(store, in.Name)
		default:
			return list(store, in.Query, limitOf(in.Limit))
		}
	}
}

func parse(raw json.RawMessage) (input, error) {
	in := input{Action: "list"}
	if err := tooldef.DecodeStrict(raw, &in); err != nil {
		return input{}, fmt.Errorf("memory: invalid arguments: %w", err)
	}
	in.Action = strings.ToLower(strings.TrimSpace(in.Action))
	if in.Action == "" {
		in.Action = "list"
	}
	switch in.Action {
	case "list", "read", "overlaps", "forget":
	default:
		return input{}, fmt.Errorf("memory: unknown action %q (use list, read, overlaps or forget)", in.Action)
	}
	if (in.Action == "read" || in.Action == "forget") && strings.TrimSpace(in.Name) == "" {
		return input{}, fmt.Errorf("memory: %s needs a name; call action=list to see what is stored", in.Action)
	}
	return in, nil
}

func limitOf(limit *int) int {
	if limit == nil || *limit <= 0 {
		return defaultLimit
	}
	return min(*limit, maxLimit)
}

func list(store *memory.Store, query string, limit int) (tooldef.Result, error) {
	stored := store.Budget().Facts
	rows := store.Search(query, limit)
	if len(rows) == 0 {
		if query != "" {
			return tooldef.Result{
				Content: fmt.Sprintf("No memory matches %q. %d stored; call again without a query to see them.",
					query, stored),
				Detail: "no match",
			}, nil
		}
		return tooldef.Result{Content: "Nothing is stored in memory yet.", Detail: "empty"}, nil
	}

	var sb strings.Builder
	if query != "" {
		fmt.Fprintf(&sb, "%d of %d memories, best match first for %q:\n\n", len(rows), stored, query)
	} else {
		fmt.Fprintf(&sb, "%d of %d memories:\n\n", len(rows), stored)
	}
	for _, entry := range rows {
		fmt.Fprintf(&sb, "- %s (%s) — %s\n", entry.Name, entry.Kind, oneLine(entry.Description))
	}
	if hidden := stored - len(rows); hidden > 0 && query == "" {
		fmt.Fprintf(&sb, "\n%d more; raise limit or pass a query to narrow it.\n", hidden)
	}
	sb.WriteString("\nRead one in full with action=read.")

	return tooldef.Result{Content: sb.String(), Detail: fmt.Sprintf("list (%d)", len(rows))}, nil
}

func read(store *memory.Store, name string) (tooldef.Result, error) {
	entry, ok := store.Fact(name)
	if !ok {
		return tooldef.Result{}, fmt.Errorf("memory: no memory named %q; call action=list to see what is stored", name)
	}
	content, err := os.ReadFile(entry.Path)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("memory: read %s: %w", entry.File, err)
	}
	// Reading a memory on purpose is the clearest signal that it still earns
	// its place; the stale list is built from the absence of it.
	store.Used(entry.Name)
	body := truncate(string(content), maxFactRunes)
	return tooldef.Result{
		Content: fmt.Sprintf("%s\n\n%s", entry.File, body),
		Detail:  entry.Name,
	}, nil
}

func overlaps(store *memory.Store, limit int) (tooldef.Result, error) {
	pairs := store.Overlaps(0, limit)
	if len(pairs) == 0 {
		return tooldef.Result{
			Content: "No two memories overlap enough to be worth merging.",
			Detail:  "no overlaps",
		}, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d pairs worth merging, closest first:\n\n", len(pairs))
	for _, pair := range pairs {
		fmt.Fprintf(&sb, "- %s + %s (%.2f)\n    %s\n    %s\n",
			pair.A.Name, pair.B.Name, pair.Similarity,
			oneLine(pair.A.Description), oneLine(pair.B.Description))
	}
	sb.WriteString("\nMerge a pair by writing one file that covers both, then forgetting the\n")
	sb.WriteString("leftover. Keep them apart if the difference is the point.")

	return tooldef.Result{Content: sb.String(), Detail: fmt.Sprintf("overlaps (%d)", len(pairs))}, nil
}

func forget(store *memory.Store, name string) (tooldef.Result, error) {
	entry, err := store.Forget(name)
	if err != nil {
		return tooldef.Result{}, err
	}
	return tooldef.Result{
		Content: fmt.Sprintf("Forgot %s. The file moved to forgotten/%s: it is out of the prompt, "+
			"out of retrieval and out of the index, but not off the disk.", entry.Name, entry.File),
		Detail: "forget " + entry.Name,
	}, nil
}

// detail is the row the TUI shows before the call runs.
func detail(raw json.RawMessage) string {
	in, err := parse(raw)
	if err != nil {
		return ""
	}
	switch in.Action {
	case "read", "forget":
		return in.Action + " " + in.Name
	case "overlaps":
		return "overlaps"
	}
	if in.Query != "" {
		return "list " + in.Query
	}
	return "list"
}

// oneLine keeps a row on one line whatever the description was written as.
func oneLine(text string) string {
	if text = strings.Join(strings.Fields(text), " "); text == "" {
		return "(no description)"
	}
	return text
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit])) + "\n… (truncated — read the file for the rest)"
}
