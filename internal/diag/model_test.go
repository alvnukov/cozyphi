package diag_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// modelFacts is a plausible model as the loader or the engine would report
// it. Tests vary the pieces they are about and leave the rest alone.
func modelFacts(name, effort string) diag.ModelFacts {
	return diag.ModelFacts{
		Known:           true,
		Name:            name,
		RequestName:     name + "-2026",
		Protocol:        "openai",
		Effort:          effort,
		RequestEffort:   effort,
		EffortLevels:    []string{"low", "high"},
		ContextWindow:   200000,
		MaxOutputTokens: 8192,
		Variants:        []string{"high", "low"},
		Options:         []string{"temperature"},
	}
}

func modelState(loaded diag.ModelFacts, acting diag.ModelActing) diag.ModelState {
	return diag.ModelState{
		Known:    true,
		Loaded:   loaded,
		Acting:   acting,
		Window:   loaded.ContextWindow,
		Revision: "3",
	}
}

func actingOn(facts diag.ModelFacts) diag.ModelActing {
	return diag.ModelActing{Name: facts.Name, Effort: facts.Effort}
}

func modelFields(t *testing.T, deps diag.ModelDeps) []diag.Field {
	t.Helper()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), diag.NewModelCollector(deps))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryModel)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	return snapshot.Categories[0].Fields
}

// deps wires a collector from a configured model, its selection source and an
// engine state, which is all three owners of a model observation.
func deps(configured diag.ModelFacts, source diag.Source, state diag.ModelState) diag.ModelDeps {
	return diag.ModelDeps{
		Configured:       func() diag.ModelFacts { return configured },
		ConfiguredSource: func() diag.Source { return source },
		State:            func() diag.ModelState { return state },
	}
}

func TestModelSelectionSourceReportsOnlyWhatTheLoaderRecords(t *testing.T) {
	assert.Equal(t, diag.Source{Kind: diag.SourceEnv, Ref: "COZYPHI_MODEL"},
		diag.ModelSelectionSource(true, true), "an environment pin outranks a declared default")
	assert.Equal(t, diag.Source{Kind: diag.SourceConfigFile, Ref: "models[].default"},
		diag.ModelSelectionSource(false, true))
	assert.Equal(t, diag.SourceDefault, diag.ModelSelectionSource(false, false).Kind,
		"falling back to the first entry is a default, not a config file decision")
}

func TestConfiguredModelIsReportedWithItsOwnOrigin(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source diag.Source
	}{
		{"config file default", diag.ModelSelectionSource(false, true)},
		{"environment override", diag.ModelSelectionSource(true, false)},
		{"first entry", diag.ModelSelectionSource(false, false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configured := modelFacts("chosen", "high")
			fields := modelFields(t, deps(configured, tc.source, modelState(configured, actingOn(configured))))

			name := fieldByKey(t, fields, diag.KeyModelName)
			assert.Equal(t, diag.StatePresent, name.Configured.State)
			assert.Equal(t, "chosen", name.Configured.Value.Str)
			assert.Equal(t, tc.source, name.Configured.Source)
			assert.Equal(t, tc.source, name.Loaded.Source,
				"an engine still on the configured model is explained by the configured source")
			assert.Equal(t, tc.source, name.Effective.Source)
			assert.Equal(t, "chosen", name.Effective.Value.Str)
		})
	}
}

func TestASessionChoiceIsAttributedToTheSessionAndNotToTheConfigFile(t *testing.T) {
	configured := modelFacts("from-disk", "low")
	loaded := modelFacts("picked-later", "high")
	fields := modelFields(t, deps(configured, diag.ModelSelectionSource(false, true),
		modelState(loaded, actingOn(loaded))))

	name := fieldByKey(t, fields, diag.KeyModelName)
	assert.Equal(t, "from-disk", name.Configured.Value.Str, "the loader's answer is unchanged")
	assert.Equal(t, "picked-later", name.Loaded.Value.Str)
	assert.Equal(t, diag.SourceSession, name.Loaded.Source.Kind)
	assert.Equal(t, diag.SourceSession, name.Effective.Source.Kind)
	assert.Equal(t, "picked-later", name.Effective.Value.Str)
}

