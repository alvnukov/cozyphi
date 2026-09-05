package agent

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// Integration for the plan gate's auto-binding: a call whose plan_step does
// not resolve, on a plan with exactly one startable compatible step, runs
// through the same machinery an explicit plan_step uses — auto-start, skill
// preload choreography, attempt evidence — and an ambiguous plan is refused
// with the bounded candidate list.

type autobindFixture struct {
	ex     *Executor
	plan   session.Plan
	ran    atomic.Int32
	starts []string
	skills func() (string, bool)
}

func newAutobindFixture(t *testing.T, items ...session.PlanItem) *autobindFixture {
	t.Helper()
	f := &autobindFixture{plan: session.Plan{Revision: 1, Approved: true, Items: items}}
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				f.ran.Add(1)
				return tools.Result{Content: "ok"}, nil
			},
		},
	}
	f.ex = NewExecutor(reg, permission.AllowAll{}, nil, nil)
	f.ex.SetPlanGate(
		&plangate.Checker{Phase: plangate.PhaseDeny},
		func() session.Plan { return f.plan },
		func(_ context.Context, stepID string) error {
			f.starts = append(f.starts, stepID)
			for i := range f.plan.Items {
				if f.plan.Items[i].ID == stepID {
					f.plan.Items[i].Status = session.PlanInProgress
				}
			}
			return nil
		},
		nil,
		nil,
		nil,
	)
	f.ex.SetPlanSkillDrain(func() (string, bool) {
		if f.skills != nil {
			return f.skills()
		}
		return "", false
	})
	return f
}

func TestExecutorAutoBindRunsAndStartsUniqueStep(t *testing.T) {
	f := newAutobindFixture(t,
		session.PlanItem{ID: "survey", Content: "look", Status: session.PlanInProgress, Type: session.StepExplore},
		session.PlanItem{ID: "ship", Content: "run", Status: session.PlanPending, Type: session.StepRun},
	)

	var statuses []session.ToolStatus
	msgs, _, _ := f.ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"make test"}`},
	}}, func(td session.ToolData) bool {
		statuses = append(statuses, td.Run.Status)
		return true
	})

	require.Equal(t, int32(1), f.ran.Load(), "the unique candidate takes the call")
	require.Equal(t, []string{"ship"}, f.starts, "auto-bind uses the regular pending auto-start")
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "ok")
	assert.Contains(t, msgs[0].Content, `auto-bound to "ship"`, "the model is told which step took the call")
	assert.NotContains(t, statuses, session.ToolRejected)
}

func TestExecutorAutoBindAmbiguousRefusesWithCandidateList(t *testing.T) {
	f := newAutobindFixture(t,
		session.PlanItem{ID: "run-a", Content: "run", Status: session.PlanInProgress, Type: session.StepRun},
		session.PlanItem{ID: "run-b", Content: "run more", Status: session.PlanPending, Type: session.StepRun},
	)

	var statuses []session.ToolStatus
	msgs, _, _ := f.ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"make test"}`},
	}}, func(td session.ToolData) bool {
		statuses = append(statuses, td.Run.Status)
		return true
	})

	require.Equal(t, int32(0), f.ran.Load(), "the gate does not guess between candidates")
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "compatible steps")
	assert.Contains(t, msgs[0].Content, "run-a (run, in_progress)")
	assert.Contains(t, msgs[0].Content, "run-b (run, pending)")
	assert.Contains(t, statuses, session.ToolRejected)
	assert.Empty(t, f.starts)
}

// Auto-binding must ride the skill-preload choreography: starting the bound
// pending step fires inject_skill, the first attempt is refused as service
// choreography, and the retry runs under the preloaded guidance.
func TestExecutorAutoBindSkillPreloadRefusesThenRetryRuns(t *testing.T) {
	f := newAutobindFixture(t,
		session.PlanItem{ID: "survey", Content: "look", Status: session.PlanInProgress, Type: session.StepExplore},
		session.PlanItem{ID: "ship", Content: "run", Status: session.PlanPending, Type: session.StepRun},
	)
	preloaded := atomic.Bool{}
	preloaded.Store(true)
	f.skills = func() (string, bool) {
		if preloaded.Swap(false) {
			return "## Skill: tdd\nWrite the failing test first.", true
		}
		return "", false
	}

	var statuses []session.ToolStatus
	msgs, _, _ := f.ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"go test ./..."}`},
	}}, func(td session.ToolData) bool {
		statuses = append(statuses, td.Run.Status)
		return true
	})

	require.Len(t, msgs, 1)
	require.Contains(t, statuses, session.ToolRejected, "the skill preload refuses the first attempt")
	assert.Contains(t, msgs[0].Content, plangate.ReasonSkillPreload, "the refusal carries the preload")
	require.Equal(t, int32(0), f.ran.Load())
	require.Equal(t, []string{"ship"}, f.starts, "the bound step still started")

	retry, _, _ := f.ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c2",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"go test ./..."}`},
	}}, func(td session.ToolData) bool {
		statuses = append(statuses, td.Run.Status)
		return true
	})

	require.Len(t, retry, 1)
	assert.Contains(t, retry[0].Content, "ok", "the retry dispatches under the preloaded guidance")
	require.Equal(t, int32(1), f.ran.Load())
}
