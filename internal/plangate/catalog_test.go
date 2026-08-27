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
		"write", "edit", "bash",
		"agent_spawn", "agent_wait", "agent_list", "agent_cancel",
		"mcp_list", "mcp_inspect", "mcp_call",
		"plan", "context", "question", "watch", "memory",
	}, names)
	assert.Equal(t, []string{"plan", "context", "question", "watch", "memory"}, mandatory)

	require.NotEmpty(t, got)
	got[0].Name = "changed"
	assert.Equal(t, "read", plangate.KnownTools()[0].Name, "callers receive a detached catalog")
}