func TestChangingOnlyTheEffortLeavesTheModelAttributedToItsEntry(t *testing.T) {
	configured := modelFacts("same-model", "low")
	loaded := modelFacts("same-model", "high")
	fields := modelFields(t, deps(configured, diag.ModelSelectionSource(false, true),
		modelState(loaded, actingOn(loaded))))

	effort := fieldByKey(t, fields, diag.KeyModelEffort)
	assert.Equal(t, "low", effort.Configured.Value.Str)
	assert.Equal(t, "high", effort.Effective.Value.Str)
	assert.Equal(t, diag.SourceSession, effort.Loaded.Source.Kind,
		"the depth was chosen after the load")

	window := fieldByKey(t, fields, diag.KeyModelContextWindow)
	assert.Equal(t, diag.SourceConfigFile, window.Loaded.Source.Kind,
		"the entry that carries the window is still the configured one")
}

func TestAPlanPinIsNamedAsThePinItIsAndNarrowsTheScope(t *testing.T) {
	configured := modelFacts("session-model", "low")
	pinned := modelFacts("step-model", "high")
	state := modelState(pinned, actingOn(pinned))
	state.Pinned = true
	fields := modelFields(t, deps(configured, diag.ModelSelectionSource(false, true), state))

	name := fieldByKey(t, fields, diag.KeyModelName)
	assert.Equal(t, diag.SourcePlan, name.Loaded.Source.Kind)
	assert.Equal(t, diag.SourcePlan, name.Effective.Source.Kind)
	assert.Equal(t, diag.ScopeStep, name.Scope, "a pinned model lasts as long as the step")

	pin := fieldByKey(t, fields, diag.KeyModelPlanPinned)
	assert.Equal(t, diag.StatePresent, pin.Effective.State)
	assert.True(t, pin.Effective.Value.Bool)
	assert.Equal(t, diag.StateNotApplicable, pin.Configured.State,
		"no config key can pin a step's model")
}

func TestARoundInFlightKeepsItsOwnModelAndSaysWhatIsNotObservable(t *testing.T) {
	configured := modelFacts("before", "low")
	loaded := modelFacts("after", "high")
	fields := modelFields(t, deps(configured, diag.ModelSelectionSource(false, true),
		modelState(loaded, diag.ModelActing{Name: "before", Effort: "low"})))

	name := fieldByKey(t, fields, diag.KeyModelName)
	assert.Equal(t, "after", name.Loaded.Value.Str, "the engine holds the next round's model")
	assert.Equal(t, "before", name.Effective.Value.Str, "the round in flight answers on the one it started")
	assert.Equal(t, diag.SourceComputed, name.Effective.Source.Kind)

	protocol := fieldByKey(t, fields, diag.KeyModelProtocol)
	assert.Equal(t, diag.StateUnavailable, protocol.Effective.State,
		"the acting model's capabilities are not observable while the two disagree")
	assert.Equal(t, diag.StatePresent, protocol.Loaded.State)
}

func TestWithoutAnEngineTheSavedDefaultIsNotPresentedAsTheRunningModel(t *testing.T) {
	configured := modelFacts("configured-default", "high")
	fields := modelFields(t, deps(configured, diag.ModelSelectionSource(false, true), diag.ModelState{}))

	for _, key := range []string{diag.KeyModelName, diag.KeyModelEffort, diag.KeyModelContextWindow} {
		field := fieldByKey(t, fields, key)
		assert.Equal(t, diag.StateUnavailable, field.Loaded.State, key)
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, key)
		assert.Empty(t, field.Effective.Value.Str, key)
	}
	assert.Equal(t, "configured-default",
		fieldByKey(t, fields, diag.KeyModelName).Configured.Value.Str,
		"what was configured is still reported; it is simply not claimed to be running")
}

func TestUnsetModelValuesStayLegibleAsUnset(t *testing.T) {
	bare := diag.ModelFacts{Known: true, Name: "bare", RequestName: "bare"}
	fields := modelFields(t, deps(bare, diag.ModelSelectionSource(false, false),
		modelState(bare, actingOn(bare))))

	window := fieldByKey(t, fields, diag.KeyModelContextWindow)
	assert.Equal(t, diag.StateUnset, window.Configured.State)
	assert.Equal(t, int64(0), window.Configured.Value.Int, "the zero a caller would get is carried anyway")

	levels := fieldByKey(t, fields, diag.KeyModelEffortLevels)
	assert.Equal(t, diag.StateUnset, levels.Loaded.State, "a model offering no runtime depth says so")
	assert.Empty(t, levels.Loaded.Value.List)

	thinking := fieldByKey(t, fields, diag.KeyModelThinking)
	assert.Equal(t, diag.StatePresent, thinking.Effective.State, "false is a real answer here")
	assert.False(t, thinking.Effective.Value.Bool)
}

