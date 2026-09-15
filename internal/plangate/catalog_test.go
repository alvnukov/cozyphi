package plangate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plangate"
)

func TestKnownToolsListsGateableAndMandatoryToolsInStableOrder(t *testing.T) {
	got := plangate.KnownTools()
	names := make([]string, len(got))
	mandatory := make([]string, 0)
	for i, tool := range got {
		names[i] = tool.Name
		if tool.MandatoryExemption {
			mandatory = append(mandatory, tool.Name)
		}
	}

	assert.Equal(t, []string{
		"read", "grep", "find", "ls", "lsp",
		"write", "edit", "bash", "watch",
		"agent_spawn", "agent_wait", "agent_list", "agent_cancel",
		"mcp_list", "mcp_inspect", "mcp_call",
		"plan", "context", "harness", "memory", "question", "session", "shell_task", "task",
	}, names)
	assert.Equal(
		t,
		[]string{
			"plan",
		},
		mandatory,
	)

	// Exemption-only tools carry no capability rank, so an editor must never
	// offer them a step type; watch is ranked and stays assignable.
	exemptionOnly := make([]string, 0)
	for _, tool := range got {
		if tool.ExemptionOnly {
			exemptionOnly = append(exemptionOnly, tool.Name)
		}
	}
	assert.Equal(
		t,
		[]string{"context", "harness", "memory", "question", "session", "shell_task", "task"},
		exemptionOnly,
	)

	require.NotEmpty(t, got)
	got[0].Name = "changed"
	assert.Equal(t, "read", plangate.KnownTools()[0].Name, "callers receive a detached catalog")
}
