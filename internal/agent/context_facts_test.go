package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/memory"
)

// contextTestWindow is the window the fixture model declares, so a narrowing
// applied on top of it has something to narrow.
const contextTestWindow = 200000

// contextEngine is an ordinary session on a model with a declared window,
// capped at spawn the way a sub-agent is.
func contextEngine(t *testing.T, ceiling int, store *memory.Store) *Engine {
	t.Helper()
	engine, err := NewEngine(EngineOpts{
		Model:          llm.ModelConfig{Name: "fake", ContextWindow: contextTestWindow},
		SessionOpts:    SessionOpts{Cwd: t.TempDir()},
		ContextCeiling: ceiling,
		Memory:         store,
	})
	require.NoError(t, err)
	return engine
}

func TestContextObservationSeparatesTheModelsWindowFromEveryNarrowingOfIt(t *testing.T) {
	engine := contextEngine(t, 120000, nil)

	state := engine.ContextObservation()
	require.True(t, state.Known)
	assert.Equal(t, contextTestWindow, state.Window.Model, "what the model entry declares")
	assert.Equal(t, 120000, state.Window.Ceiling, "what the parent capped it at")
	assert.Equal(t, 0, state.Window.Override, "nobody asked this session for less")
	assert.Equal(t, 120000, state.Window.Effective)

	engine.SetContextWindowOverride(60000)

	state = engine.ContextObservation()
	assert.Equal(t, 60000, state.Window.Override)
	assert.Equal(t, 60000, state.Window.Effective,
		"each narrowing only narrows, and the effective window is what is left")
	assert.Equal(t, contextTestWindow, state.Window.Model, "the model's own window is unchanged by either")
}

func TestTheCompactionPolicyAndThePressureAreObservedTogether(t *testing.T) {
	engine := contextEngine(t, 0, nil)

	state := engine.ContextObservation()
	assert.True(t, state.Compaction.Enabled)
	assert.Positive(t, state.Compaction.Headroom)
	assert.Positive(t, state.Compaction.KeepRecent)
	assert.Equal(t, contextTestWindow-state.Compaction.Headroom, state.Compaction.Threshold,
		"the threshold is the window minus the headroom kept below it")
	assert.False(t, state.Compaction.Recommended, "a fresh session is under no pressure")
	assert.False(t, state.Compaction.Pending)
	assert.Zero(t, state.Compaction.Strikes)
	assert.False(t, state.Compaction.Stopped)
	assert.Zero(t, state.Compaction.Count)
	assert.False(t, state.Compaction.Last.Known,
		"no compaction has run, and that is said rather than shown as zeroes")
}

func TestTheTokenCountCarriesItsOwnProvenance(t *testing.T) {
	state := contextEngine(t, 0, nil).ContextObservation()

	assert.Contains(t,
		[]string{diag.TokenSourceProvider, diag.TokenSourceCalibrated, diag.TokenSourceEstimate},
		state.Usage.TokenSource,
		"the owner names how the count was arrived at, and only in the agreed vocabulary")
	assert.Equal(t, diag.TokenSourceEstimate, state.Usage.TokenSource,
		"no provider has counted this context yet, so the number is a heuristic")
}

// The record exists precisely so observing cannot become a load: after the
// prompt is built, the sources on disk are removed and the answer must still
// describe the prompt the next request would carry.
func TestThePromptSourcesComeFromTheRenderRatherThanFromDisk(t *testing.T) {
	dir := t.TempDir()
	writeMemory(t, dir, "feedback", "scoped-gates", "Gates run on changed packages only.",
		"Never sweep the whole repository.")
	writeMemory(t, dir, "project", "read-only-harness", "The harness observes and never acts.",
		"Every collector is a read.")
	store, err := memory.Open(dir, nil)
	require.NoError(t, err)

	workspace := t.TempDir()
	t.Chdir(workspace)
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "AGENTS.md"),
		[]byte("# SENTINEL-INSTRUCTION\nnever echo this text\n"), 0o600))

	engine := contextEngine(t, 0, store)
	rendered := engine.systemPrompt()
	require.Contains(t, rendered, "SENTINEL-INSTRUCTION", "the render really loaded the file")

	before := engine.ContextObservation().Sources
	require.True(t, before.Loaded)
	assert.Equal(t, 1, before.Instructions)
	assert.Equal(t, []string{"workspace"}, before.InstructionScopes,
		"where the file was found, never where it lives")
	assert.True(t, before.MemoryStore)
	assert.Equal(t, 2, before.MemoryFacts)
	assert.Positive(t, before.MemoryRunes)
	assert.Equal(t, len(rendered), before.PromptBytes)

	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.Remove(filepath.Join(workspace, "AGENTS.md")))

	assert.Equal(t, before, engine.ContextObservation().Sources,
		"the answer is the load that happened, so removing its inputs changes nothing "+
			"until an owner renders again")
}

func TestObservingTheContextCompactsNothingAndSchedulesNothing(t *testing.T) {
	engine := contextEngine(t, 0, nil)
	engine.systemPrompt()

	before := engine.ContextObservation()
	entries := len(engine.Session().PathEntries())

	for range 5 {
		engine.ContextObservation()
	}

	after := engine.ContextObservation()
	assert.Len(t, engine.Session().PathEntries(), entries,
		"an observation appends nothing to the session")
	assert.Equal(t, before.Compaction, after.Compaction,
		"and it neither compacts, trims, nor schedules one")
	assert.Equal(t, before.Sources, after.Sources,
		"nor does it re-render the prompt it reports on")
	assert.Equal(t, before.Revision, after.Revision,
		"the generation is the same one, and the revision says so")
}

func TestTheRevisionFollowsAFreshRenderRatherThanTheConversation(t *testing.T) {
	engine := contextEngine(t, 0, nil)
	engine.systemPrompt()
	before := engine.ContextObservation().Revision
	require.NotEmpty(t, before)

	engine.systemPrompt()

	assert.NotEqual(t, before, engine.ContextObservation().Revision,
		"a new render is a new generation for anything read from the record")
}

func TestANilEngineAnswersUnknownRatherThanCrashingTheSnapshot(t *testing.T) {
	var engine *Engine
	assert.Equal(t, diag.ContextState{}, engine.ContextObservation())
}
