package agent

import (
	"strings"
	"testing"
)

func TestPromptAgentsEnabledToggle(t *testing.T) {
	with := Prompt("", true)
	without := Prompt("", false)

	if !strings.Contains(with, "agent_task") {
		t.Fatal("expected agent_task guidance when agents enabled")
	}
	if !strings.Contains(with, "Sub-agents:") {
		t.Fatal("expected Sub-agents section when agents enabled")
	}
	if strings.Contains(without, "agent_task") || strings.Contains(without, "agent_spawn") {
		t.Fatal("did not expect sub-agent tool names when agents disabled")
	}
	if !strings.Contains(without, "`glob` / `grep` / `list` yourself") {
		t.Fatal("expected direct-search guidance when agents disabled")
	}
}
