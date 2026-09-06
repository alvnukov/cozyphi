package diag_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// loadedContext is an owner's answer with every layer populated: a window the
// model declares and a session narrowed, usage the provider counted, a
// compaction behind it, and a prompt assembled from instructions, skills and
// memory.
func loadedContext() diag.ContextState {
	return diag.ContextState{
		Known: true,
		Window: diag.ContextWindow{
			Model:     200000,
			Ceiling:   0,
			Override:  120000,
			Effective: 120000,
		},
		Usage: diag.ContextUsage{
			Tokens:             48000,
			TokenSource:        diag.TokenSourceProvider,
			Bytes:              192000,
			Messages:           64,
			MicroElidedResults: 3,
			MicroElidedBytes:   40960,
		},
		Compaction: diag.ContextCompaction{
			Enabled:     true,
			Threshold:   103616,
			Headroom:    16384,
			KeepRecent:  20000,
			Recommended: false,
			Count:       1,
			Last: diag.ContextCompactionLast{
				Known:              true,
				TokensBefore:       110000,
				TokensAfter:        21000,
				MessagesSummarized: 80,
				MessagesKept:       12,
			},
		},
		Sources: diag.ContextSources{
			Loaded:            true,
			PromptBytes:       24000,
			Instructions:      2,
			InstructionScopes: []string{"agent_dir", "workspace"},
			SkillDir:          true,
			Skills:            7,
			MemoryStore:       true,
			MemoryFacts:       19,
			MemoryStanding:    4,
			MemoryRunes:       3200,
		},
		Revision: "c0ffee",
	}
}

func contextFields(t *testing.T, state diag.ContextState) []diag.Field {
	t.Helper()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewContextCollector(diag.ContextDeps{State: func() diag.ContextState { return state }}))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryContext)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	require.False(t, snapshot.Truncated, "the category fits the response budget on its own")
	return snapshot.Categories[0].Fields
}

func TestEveryDeclaredContextKeyIsAnswered(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewContextCollector(diag.ContextDeps{State: loadedContext}))
	fields := contextFields(t, loadedContext())

	var entry diag.CatalogEntry
	for _, candidate := range registry.Catalog().Categories {
		if candidate.Category == diag.CategoryContext {
			entry = candidate
		}
	}
	require.NotEmpty(t, entry.Keys, "the catalog lists what can be asked for")

	for _, key := range entry.Keys {
		field := fieldByKey(t, fields, key)
		assert.NotEqual(t, diag.StateUnavailable, field.Effective.State,
			"a key the catalog advertises is answered by an owner that knows: %s", key)
	}
	assert.Len(t, fields, len(entry.Keys), "the answer carries exactly the declared keys")
}

func TestTheWindowSeparatesWhatTheModelOffersFromWhatThisSessionBudgets(t *testing.T) {
	fields := contextFields(t, loadedContext())

	window := fieldByKey(t, fields, diag.KeyContextWindow)
	assert.Equal(t, int64(200000), window.Configured.Value.Int,
		"the configured layer is the model entry's own window")
	assert.Equal(t, int64(120000), window.Effective.Value.Int,
		"the effective layer is what the narrowings left of it")

	override := fieldByKey(t, fields, diag.KeyContextOverride)
	assert.Equal(t, diag.StatePresent, override.Effective.State)
	assert.Equal(t, int64(120000), override.Effective.Value.Int)

	ceiling := fieldByKey(t, fields, diag.KeyContextCeiling)
	assert.Equal(t, diag.StateUnset, ceiling.Effective.State,
		"no parent capped this engine, and that is an answer rather than missing data")
	assert.Contains(t, ceiling.Effective.Source.Ref, "no parent")
}

