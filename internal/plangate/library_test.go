package plangate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

func TestBoundedCodeDelegatesThePlanToActualOperations(t *testing.T) {
	p := plangate.DefaultPolicy()
	for _, n := range []string{"code", "code_wait", "code_test"} {
		require.False(t, p.Check(plangate.PhaseDeny, session.Plan{}, plangate.ToolCall{Name: n}).Deny)
	}
	require.True(t, p.Check(plangate.PhaseDeny, session.Plan{}, plangate.ToolCall{Name: "write"}).Deny)
	require.True(
		t,
		p.Check(plangate.PhaseDeny, session.Plan{}, plangate.ToolCall{Name: "library", Action: "open"}).Deny,
	)
	for _, op := range tooldef.LibraryActions(true) {
		step := session.PlanItem{ID: "scan", Content: "look", Type: session.StepExplore, Status: session.PlanInProgress}
		plan := session.Plan{Approved: true, Items: []session.PlanItem{step}}
		call := plangate.ToolCall{Name: "library", Action: op, Step: plangate.StepRef{ID: "scan"}}
		v := p.Check(plangate.PhaseDeny, plan, call)
		require.Equal(t, !tooldef.LibraryReadAction(op), v.Deny, op)
	}
}
