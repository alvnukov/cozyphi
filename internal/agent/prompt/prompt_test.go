package prompt

import (
	"strings"
	"testing"
)

func TestBuildAgentsEnabledToggle(t *testing.T) {
	with := Build("", true, false, nil, false)
	without := Build("", false, false, nil, false)

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
	got := Build("", false, false, nil, false)
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
	none := Build("", false, false, nil, false)
	if strings.Contains(none, "# MCP") {
		t.Fatal("expected no MCP section without servers")
	}
	if strings.Contains(none, "External docs/URLs") {
		t.Fatal("Discovery must not mention MCP/URLs when no servers are configured")
	}

	got := Build("", false, false, []string{"browsermcp", "github"}, false)
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