// The whole point of the token-source field: a number a heuristic produced
// must never reach a reader looking like one the provider counted.
func TestTheTokenCountSaysWhetherItWasMeasuredOrEstimated(t *testing.T) {
	measured := contextFields(t, loadedContext())
	tokens := fieldByKey(t, measured, diag.KeyContextTokens)
	assert.Equal(t, diag.TokenSourceProvider,
		fieldByKey(t, measured, diag.KeyContextTokenSource).Effective.Value.Str)
	assert.Contains(t, tokens.Effective.Source.Ref, "measurement")

	state := loadedContext()
	state.Usage.TokenSource = diag.TokenSourceEstimate
	estimated := contextFields(t, state)
	assert.Contains(t, fieldByKey(t, estimated, diag.KeyContextTokens).Effective.Source.Ref,
		"not a measurement",
		"a heuristic is labeled a heuristic where the number is read")

	state.Usage.TokenSource = diag.TokenSourceCalibrated
	calibrated := contextFields(t, state)
	assert.Contains(t, fieldByKey(t, calibrated, diag.KeyContextTokens).Effective.Source.Ref,
		"part measured, part estimated")

	state.Usage.TokenSource = ""
	silent := contextFields(t, state)
	source := fieldByKey(t, silent, diag.KeyContextTokenSource)
	assert.Equal(t, diag.StateUnset, source.Effective.State)
	assert.Equal(t, diag.SourceUnknown, source.Effective.Source.Kind,
		"an owner that did not say gets an explicit unknown, never the benefit of the doubt")
	assert.Equal(t, diag.SourceUnknown,
		fieldByKey(t, silent, diag.KeyContextTokens).Effective.Source.Kind)
}

func TestTheCompactionThresholdKeepsTheConfiguredNumberApartFromTheDerivedOne(t *testing.T) {
	derived := fieldByKey(t, contextFields(t, loadedContext()), diag.KeyContextCompactThreshold)
	assert.Equal(t, diag.StateUnset, derived.Configured.State,
		"nobody set a threshold, so the configured layer says so")
	assert.Contains(t, derived.Configured.Source.Ref, "window-derived")
	assert.Equal(t, int64(103616), derived.Effective.Value.Int,
		"and the effective layer is what the window works out to")

	state := loadedContext()
	state.Compaction.ReminderTokens = 90000
	chosen := fieldByKey(t, contextFields(t, state), diag.KeyContextCompactThreshold)
	assert.Equal(t, int64(90000), chosen.Configured.Value.Int)
	assert.Equal(t, diag.SourceConfigFile, chosen.Configured.Source.Kind)
}

func TestCompactionOffLeavesNoThresholdActing(t *testing.T) {
	state := loadedContext()
	state.Compaction.Enabled = false
	state.Compaction.Threshold = 0
	fields := contextFields(t, state)

	assert.False(t, fieldByKey(t, fields, diag.KeyContextCompactionEnabled).Effective.Value.Bool)
	threshold := fieldByKey(t, fields, diag.KeyContextCompactThreshold)
	assert.Equal(t, diag.StateUnset, threshold.Effective.State)
	assert.Contains(t, threshold.Effective.Source.Ref, "compaction is off")
}

func TestACompactionThatNeverRanIsNotReportedAsOneThatFreedNothing(t *testing.T) {
	state := loadedContext()
	state.Compaction.Count = 0
	state.Compaction.Last = diag.ContextCompactionLast{}
	fields := contextFields(t, state)

	kind := fieldByKey(t, fields, diag.KeyContextLastCompactionKind)
	assert.Equal(t, diag.StateUnset, kind.Effective.State)
	assert.Contains(t, kind.Effective.Source.Ref, "no compaction yet")
	before := fieldByKey(t, fields, diag.KeyContextLastTokensBefore)
	assert.Equal(t, diag.StateUnset, before.Effective.State)

	assert.Equal(t, int64(0), fieldByKey(t, fields, diag.KeyContextCompactCount).Effective.Value.Int,
		"the count is a real zero: this path carries no compaction")
}

func TestATrimIsToldApartFromAGeneratedSummary(t *testing.T) {
	assert.Equal(t, diag.CompactionSummary,
		fieldByKey(t, contextFields(t, loadedContext()), diag.KeyContextLastCompactionKind).Effective.Value.Str)

	state := loadedContext()
	state.Compaction.Last.FromTrim = true
	assert.Equal(t, diag.CompactionTrim,
		fieldByKey(t, contextFields(t, state), diag.KeyContextLastCompactionKind).Effective.Value.Str,
		"nothing was summarized, and the counters next to it read differently")
}

func TestThePromptSourcesAreCountsAndScopesRatherThanPaths(t *testing.T) {
	fields := contextFields(t, loadedContext())

	instructions := fieldByKey(t, fields, diag.KeyContextInstructions)
	assert.Equal(t, int64(2), instructions.Loaded.Value.Int, "what discovery took in")
	assert.Equal(t, int64(2), instructions.Effective.Value.Int, "what the prompt carries")

	scopes := fieldByKey(t, fields, diag.KeyContextInstructionScopes)
	assert.Equal(t, []string{"agent_dir", "workspace"}, scopes.Effective.Value.List)

	assert.Equal(t, int64(7), fieldByKey(t, fields, diag.KeyContextSkills).Loaded.Value.Int)
	assert.Equal(t, int64(19), fieldByKey(t, fields, diag.KeyContextMemory).Effective.Value.Int)
	assert.Equal(t, int64(4), fieldByKey(t, fields, diag.KeyContextMemoryStanding).Effective.Value.Int)
	assert.Equal(t, int64(24000), fieldByKey(t, fields, diag.KeyContextPromptBytes).Effective.Value.Int)
}

