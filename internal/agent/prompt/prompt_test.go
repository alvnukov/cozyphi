package prompt

import (
	"strings"
	"testing"
)

func TestBuildCarriesCompactionReminderPolicy(t *testing.T) {
	got := Build(Options{})
	if !strings.Contains(got, "# Compaction reminders") {
		t.Fatal("expected the standing compaction-reminder policy section")
	}
	if !strings.Contains(got, `Call the context tool with {"action":"compact"}`) {
		t.Fatal("expected the policy to name the compact call")
	}
	if !strings.Contains(got, "workingContext or session notes") {
		t.Fatal("expected the policy to point at the durable context")
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
	if strings.Contains(without, "agent_spawn") {
		t.Fatal("did not expect sub-agent tool names when agents disabled")
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
