package agent

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/memory"
	"github.com/alvnukov/cozyphi/internal/session"
)

// Only previously referenced import names are useful for an anaphoric request.
// Tool results or code from a different turn must never become advisor input.
var libraryImport = regexp.MustCompile(`cozy:lib/[a-z][a-z0-9-]{0,62}`)

type pendingCodeHints <-chan string

// Only immutable request data moves to the worker while the turn prepares
// skills and memory. The consumer owns the seen set and all transcript writes.
func (engine *Engine) startCodeHints(ctx context.Context, task string, eligible bool) pendingCodeHints {
	if !eligible || ctx.Err() != nil || strings.TrimSpace(task) == "" {
		return nil
	}
	engine.mu.RLock()
	mode := engine.codeMode
	if mode == nil || mode.Hints == nil || engine.executor == nil {
		engine.mu.RUnlock()
		return nil
	}
	ex := engine.executor
	sess := engine.session
	id, cwd := ex.sessionID, ex.cwd
	defs := make([]llm.ToolDefinition, 0, len(ex.registry))
	for _, t := range ex.registry {
		defs = append(defs, t.Definition)
	}
	engine.mu.RUnlock()
	slices.SortFunc(defs, func(a, b llm.ToolDefinition) int { return cmp.Compare(a.Name, b.Name) })
	var query strings.Builder
	query.WriteString(task)
	if sess != nil {
		p := sess.Plan()
		for _, step := range p.Items {
			if step.Status == session.PlanInProgress {
				query.WriteString("\nCurrent plan step: " + step.Content)
				break
			}
		}
		if refersToPriorFunction(task) {
			if prev := recentLibraryImport(sess.BuildContext()); prev != "" {
				query.WriteString("\nPreviously used package: " + prev)
			}
		}
	}
	result := make(chan string, 1)
	go func() { result <- mode.Hints(ctx, id, cwd, query.String(), defs) }()
	return result
}

func (pending pendingCodeHints) reminder(ctx context.Context, seen map[string]bool) string {
	if pending == nil {
		return ""
	}
	var raw string
	select {
	case <-ctx.Done():
		return ""
	case raw = <-pending:
	}
	if ctx.Err() != nil {
		return ""
	}
	var out []string
	for line := range strings.SplitSeq(raw, "\n") {
		if line != "" && !seen[line] {
			out = append(out, line)
			seen[line] = true
		}
	}
	if len(out) == 0 {
		return ""
	}
	return memory.ReminderOpen + "\nOptional code references (catalog data). Use any eligible tool or your own code as appropriate.\n" +
		strings.Join(
			out,
			"\n",
		) + "\n</system-reminder>"
}

func refersToPriorFunction(task string) bool {
	q := " " + strings.ToLower(task) + " "
	for _, s := range []string{" it ", " the one ", " previous ", " last ", " её ", " ее ", " эту ", " прошл", " предыдущ", " последн"} {
		if strings.Contains(q, s) {
			return true
		}
	}
	return false
}

func recentLibraryImport(msgs []llm.Message) string {
	for i := len(msgs) - 1; i >= 0 && i >= len(msgs)-8; i-- {
		if msgs[i].Role != llm.RoleAssistant {
			continue
		}
		for _, toolCall := range slices.Backward(msgs[i].ToolCalls) {
			call := toolCall.Function
			if call.Name == "code" {
				if name := libraryImport.FindString(call.Arguments); name != "" {
					return name
				}
			}
		}
	}
	return ""
}