func TestAnAbsentSkillDirectoryAndAnAbsentStoreAreNotReportedAsEmptyOnes(t *testing.T) {
	state := loadedContext()
	state.Sources.SkillDir = false
	state.Sources.Skills = 0
	state.Sources.MemoryStore = false
	fields := contextFields(t, state)

	skills := fieldByKey(t, fields, diag.KeyContextSkills)
	assert.Equal(t, diag.StateUnset, skills.Effective.State)
	assert.Contains(t, skills.Effective.Source.Ref, "no skill directory is configured")

	memories := fieldByKey(t, fields, diag.KeyContextMemory)
	assert.Equal(t, diag.StateUnset, memories.Effective.State)
	assert.Contains(t, memories.Effective.Source.Ref, "no memory store")
}

func TestBeforeTheFirstRenderThePromptSourcesAreUnavailableRatherThanZero(t *testing.T) {
	state := loadedContext()
	state.Sources = diag.ContextSources{}
	fields := contextFields(t, state)

	for _, key := range []string{
		diag.KeyContextPromptBytes,
		diag.KeyContextInstructions,
		diag.KeyContextInstructionScopes,
		diag.KeyContextSkills,
		diag.KeyContextMemory,
	} {
		assert.Equal(t, diag.StateUnavailable, fieldByKey(t, fields, key).Effective.State,
			"no prompt has been assembled, so there is nothing to report about one: %s", key)
	}
	assert.Equal(t, diag.StatePresent, fieldByKey(t, fields, diag.KeyContextTokens).Effective.State,
		"the window is still observable while the prompt record is empty")
}

func TestNoOwnerYetLeavesTheWholeContextCategoryUnavailable(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewContextCollector(diag.ContextDeps{}))

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryContext)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.NotEmpty(t, snapshot.Categories[0].Fields)
	for _, field := range snapshot.Categories[0].Fields {
		assert.Equal(t, diag.StateUnavailable, field.Effective.State,
			"a nil accessor is a wiring gap, not an empty context: %s", field.Key)
	}
}

func TestTheContextAnswerIsDetachedFromTheOwnersSlices(t *testing.T) {
	state := loadedContext()
	fields := contextFields(t, state)
	scopes := fieldByKey(t, fields, diag.KeyContextInstructionScopes).Effective.Value.List

	state.Sources.InstructionScopes[0] = "mutated"

	assert.Equal(t, []string{"agent_dir", "workspace"}, scopes,
		"a snapshot taken is a snapshot kept")
}

// The category's whole purpose is to describe the conversation without
// carrying any of it, so the rendered answer is searched for the text the
// fixture's owner holds.
func TestNoConversationTextReachesTheContextAnswer(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewContextCollector(diag.ContextDeps{State: loadedContext}))

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryContext)
	require.NoError(t, err)
	rendered, err := json.Marshal(snapshot)
	require.NoError(t, err)

	body := string(rendered)
	for _, sentinel := range []string{
		"SENTINEL-SUMMARY", "SENTINEL-MESSAGE", "SENTINEL-PROMPT",
		"SENTINEL-MEMORY", "SENTINEL-SKILL", "/Users/", "AGENTS.md",
	} {
		assert.NotContains(t, body, sentinel)
	}
	assert.NotContains(t, body, "preview",
		"nothing in this category previews anything")

	explained, err := registry.Explain(t.Context(), diag.CategoryContext, diag.KeyContextMemory)
	require.NoError(t, err)
	assert.Equal(t, diag.CategoryContext, explained.Category)
	assert.Equal(t, "c0ffee", explained.Field.Revision)
}

func TestAnUnknownContextKeyIsRefusedRatherThanAnswered(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewContextCollector(diag.ContextDeps{State: loadedContext}))

	_, err := registry.Explain(t.Context(), diag.CategoryContext, "usage.transcript")
	require.Error(t, err)
	assert.Contains(t, err.Error(), diag.KeyContextTokens,
		"the refusal names what can be asked for instead of observing something near it")
}
