package prompt

import (
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/tasks"
)

func TestBuildCarriesCompactionReminderPolicy(t *testing.T) {
	got := Build(Options{})
	if !strings.Contains(got, "# Compaction reminders") {
		t.Fatal("expected the standing compaction-reminder policy section")
	}
	if !strings.Contains(got, `Call the context tool with {"action":"compact"}`) {
		t.Fatal("expected the policy to name the compact call")
	}
	if !strings.Contains(got, "last assistant message: recent messages survive compaction verbatim") {
		t.Fatal("expected the policy to point at the last assistant message, which compaction keeps verbatim")
	}
}

func TestBuildAgentsEnabledToggle(t *testing.T) {
	with := Build(Options{Agents: true})
	without := Build(Options{})

	if !strings.Contains(with, "agent_spawn") {
		t.Fatal("expected agent_spawn guidance when agents enabled")
	}
	if !strings.Contains(with, "Sub-agents:") {
		t.Fatal("expected Sub-agents section when agents enabled")
	}
	// The explicit-skills rule rides the same section: every agent_spawn
	// decides skills, and a no-skill spawn says why.
	if !strings.Contains(with, "no_skill_reason") {
		t.Fatal("expected the agent_spawn skills rule when agents enabled")
	}
	// The tool name is checked backticked: a plain substring would also
	// match a checkout path that happens to contain "agent_spawn".
	if strings.Contains(without, "`agent_spawn`") {
		t.Fatal("did not expect sub-agent tool names when agents disabled")
	}
	if strings.Contains(without, "no_skill_reason") {
		t.Fatal("did not expect sub-agent skills guidance when agents disabled")
	}
	if !strings.Contains(without, "`find` / `grep` / `ls` yourself") {
		t.Fatal("expected direct-search guidance when agents disabled")
	}
}

func TestBuildEditHashCopyIsUnambiguous(t *testing.T) {
	got := Build(Options{})
	if strings.Contains(got, "copy `@file path#TAG` into") {
		t.Fatal("prompt must not tell the model to paste the whole @file header into edit.hash")
	}
	if !strings.Contains(got, "4 hex chars after `#`") {
		t.Fatal("expected edit.hash to be described as the 4 hex chars after #")
	}
	if strings.Contains(got, "Known path or exact symbol") {
		t.Fatal("known-path routing must not bundle ls/find/grep/read together")
	}
	if strings.Contains(got, "creates a new file only") || strings.Contains(got, "fails if it already exists") {
		t.Fatal("write must not be described as create-only")
	}
	if !strings.Contains(got, "`write` creates or overwrites") {
		t.Fatal("expected write to create or overwrite")
	}
	if !strings.Contains(got, "Prefer cwd-relative paths") {
		t.Fatal("expected tools to prefer cwd-relative paths")
	}
}

func TestBuildMCPCatalog(t *testing.T) {
	none := Build(Options{})
	if strings.Contains(none, "# MCP") {
		t.Fatal("expected no MCP section without servers")
	}
	if strings.Contains(none, "External docs/URLs") {
		t.Fatal("Discovery must not mention MCP/URLs when no servers are configured")
	}

	got := Build(Options{MCPServers: []string{"browsermcp", "github"}})
	if !strings.Contains(got, "# MCP") {
		t.Fatal("expected MCP section")
	}
	if !strings.Contains(got, "- browsermcp") || !strings.Contains(got, "- github") {
		t.Fatalf("expected server names in catalog, got:\n%s", got)
	}
	if !strings.Contains(got, "mcp_list") || !strings.Contains(got, "mcp_inspect") ||
		!strings.Contains(got, "mcp_call") {
		t.Fatal("expected mcp_* usage guidance")
	}
	if !strings.Contains(got, "docs/URLs") {
		t.Fatal("expected MCP block to mention external docs/URLs when servers are configured")
	}
	if strings.Contains(got, `"properties"`) || strings.Contains(got, "inputSchema") {
		t.Fatal("MCP catalog must not include tool schemas")
	}
}

// TestWatchRoutingFollowsTheTool pins the rule the whole # Tools section
// exists for: the prompt names a tool only when the engine carries it. A
// sub-agent or a headless run gets no watch manager, so telling it to reach
// for `watch` would be advice it cannot take.
func TestWatchRoutingFollowsTheTool(t *testing.T) {
	with := Build(Options{Watches: true})
	if !strings.Contains(with, "never poll it") {
		t.Fatalf("a session with watches must be told to use them:\n%s", with)
	}
	if !strings.Contains(with, "`watch`") {
		t.Fatalf("the routing must name the tool:\n%s", with)
	}

	without := Build(Options{})
	if strings.Contains(without, "`watch`") {
		t.Fatalf("no manager, no mention:\n%s", without)
	}
}

// TestTaskRoutingFollowsTheTool applies the same rule to the task tool: a
// session with a registry is told how to work it at the level the user
// set, one without (or with the registry off) hears nothing.
func TestTaskRoutingFollowsTheTool(t *testing.T) {
	with := Build(Options{Tasks: tasks.AccessWrite})
	if !strings.Contains(with, "`task`") || !strings.Contains(with, "`current` before choosing work") {
		t.Fatalf("a session with a registry must be told the task workflow:\n%s", with)
	}
	if strings.Contains(with, "asks the user first") {
		t.Fatal("write asks nobody")
	}
	ask := Build(Options{Tasks: tasks.AccessAsk})
	if !strings.Contains(ask, "`start` when you take one") || !strings.Contains(ask, "asks the user first") {
		t.Fatalf("ask keeps the workflow and says each write is a question:\n%s", ask)
	}
	read := Build(Options{Tasks: tasks.AccessRead})
	if !strings.Contains(read, "read but not change") || strings.Contains(read, "`start` when you take one") {
		t.Fatalf("read offers reads only and the way out:\n%s", read)
	}
	for _, silent := range []tasks.Access{"", tasks.AccessOff} {
		if strings.Contains(Build(Options{Tasks: silent}), "`task`") {
			t.Fatalf("level %q: no registry, no mention", silent)
		}
	}
}

// TestPlanAppendixShapesTasksWhenWritable pins the plan-mode rule: with a
// writable registry the appendix says shaping tasks is planning, so the
// model does not read rule 2 as a ban on the registry; a read-only one
// gets no such rule.
func TestPlanAppendixShapesTasksWhenWritable(t *testing.T) {
	for level, want := range map[tasks.Access]bool{
		tasks.AccessWrite: true,
		tasks.AccessAsk:   true,
		tasks.AccessRead:  false,
		tasks.AccessOff:   false,
	} {
		got := strings.Contains(Build(Options{Tasks: level, Plan: true}), "planning material")
		if got != want {
			t.Fatalf("level %q: plan appendix mentions the registry = %v, want %v", level, got, want)
		}
	}
}
