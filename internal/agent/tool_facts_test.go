package agent_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// recordingGate is a boundary that would notice any decision asked of it.
// Observing the tool layer must leave its count at zero: a view that had to
// evaluate a call in order to describe one would be a second permission
// evaluator, which is exactly what this category must not become.
type recordingGate struct{ calls int }

func (g *recordingGate) Check(context.Context, permission.Request) (permission.Decision, string) {
	g.calls++
	return permission.Allow, ""
}

// primaryEngine is the ordinary session: default tools, a plan, no manager
// attached beyond what NewEngine builds itself.
func primaryEngine(t *testing.T, opts agent.EngineOpts) *agent.Engine {
	t.Helper()
	opts.Model = llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"}
	if opts.SessionOpts.Cwd == "" {
		opts.SessionOpts.Cwd = t.TempDir()
	}
	eng, err := agent.NewEngine(opts)
	require.NoError(t, err)
	return eng
}

func factsFor(t *testing.T, state diag.ToolState, name string) diag.ToolFacts {
	t.Helper()
	for _, facts := range state.Tools {
		if facts.Name == name {
			return facts
		}
	}
	t.Fatalf("the catalog tool %q was not answered for", name)
	return diag.ToolFacts{}
}

func TestToolObservationAnswersForTheWholeCatalogExactlyOnce(t *testing.T) {
	state := primaryEngine(t, agent.EngineOpts{}).ToolObservation()

	require.True(t, state.Known)
	seen := make(map[string]int, len(state.Tools))
	for _, facts := range state.Tools {
		seen[facts.Name]++
	}
	for _, name := range diag.ToolCatalog() {
		assert.Equal(t, 1, seen[name], "%s must be answered for once", name)
	}
	assert.Len(t, state.Tools, len(diag.ToolCatalog()),
		"the engine answers the catalog and invents nothing beyond it")
}

func TestAdmittedIsTheToolListThisSessionWouldAssemble(t *testing.T) {
	// The projection derives the admitted set from the engine's own fields
	// rather than by building a tool list. This is the contract that keeps
	// the two from drifting apart.
	for _, wired := range []agent.EngineOpts{
		{},
		{Tools: tools.ReadonlyTools()},
		{SessionOpts: agent.SessionOpts{ParentID: "parent"}},
		{QuestionAsk: func(context.Context, []tools.Question) ([]tools.QuestionAnswer, error) {
			return nil, nil
		}},
		{Diagnostics: diag.NewRegistry(nil, diag.DefaultLimits())},
		{LSP: func(context.Context, lsp.Query) (lsp.Result, error) { return lsp.Result{}, nil }},
	} {
		engine := primaryEngine(t, wired)
		assembled := engine.ToolNames()
		slices.Sort(assembled)

		observed := slices.Clone(engine.ToolObservation().Admitted)
		slices.Sort(observed)

		assert.Equal(t, assembled, observed,
			"what the harness calls admitted must be what the builder would produce")
	}
}

func TestAToolWithNoOwnerAttachedSaysWhatIsMissing(t *testing.T) {
	state := primaryEngine(t, agent.EngineOpts{}).ToolObservation()

	for _, name := range []string{
		"mcp_list", "mcp_inspect", "mcp_call", "lsp", "memory", "watch",
		"task", "harness", "agent_spawn", "agent_list", "agent_wait", "agent_cancel",
	} {
		facts := factsFor(t, state, name)
		assert.False(t, facts.Admitted, "%s has no owner on a bare engine", name)
		assert.False(t, facts.Registered, name)
		assert.NotEmpty(t, facts.AdmittedFrom.Ref,
			"%s is absent for a reason the user can read", name)
	}
	assert.Contains(t, factsFor(t, state, "mcp_call").AdmittedFrom.Ref, "MCP pool")
	assert.Contains(t, factsFor(t, state, "harness").AdmittedFrom.Ref, "--developer-mode")
	assert.Contains(t, factsFor(t, state, "agent_spawn").AdmittedFrom.Ref, "sub-agent")
}

func TestAttachingAnOwnerTurnsTheToolOnAndNamesIt(t *testing.T) {
	registry := diag.NewRegistry(nil, diag.DefaultLimits())
	state := primaryEngine(t, agent.EngineOpts{Diagnostics: registry}).ToolObservation()

	facts := factsFor(t, state, "harness")
	assert.True(t, facts.Admitted)
	assert.True(t, facts.Registered)
	assert.Equal(t, diag.SourceCLIFlag, facts.AdmittedFrom.Kind,
		"the capability came from the command line, and only from there")
}

func TestASubAgentReportsWhyItHasNoPlanAndNoTitle(t *testing.T) {
	child := primaryEngine(t, agent.EngineOpts{
		SessionOpts: agent.SessionOpts{ParentID: "parent"},
		Tools:       agent.ChildTools(),
	})
	state := child.ToolObservation()

	plan := factsFor(t, state, "plan")
	assert.False(t, plan.Registered)
	assert.Contains(t, plan.AdmittedFrom.Ref, "sub-agent")

	title := factsFor(t, state, "session")
	assert.False(t, title.Registered)
	assert.Contains(t, title.AdmittedFrom.Ref, "primary session")

	assert.Empty(t, state.PlanGate, "a sub-agent has no plan gate to stand in a phase of")
	for _, facts := range state.Tools {
		assert.False(t, facts.Gated, "%s: nothing gates a session without a plan", facts.Name)
	}
}

