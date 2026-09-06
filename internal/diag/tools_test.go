package diag_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// ownedTools is an engine's answer with every owner attached: the whole
// catalog admitted, registered and reachable, and the plan gate hinting
// rather than denying.
func ownedTools() diag.ToolState {
	state := diag.ToolState{
		Known:    true,
		Mode:     "useplan",
		PlanGate: "hint",
		Revision: "cafe1",
	}
	for _, name := range diag.ToolCatalog() {
		state.Admitted = append(state.Admitted, name)
		state.Registered = append(state.Registered, name)
		state.Callable = append(state.Callable, name)
		state.Tools = append(state.Tools, diag.ToolFacts{
			Name:           name,
			Admitted:       true,
			AdmittedFrom:   diag.Source{Kind: diag.SourceSession, Ref: "attached to this session"},
			Registered:     true,
			RegisteredFrom: diag.Source{Kind: diag.SourceSession, Ref: "attached to this session"},
			Callable:       true,
			Check:          diag.CheckToolName,
		})
	}
	return state
}

func toolFields(t *testing.T, state diag.ToolState) []diag.Field {
	t.Helper()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewToolCollector(diag.ToolDeps{State: func() diag.ToolState { return state }}))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryTools)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	return snapshot.Categories[0].Fields
}

// withTool replaces one tool's facts, leaving the rest of the answer alone.
func withTool(state diag.ToolState, facts diag.ToolFacts) diag.ToolState {
	for i := range state.Tools {
		if state.Tools[i].Name == facts.Name {
			state.Tools[i] = facts
		}
	}
	return state
}

func TestEveryCatalogToolIsReported(t *testing.T) {
	fields := toolFields(t, ownedTools())

	for _, name := range diag.ToolCatalog() {
		field := fieldByKey(t, fields, diag.ToolKey(name))
		assert.Equal(t, diag.StatePresent, field.Effective.State,
			"a tool the owner knows about is answered for, not left unavailable")
	}
	assert.NotEmpty(t, diag.ToolCatalog())
}

func TestTheThreeLayersAreTheThreeQuestionsAboutATool(t *testing.T) {
	state := ownedTools()
	state = withTool(state, diag.ToolFacts{
		Name:           "watch",
		Admitted:       true,
		AdmittedFrom:   diag.Source{Kind: diag.SourceSession, Ref: "the watch manager attached to this session"},
		Registered:     false,
		RegisteredFrom: diag.Source{Kind: diag.SourceSession, Ref: "the session narrowed the set after assembly"},
		Check:          diag.CheckArguments,
	})
	field := fieldByKey(t, toolFields(t, state), diag.ToolKey("watch"))

	assert.Equal(t, diag.ToolRegistered, field.Configured.Value.Str,
		"the session's ordinary posture carries it")
	assert.Equal(t, diag.ToolUnavailable, field.Loaded.Value.Str,
		"the live registry does not")
	assert.Equal(t, diag.ToolUnavailable, field.Effective.Value.Str,
		"what is not registered cannot be reached, whatever the gate says")
	assert.Contains(t, field.Loaded.Source.Ref, "narrowed",
		"the owner's reason for the absence travels with the layer that is absent")
}

func TestARestrictedToolIsRegisteredButOutOfReach(t *testing.T) {
	state := ownedTools()
	state.PlanGate = "deny"
	state = withTool(state, diag.ToolFacts{
		Name:           "bash",
		Admitted:       true,
		AdmittedFrom:   diag.Source{Kind: diag.SourceSession, Ref: "the built-in tool set"},
		Registered:     true,
		RegisteredFrom: diag.Source{Kind: diag.SourceSession, Ref: "the built-in tool set"},
		Gated:          true,
		Callable:       false,
		Check:          diag.CheckArguments,
	})
	field := fieldByKey(t, toolFields(t, state), diag.ToolKey("bash"))

	assert.Equal(t, diag.ToolRegistered, field.Loaded.Value.Str)
	assert.Equal(t, diag.ToolRestricted, field.Effective.Value.Str)
	assert.Equal(t, diag.SourcePlan, field.Effective.Source.Kind,
		"the plan gate is what restricts it, and says so")
	assert.Equal(t, diag.ScopeStep, field.Scope,
		"a gated tool's answer lasts as long as the step does")
}

func TestAnArgumentDependentToolIsNeverReportedAsAllowed(t *testing.T) {
	state := ownedTools()
	state = withTool(state, diag.ToolFacts{
		Name:           "read",
		Admitted:       true,
		AdmittedFrom:   diag.Source{Kind: diag.SourceSession, Ref: "the built-in tool set"},
		Registered:     true,
		RegisteredFrom: diag.Source{Kind: diag.SourceSession, Ref: "the built-in tool set"},
		Callable:       true,
		Check:          diag.CheckArguments,
	})
	field := fieldByKey(t, toolFields(t, state), diag.ToolKey("read"))

	assert.Equal(t, diag.ToolRequiresArgumentCheck, field.Effective.Value.Str)
	assert.Contains(t, field.Effective.Source.Ref, "call",
		"the reason names the call, because no answer holds for every call")
}