func TestTheEffectiveWindowIsTheOneTheEngineBudgetsAgainst(t *testing.T) {
	configured := modelFacts("wide", "high")
	state := modelState(configured, actingOn(configured))
	state.Window = 32000
	fields := modelFields(t, deps(configured, diag.ModelSelectionSource(false, true), state))

	window := fieldByKey(t, fields, diag.KeyModelContextWindow)
	assert.Equal(t, int64(200000), window.Loaded.Value.Int, "the model's own window")
	assert.Equal(t, int64(32000), window.Effective.Value.Int, "narrowed by the session")
	assert.Equal(t, diag.SourceComputed, window.Effective.Source.Kind)
	assert.Equal(t, diag.ApplyImmediate, window.Apply, "a narrower window lands on the running engine")
}

func TestTheOverrideOrderIsPublishedWithTheFields(t *testing.T) {
	configured := modelFacts("m", "high")
	fields := modelFields(t, deps(configured, diag.ModelSelectionSource(false, true),
		modelState(configured, actingOn(configured))))

	order := fieldByKey(t, fields, diag.KeyModelSourceOrder)
	assert.Equal(t, []string{"default", "config_file", "env", "session", "plan"}, order.Effective.Value.List)
	assert.Equal(t, diag.StateNotApplicable, order.Configured.State)
}

func TestModelObservationsAreDetachedFromTheOwner(t *testing.T) {
	configured := modelFacts("m", "high")
	state := modelState(configured, actingOn(configured))
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewModelCollector(deps(configured, diag.ModelSelectionSource(false, true), state)))

	first, err := registry.Snapshot(t.Context(), diag.CategoryModel)
	require.NoError(t, err)
	levels := fieldByKey(t, first.Categories[0].Fields, diag.KeyModelEffortLevels)
	levels.Effective.Value.List[0] = "tampered"
	fieldByKey(t, first.Categories[0].Fields, diag.KeyModelVariants).Loaded.Value.List[0] = "tampered"

	second, err := registry.Snapshot(t.Context(), diag.CategoryModel)
	require.NoError(t, err)
	assert.Equal(t, []string{"low", "high"},
		fieldByKey(t, second.Categories[0].Fields, diag.KeyModelEffortLevels).Effective.Value.List,
		"an answer already given is a copy; writing to it reaches neither the owner nor the next answer")
	assert.Equal(t, []string{"high", "low"}, configured.Variants,
		"the owner's own slices are not handed out")
	assert.Equal(t, []string{"high", "low"},
		fieldByKey(t, second.Categories[0].Fields, diag.KeyModelVariants).Loaded.Value.List)
}

func TestAnUnwiredModelCollectorAnswersUnavailableRatherThanPanicking(t *testing.T) {
	fields := modelFields(t, diag.ModelDeps{})

	require.Len(t, fields, 20)
	name := fieldByKey(t, fields, diag.KeyModelName)
	assert.Equal(t, diag.StateUnavailable, name.Configured.State)
	assert.Equal(t, diag.StateUnavailable, name.Loaded.State)
	assert.Equal(t, diag.StateUnavailable, name.Effective.State)
}

func TestTheModelCategoryDeclaresWhatItWillNotShow(t *testing.T) {
	configured := modelFacts("m", "high")
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewModelCollector(deps(configured, diag.ModelSelectionSource(false, true),
			modelState(configured, actingOn(configured)))))

	var entry diag.CatalogEntry
	for _, candidate := range registry.Catalog().Categories {
		if candidate.Category == diag.CategoryModel {
			entry = candidate
		}
	}
	require.Equal(t, diag.CategoryModel, entry.Category)
	assert.Contains(t, entry.Reason, "api keys")
	assert.Contains(t, entry.Reason, "presence and kind only",
		"what the category will say about a credential is published with the exclusions")
	assert.Contains(t, entry.Reason, "load-error text")
	assert.NotContains(t, entry.Keys, "api_key")
	assert.NotContains(t, entry.Keys, "base_url")
	assert.NotContains(t, entry.Keys, "credentials")

	_, err := registry.Explain(t.Context(), diag.CategoryModel, "api_key")
	require.Error(t, err, "a field that does not exist is an error, not an empty answer")

	explained, err := registry.Explain(t.Context(), diag.CategoryModel, diag.KeyModelName)
	require.NoError(t, err)
	assert.Equal(t, "m", explained.Field.Effective.Value.Str)
	assert.Equal(t, "3", explained.Field.Revision, "an observation names the model generation it saw")
}