func TestAReadonlySessionExplainsItsNarrowerToolSet(t *testing.T) {
	state := primaryEngine(t, agent.EngineOpts{Tools: tools.ReadonlyTools()}).ToolObservation()

	for _, name := range []string{"write", "edit"} {
		facts := factsFor(t, state, name)
		assert.False(t, facts.Admitted, name)
		assert.Contains(t, facts.AdmittedFrom.Ref, "read-only",
			"%s is missing because of the role, not because a decision denied it", name)
	}
	assert.True(t, factsFor(t, state, "read").Registered)
}

func TestPlanModeNarrowsWhatIsRegisteredWithoutChangingWhatTheSessionCarries(t *testing.T) {
	engine := primaryEngine(t, agent.EngineOpts{})
	ordinary := engine.ToolObservation()
	require.Contains(t, ordinary.Registered, "write")

	engine.SetMode(agent.ModePlan)
	planning := engine.ToolObservation()

	assert.Equal(t, "plan", planning.Mode)
	assert.Contains(t, planning.Admitted, "write",
		"the posture narrowed the registry; the session still carries the tool")
	assert.NotContains(t, planning.Registered, "write")
	assert.Contains(t, factsFor(t, planning, "write").RegisteredFrom.Ref, "plan mode")
}

func TestThePlanGatePhaseAndReachAreReadNotDecided(t *testing.T) {
	gate := &recordingGate{}
	engine := primaryEngine(t, agent.EngineOpts{Gate: gate})
	state := engine.ToolObservation()

	assert.Equal(t, "deny", state.PlanGate,
		"the primary posture is useplan, which denies until a step admits a tool")
	assert.NotContains(t, state.Callable, "bash",
		"no approved step is in progress, so nothing that changes anything is reachable")
	assert.Contains(t, state.Callable, "plan")
	assert.Contains(t, state.Callable, "context")
	assert.False(t, factsFor(t, state, "bash").Callable)
	assert.True(t, factsFor(t, state, "bash").Gated)
	assert.False(t, factsFor(t, state, "plan").Gated, "an exempt tool is not gated")
	assert.Zero(t, gate.calls, "observing the tool layer asks the boundary nothing")
}

func TestHintPhaseLeavesEveryRegisteredToolReachable(t *testing.T) {
	engine := primaryEngine(t, agent.EngineOpts{})
	engine.SetMode(agent.ModeBuild)
	state := engine.ToolObservation()

	assert.Equal(t, "hint", state.PlanGate)
	assert.Equal(t, state.Registered, state.Callable,
		"a hinting gate narrows nothing, so reachable is the whole registry")
}

func TestTheRevisionFollowsTheToolListAndNothingElse(t *testing.T) {
	engine := primaryEngine(t, agent.EngineOpts{})
	first := engine.ToolObservation().Revision
	require.NotEmpty(t, first)
	assert.Equal(t, first, engine.ToolObservation().Revision,
		"two reads of an unchanged engine describe the same list")

	engine.SetMode(agent.ModePlan)
	assert.NotEqual(t, first, engine.ToolObservation().Revision,
		"a posture that rebinds the registry is visible as a different revision")
}

func TestObservingDispatchesNoToolAndLeavesTheRegistryAsItWas(t *testing.T) {
	engine := primaryEngine(t, agent.EngineOpts{})
	before := engine.ToolNames()

	for range 3 {
		engine.ToolObservation()
	}

	assert.Equal(t, before, engine.ToolNames(),
		"an observation neither adds a tool to the session nor takes one away")
	assert.True(t, engine.HasTool("read"))
}

func TestNoEngineIsAnUnknownAnswerRatherThanAnInventedOne(t *testing.T) {
	var engine *agent.Engine

	state := engine.ToolObservation()

	assert.False(t, state.Known)
	assert.Empty(t, state.Tools)
	assert.Empty(t, state.Registered)
}

func TestEveryCatalogToolCarriesItsCheckClassification(t *testing.T) {
	state := primaryEngine(t, agent.EngineOpts{}).ToolObservation()

	for _, facts := range state.Tools {
		assert.Equal(t, permission.ToolCheckKind(facts.Name), facts.Check, facts.Name)
		assert.NotEqual(t, diag.CheckUnknown, facts.Check, facts.Name)
	}
}

func TestTheProjectionCarriesNoSchemaNoArgumentAndNoPath(t *testing.T) {
	secret := t.TempDir()
	engine := primaryEngine(t, agent.EngineOpts{SessionOpts: agent.SessionOpts{Cwd: secret}})

	encoded, err := json.Marshal(engine.ToolObservation())
	require.NoError(t, err)

	assert.NotContains(t, string(encoded), secret,
		"where the session runs belongs to the runtime category, not to a tool answer")
	assert.NotContains(t, string(encoded), "http://127.0.0.1:9")
	assert.NotContains(t, string(encoded), "properties",
		"a tool's parameter schema never enters this view")
}