func TestAToolNobodySuppliesSaysWhatIsMissing(t *testing.T) {
	state := ownedTools()
	missing := diag.Source{Kind: diag.SourceSession, Ref: "no MCP pool is attached to this session"}
	for _, name := range []string{"mcp_list", "mcp_inspect", "mcp_call"} {
		state = withTool(state, diag.ToolFacts{
			Name:           name,
			AdmittedFrom:   missing,
			RegisteredFrom: missing,
			Check:          diag.CheckArguments,
		})
	}
	fields := toolFields(t, state)

	for _, name := range []string{"mcp_list", "mcp_inspect", "mcp_call"} {
		field := fieldByKey(t, fields, diag.ToolKey(name))
		assert.Equal(t, diag.ToolUnavailable, field.Configured.Value.Str)
		assert.Equal(t, diag.ToolUnavailable, field.Effective.Value.Str)
		assert.Equal(t, missing.Ref, field.Effective.Source.Ref,
			"a user who expected the tool is told what would have to be attached")
	}
}

func TestTheAggregateFieldSeparatesAdmittedRegisteredAndReachable(t *testing.T) {
	state := ownedTools()
	state.Registered = []string{"read", "bash", "plan", "context"}
	state.Callable = []string{"plan", "context"}
	field := fieldByKey(t, toolFields(t, state), diag.KeyToolsRegistered)

	assert.Equal(t, diag.KindList, field.Configured.Value.Kind)
	assert.Equal(t, state.Registered, field.Loaded.Value.List)
	assert.Equal(t, state.Callable, field.Effective.Value.List)
	assert.Equal(t, diag.ApplyNextTurn, field.Apply,
		"a rebind lands between turns, so that is when the list can change")
}

func TestModeAndPlanGateAreReportedAsTheyStand(t *testing.T) {
	fields := toolFields(t, ownedTools())

	mode := fieldByKey(t, fields, diag.KeyToolsMode)
	assert.Equal(t, diag.StateNotApplicable, mode.Configured.State,
		"a posture is entered at runtime, never configured")
	assert.Equal(t, "useplan", mode.Effective.Value.Str)

	gate := fieldByKey(t, fields, diag.KeyToolsPlanGate)
	assert.Equal(t, "hint", gate.Effective.Value.Str)
}

func TestASessionWithoutAPlanGateReportsUnsetNotUnavailable(t *testing.T) {
	state := ownedTools()
	state.PlanGate = ""
	field := fieldByKey(t, toolFields(t, state), diag.KeyToolsPlanGate)

	assert.Equal(t, diag.StateUnset, field.Effective.State,
		"no plan gate is a fact about the session, not a failure to look")
}

func TestNoEngineYetLeavesEveryLayerUnavailable(t *testing.T) {
	fields := toolFields(t, diag.ToolState{})

	for _, field := range fields {
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, field.Key)
		assert.Equal(t, diag.StateUnavailable, field.Loaded.State, field.Key)
	}
}

func TestAnUnwiredCollectorAnswersUnavailableRatherThanPanicking(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewToolCollector(diag.ToolDeps{}))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryTools)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	assert.NotEmpty(t, snapshot.Categories[0].Fields)
}

func TestStatusIsAnsweredFromTheKeysAlone(t *testing.T) {
	var calls int
	collector := diag.NewToolCollector(diag.ToolDeps{State: func() diag.ToolState {
		calls++
		return ownedTools()
	}})

	status := collector.Status()

	assert.Zero(t, calls, "listing what a category can answer must not read the engine")
	assert.Equal(t, diag.CategoryTools, collector.Category())
	assert.Equal(t, diag.AvailabilityAvailable, status.Availability)
	assert.Len(t, status.Keys, len(diag.ToolCatalog())+3)
}

func TestTheCatalogIsHandedOutDetached(t *testing.T) {
	first := diag.ToolCatalog()
	require.NotEmpty(t, first)
	first[0] = "mutated"

	assert.NotEqual(t, "mutated", diag.ToolCatalog()[0],
		"a caller ranging over the catalog cannot rewrite it for everybody else")
}

func TestNoSchemaOrArgumentCanRideAlongWithAToolAnswer(t *testing.T) {
	state := ownedTools()
	encoded, err := json.Marshal(state)
	require.NoError(t, err)

	assert.NotContains(t, string(encoded), "properties")
	assert.NotContains(t, string(encoded), "description")
	for _, field := range []string{"Schema", "Description", "Arguments", "Params", "Result"} {
		assert.NotContains(t, string(encoded), field,
			"the projection lists the members it exports; a schema is not one of them")
	}
}