// withOwners adds the other two owners of the category — the provider manager
// and the read-only import — to a wiring that already has the model's.
func withOwners(base diag.ModelDeps, providers diag.ProviderFacts, imported diag.ImportFacts) diag.ModelDeps {
	base.Providers = func() diag.ProviderFacts { return providers }
	base.Import = func() diag.ImportFacts { return imported }
	return base
}

// modelOwners is the ordinary wiring: a settled model plus the two owners a
// test wants to vary.
func modelOwners(t *testing.T, providers diag.ProviderFacts, imported diag.ImportFacts) []diag.Field {
	t.Helper()
	configured := modelFacts("m", "high")
	return modelFields(t, withOwners(
		deps(configured, diag.ModelSelectionSource(false, true), modelState(configured, actingOn(configured))),
		providers, imported))
}

func TestTheCatalogSaysWhichOfItsTwoOriginsItCameFrom(t *testing.T) {
	saved := modelOwners(t,
		diag.ProviderFacts{Known: true, Catalog: 7, Cached: true, Revision: "4"}, diag.ImportFacts{})
	catalog := fieldByKey(t, saved, diag.KeyModelCatalog)

	assert.Equal(t, diag.StateNotApplicable, catalog.Configured.State,
		"nobody configures a catalog; it is fetched and cached")
	assert.Equal(t, int64(7), catalog.Effective.Value.Int)
	assert.Equal(t, diag.SourceConfigFile, catalog.Effective.Source.Kind)
	assert.Contains(t, catalog.Effective.Source.Ref, "saved")
	assert.Equal(t, "4", catalog.Revision, "the catalog carries the credential store's generation")
	assert.Equal(t, diag.ScopeProcess, catalog.Scope, "one catalog serves every session")

	builtin := modelOwners(t, diag.ProviderFacts{Known: true, Catalog: 2}, diag.ImportFacts{})
	fresh := fieldByKey(t, builtin, diag.KeyModelCatalog)
	assert.Equal(t, diag.SourceBuild, fresh.Effective.Source.Kind,
		"a catalog nobody has refreshed yet is the built-in table, and says so")
	assert.Equal(t, int64(2), fresh.Effective.Value.Int)
}

func TestAConnectedProviderIsNamedAndNothingElseAboutItIs(t *testing.T) {
	connected := []string{"openai", "zai-coding-plan"}
	fields := modelOwners(t,
		diag.ProviderFacts{Known: true, Catalog: 2, Connected: connected, Revision: "1"}, diag.ImportFacts{})
	field := fieldByKey(t, fields, diag.KeyModelConnected)

	assert.Equal(t, diag.StatePresent, field.Effective.State)
	assert.Equal(t, connected, field.Effective.Value.List)
	assert.Contains(t, field.Effective.Source.Ref, "provider names only")

	field.Effective.Value.List[0] = "tampered"
	again := fieldByKey(t,
		modelOwners(t, diag.ProviderFacts{Known: true, Connected: connected, Revision: "1"}, diag.ImportFacts{}),
		diag.KeyModelConnected)
	assert.Equal(t, []string{"openai", "zai-coding-plan"}, again.Effective.Value.List,
		"the answer is detached: writing to it reaches neither the store nor the next observation")
	assert.Equal(t, []string{"openai", "zai-coding-plan"}, connected)
}

func TestAnEmptyCredentialStoreIsAnAnswerNotAGap(t *testing.T) {
	fields := modelOwners(t, diag.ProviderFacts{Known: true, Catalog: 2, Revision: "0"}, diag.ImportFacts{})
	field := fieldByKey(t, fields, diag.KeyModelConnected)

	assert.Equal(t, diag.StateUnset, field.Effective.State,
		"the store was read and holds nothing; that is not the same as not knowing")
	assert.Empty(t, field.Effective.Value.List)
}

