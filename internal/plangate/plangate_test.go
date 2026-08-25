package plangate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tools/tooldef"
)

func approved(items ...session.PlanItem) session.Plan {
	return session.Plan{Revision: 3, Approved: true, Items: items}
}

func step(status session.PlanStatus, typ session.StepType) session.PlanItem {
	return session.PlanItem{Content: "step", Status: status, Type: typ}
}

func TestCheckUnapprovedDenyBlocks(t *testing.T) {
	c := Checker{Phase: PhaseDeny}
	v := c.Check(session.Plan{Approved: false}, ToolCall{Name: "write"})
	assert.True(t, v.Miss)
	assert.True(t, v.Deny)
}

func TestCheckUnapprovedHintPasses(t *testing.T) {
	c := Checker{Phase: PhaseHint}
	v := c.Check(session.Plan{Approved: false}, ToolCall{Name: "write"})
	assert.False(t, v.Miss)
	assert.False(t, v.Deny)
}

func TestNewCheckerWiresPhase(t *testing.T) {
	require.Equal(t, PhaseDeny, NewChecker(PhaseDeny).Phase)
	require.Equal(t, PhaseHint, NewChecker(PhaseHint).Phase)
}

func TestCheckExemptToolsAlwaysPass(t *testing.T) {
	c := Checker{Phase: PhaseDeny}
	for _, name := range []string{"plan", "context"} {
		v := c.Check(approved(step(session.PlanInProgress, session.StepExplore)), ToolCall{Name: name})
		assert.False(t, v.Miss, name)
	}
}

func TestCheckMatchingToolPasses(t *testing.T) {
	c := Checker{Phase: PhaseDeny}
	cases := map[string]session.StepType{
		"read":        session.StepExplore,
		"write":       session.StepEdit,
		"bash":        session.StepRun,
		"agent_spawn": session.StepDelegate,
		"mcp_call":    session.StepIntegrate,
	}
	for tool, typ := range cases {
		v := c.Check(approved(step(session.PlanInProgress, typ)), ToolCall{Name: tool, PlanStep: 1})
		assert.False(t, v.Miss, tool)
	}
}

func TestCheckCumulativeLevels(t *testing.T) {
	c := Checker{Phase: PhaseDeny}
	// A run step may also read and edit; a delegate step may also bash.
	cases := map[string]session.StepType{
		"read":        session.StepRun,
		"edit":        session.StepRun,
		"bash":        session.StepDelegate,
		"agent_spawn": session.StepIntegrate,
		"mcp_call":    session.StepIntegrate,
	}
	for tool, typ := range cases {
		v := c.Check(approved(step(session.PlanInProgress, typ)), ToolCall{Name: tool, PlanStep: 1})
		assert.False(t, v.Miss, tool)
	}
}

func TestCheckMissCases(t *testing.T) {
	c := Checker{Phase: PhaseHint}
	active := approved(step(session.PlanInProgress, session.StepExplore))
	inactive := approved(step(session.PlanCompleted, session.StepExplore))
	cases := map[string]struct {
		plan session.Plan
		call ToolCall
	}{
		"missing step":  {active, ToolCall{Name: "read"}},
		"out of range":  {active, ToolCall{Name: "read", PlanStep: 2}},
		"inactive step": {inactive, ToolCall{Name: "read", PlanStep: 1}},
		"wrong tool":    {active, ToolCall{Name: "bash", PlanStep: 1}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			v := c.Check(tc.plan, tc.call)
			assert.True(t, v.Miss, name)
			assert.NotEmpty(t, v.Reason)
			assert.NotEmpty(t, v.Hint)
			assert.False(t, v.Deny, "hint phase must not block")
		})
	}
}

func TestCheckDenyPhaseBlocks(t *testing.T) {
	c := Checker{Phase: PhaseDeny}
	v := c.Check(approved(step(session.PlanInProgress, session.StepExplore)), ToolCall{Name: "bash", PlanStep: 1})
	assert.True(t, v.Miss)
	assert.True(t, v.Deny)
}

func TestCheckUntypedStepAllowsAnyTool(t *testing.T) {
	c := Checker{Phase: PhaseDeny}
	v := c.Check(approved(step(session.PlanInProgress, "")), ToolCall{Name: "bash", PlanStep: 1})
	assert.False(t, v.Miss)
}

func TestCheckPendingStepIsNotActive(t *testing.T) {
	c := Checker{Phase: PhaseDeny}
	v := c.Check(approved(step(session.PlanPending, session.StepExplore)), ToolCall{Name: "read", PlanStep: 1})
	assert.True(t, v.Miss)
	assert.True(t, v.Deny)
}

func TestPromptBlockRequiresInProgressStep(t *testing.T) {
	block := PromptBlock(PhaseDeny)
	assert.Contains(t, block, "in_progress")
	assert.NotContains(t, block, "pending or in_progress")
}

func TestInjectPlanStep(t *testing.T) {
	mk := func(name string) tooldef.Tool {
		return tooldef.Tool{Definition: llm.ToolDefinition{
			Name: name,
			Params: &llm.FunctionParameters{
				Type:       "object",
				Properties: llm.Object{"path": llm.Object{"type": "string"}},
			},
		}}
	}
	out := InjectPlanStep([]tooldef.Tool{mk("read"), mk("plan"), mk("context")})
	read, plan, ctx := out[0], out[1], out[2]

	_, ok := read.Definition.Params.Properties["plan_step"]
	assert.True(t, ok, "read must gain plan_step")
	_, ok = plan.Definition.Params.Properties["plan_step"]
	assert.False(t, ok, "plan is exempt")
	_, ok = ctx.Definition.Params.Properties["plan_step"]
	assert.False(t, ok, "context is exempt")
}

func TestRecorderAppendsJSONLines(t *testing.T) {
	dir := t.TempDir()
	r, err := NewRecorder(dir)
	require.NoError(t, err)
	require.NoError(t, r.Record(Miss{
		Session: "s1", Tool: "bash", PlanStep: 1, PlanRevision: 3,
		StepStatus: "in_progress", StepType: "explore", Reason: "wrong tool", Phase: "hint",
	}))
	require.NoError(t, r.Record(Miss{
		Session: "s1", Tool: "read", PlanRevision: 3, Reason: "plan_step omitted", Phase: "hint",
	}))

	raw, err := os.ReadFile(filepath.Join(dir, "plan-gate-misses.jsonl"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	require.Len(t, lines, 2)
	var first Miss
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
	assert.Equal(t, "bash", first.Tool)
	assert.Equal(t, "explore", first.StepType)
}
