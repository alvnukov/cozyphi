package sessions

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func TestModelStateLabelDistinguishesPendingAndPlanSelection(t *testing.T) {
	selected := agent.ModelSelection{Name: "chosen", Effort: "low"}
	effective := agent.ModelSelection{Name: "running", Effort: "high"}
	status := controller.ModelSelectionStatus{
		Selected:    selected,
		ModelStatus: agent.ModelStatus{Next: selected, Effective: effective, Pending: true},
	}
	require.Equal(t, "next; running running[high]", modelStateLabel(status))
	status.Next = agent.ModelSelection{Name: "plan"}
	require.Equal(t, "next; running running[high]; selected chosen[low]", modelStateLabel(status))
	status.Next, status.Effective, status.Pending = selected, selected, false
	require.Empty(t, modelStateLabel(status))
}
