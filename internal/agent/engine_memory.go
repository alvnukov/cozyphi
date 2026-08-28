package agent

import (
	"regexp"
	"strings"

	"github.com/alvnukov/cozyphi/internal/debuglog"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/memory"
)

const (
	// memoryLookback is how far back a recall query reads for what the turn is
	// about: enough for the previous exchange and the files it touched.
	memoryLookback = 8
	// memoryQueryPrompts bounds how many earlier user messages join the query.
	memoryQueryPrompts = 2
	// memoryTouchLookback is how far back the check for a memory write reads:
	// a turn's worth of tool rounds, and anything older is caught by the
	// store's own periodic scan.
	memoryTouchLookback = 256
)

// memoryTouched reports whether a tool call named the memory directory. It is
// the difference between re-checking every file after every turn and doing it
// only when the agent may have written a memory.
func (engine *Engine) memoryTouched() bool {
	dir := engine.memory.Dir()
	if dir == "" {
		return false
	}
	messages := engine.sessionRef().BuildContext()
	for i := len(messages) - 1; i >= 0 && len(messages)-i <= memoryTouchLookback; i-- {
		for _, call := range messages[i].ToolCalls {
			if strings.Contains(call.Function.Arguments, dir) {
				return true
			}
		}
	}
	return false
}

// toolPathPattern pulls file paths out of a tool call's arguments without
// knowing any tool's schema: what touches a file names it "path" or "file".
var toolPathPattern = regexp.MustCompile(`"(?:path|file)"\s*:\s*"([^"]+)"`)

// memoryQuery describes the turn to recall: what the user just asked, plus the
// prompts before it and the paths this session's tools have been touching —
// which is what a project memory is usually named by, and what the user's own
// wording often leaves out.
func (engine *Engine) memoryQuery(prompt string) memory.Query {
	query := memory.Query{Prompt: prompt}
	messages := engine.sessionRef().BuildContext()
	prompts := 0
	for i := len(messages) - 1; i >= 0 && len(messages)-i <= memoryLookback; i-- {
		message := messages[i]
		if message.Role == llm.RoleUser {
			if prompts < memoryQueryPrompts {
				// Stripped: a block recall wrote must not feed itself back in.
				query.Recent = append(query.Recent, memory.StripReminders(message.Content))
				prompts++
			}
			continue
		}
		for _, call := range message.ToolCalls {
			for _, match := range toolPathPattern.FindAllStringSubmatch(call.Function.Arguments, -1) {
				query.Recent = append(query.Recent, match[1])
			}
		}
	}
	return query
}

// prependReminder puts a recall block in front of the user's text, so the
// model reads the remembered context before the request it applies to.
func prependReminder(reminder, content string) string {
	if reminder == "" {
		return content
	}
	if content == "" {
		return reminder
	}
	return reminder + "\n\n" + content
}

// syncMemory refreshes MEMORY.md when a turn ends, however it ended, and
// rebinds the client when what memory contributes to the system prompt has
// changed — a fact written this turn has to reach the next one. Failure is
// logged, never fatal: memory is an accessory to a turn, not a precondition
// for one.
func (engine *Engine) syncMemory() {
	if engine == nil || engine.memory == nil {
		return
	}
	// A memory rewritten in place moves nothing the store's cheap staleness
	// check can see, so a turn that touched the directory says so. A turn that
	// did not costs one stat, whatever the directory holds.
	if engine.memoryTouched() {
		engine.memory.Invalidate()
	}
	// Exact duplicates — one fact saved twice under two names — are archived
	// here. It is the only compaction the harness performs by itself, because
	// it is the only one that cannot lose anything.
	if archived := engine.memory.Compact(); len(archived) > 0 {
		debuglog.Logf("memory: archived duplicates: %v", archived)
	}
	if _, err := engine.memory.SyncIndex(); err != nil {
		debuglog.Logf("memory: sync index: %v", err)
	}
	engine.mu.Lock()
	promptStale := engine.memory.PromptBlock() != engine.memoryPrompt
	if promptStale {
		engine.rebindClient(engine.buildToolList())
	}
	engine.mu.Unlock()
}