func TestTheImportTellsItsFourOutcomesApart(t *testing.T) {
	for _, tc := range []struct {
		state    diag.ImportState
		setting  string
		modelsAt diag.State
	}{
		{diag.ImportDisabled, "disabled", diag.StateNotApplicable},
		{diag.ImportNotLoaded, "enabled", diag.StateUnavailable},
		{diag.ImportFailed, "enabled", diag.StateUnavailable},
		{diag.ImportLoaded, "enabled", diag.StatePresent},
	} {
		t.Run(string(tc.state), func(t *testing.T) {
			fields := modelOwners(t, diag.ProviderFacts{Known: true},
				diag.ImportFacts{Known: true, State: tc.state, Models: 5})

			state := fieldByKey(t, fields, diag.KeyModelImport)
			assert.Equal(t, tc.setting, state.Configured.Value.Str, "what the setting asked for")
			assert.Equal(t, "opencode.enabled", state.Configured.Source.Ref)
			assert.Equal(t, string(tc.state), state.Effective.Value.Str, "what the import did")

			count := fieldByKey(t, fields, diag.KeyModelImportModels)
			assert.Equal(t, tc.modelsAt, count.Effective.State)
			if tc.modelsAt == diag.StatePresent {
				assert.Equal(t, int64(5), count.Effective.Value.Int)
			}
		})
	}
}

func TestAFailedImportReportsTheFailureAndNoneOfItsText(t *testing.T) {
	fields := modelOwners(t, diag.ProviderFacts{Known: true},
		diag.ImportFacts{Known: true, State: diag.ImportFailed})

	state := fieldByKey(t, fields, diag.KeyModelImport)
	assert.Equal(t, "failed", state.Effective.Value.Str)
	assert.Equal(t, "enabled", state.Configured.Value.Str,
		"a failure is not a switch: the setting still says the import was wanted")
	// The DTO has no member for a message, so there is nothing here to print.
	assert.NotContains(t, state.Effective.Source.Ref, "error")
}

func TestACredentialIsReportedAsPresenceAndKindOnly(t *testing.T) {
	configured := modelFacts("openai/gpt-5.5", "high")
	configured.Provider = "openai"
	configured.Credential = true
	configured.CredentialKind = "authenticator"
	fields := modelFields(t, withOwners(
		deps(configured, diag.ModelSelectionSource(false, true), modelState(configured, actingOn(configured))),
		diag.ProviderFacts{Known: true, Catalog: 2, Connected: []string{"openai"}}, diag.ImportFacts{}))

	provider := fieldByKey(t, fields, diag.KeyModelProvider)
	assert.Equal(t, "openai", provider.Effective.Value.Str)

	presence := fieldByKey(t, fields, diag.KeyModelCredential)
	assert.Equal(t, diag.StatePresent, presence.Effective.State)
	assert.True(t, presence.Effective.Value.Bool)
	assert.Equal(t, diag.KindBool, presence.Effective.Value.Kind,
		"presence is a yes or a no; there is no shape here for a value to arrive in")

	kind := fieldByKey(t, fields, diag.KeyModelCredentialKind)
	assert.Equal(t, "authenticator", kind.Effective.Value.Str)
	assert.Contains(t, kind.Effective.Source.Ref, "never what it authenticates with")
}

func TestAModelWithNoCredentialSaysSoRatherThanSayingNothing(t *testing.T) {
	configured := modelFacts("local", "")
	fields := modelFields(t, withOwners(
		deps(configured, diag.ModelSelectionSource(false, true), modelState(configured, actingOn(configured))),
		diag.ProviderFacts{Known: true}, diag.ImportFacts{Known: true, State: diag.ImportDisabled}))

	presence := fieldByKey(t, fields, diag.KeyModelCredential)
	assert.Equal(t, diag.StatePresent, presence.Effective.State)
	assert.False(t, presence.Effective.Value.Bool, "a model running unauthenticated is an observation")

	kind := fieldByKey(t, fields, diag.KeyModelCredentialKind)
	assert.Equal(t, diag.StateUnset, kind.Effective.State)
	provider := fieldByKey(t, fields, diag.KeyModelProvider)
	assert.Equal(t, diag.StateUnset, provider.Effective.State,
		"a model declared in models[] belongs to no provider")
}

func TestUnwiredProviderAndImportOwnersAnswerUnavailable(t *testing.T) {
	configured := modelFacts("m", "high")
	fields := modelFields(t,
		deps(configured, diag.ModelSelectionSource(false, true), modelState(configured, actingOn(configured))))

	for _, key := range []string{diag.KeyModelCatalog, diag.KeyModelConnected, diag.KeyModelImport} {
		field := fieldByKey(t, fields, key)
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, key)
	}
	assert.Equal(t, diag.StateUnavailable,
		fieldByKey(t, fields, diag.KeyModelImportModels).Effective.State)
}
