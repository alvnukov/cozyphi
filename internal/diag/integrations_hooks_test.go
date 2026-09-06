package diag_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// liveHooks is the arrangement worth telling apart: a name both directories
// define, one each directory defines alone, a manifest the manager was built
// without, and an entry no directory accounts for. Every gap between the
// three layers below is one of those.
func liveHooks() diag.HooksState {
	return diag.HooksState{
		Known:      true,
		Enabled:    true,
		Loaded:     true,
		Managed:    true,
		UserDir:    "/home/u/.cozyphi/hooks",
		ProjectDir: "/w/project/.cozyphi/hooks",
		Events: []string{
			"pre_tool", "post_tool", "command",
			"session_start", "session_before_switch", "session_shutdown", "post_turn",
		},
		DefaultTimeout: 5 * time.Second,
		Hooks: []diag.HookFacts{
			{
				Name: "audit", Event: "post_tool", Plugin: "org", Tool: "*",
				Origins: []diag.HookOrigin{diag.HookOriginUser},
				Timeout: 5 * time.Second, Discovered: true, Registered: true, Async: true,
			},
			{
				Name: "deploy", Event: "command",
				Origins: []diag.HookOrigin{diag.HookOriginProject},
				Timeout: 5 * time.Second, Discovered: true, Registered: true,
			},
			{
				Name: "guard-bash", Event: "pre_tool", Tool: "bash",
				Origins: []diag.HookOrigin{diag.HookOriginUser, diag.HookOriginProject},
				Timeout: 12 * time.Second, Discovered: true, Registered: true, FailClosed: true,
			},
			{
				Name: "staged", Event: "pre_tool", Tool: "write",
				Origins: []diag.HookOrigin{diag.HookOriginProject},
				Timeout: 5 * time.Second, Discovered: true,
			},
			{
				Name: "in-process", Event: "pre_tool", Tool: "*",
				Origins: []diag.HookOrigin{diag.HookOriginProcess},
				Timeout: 5 * time.Second, Registered: true, FailClosed: true,
			},
		},
		Warnings: 1,
		Revision: "d4.r3.p1.w1",
	}
}

func hooksFields(t *testing.T, state diag.HooksState) []diag.Field {
	t.Helper()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewIntegrationCollector(diag.IntegrationDeps{Hooks: func() diag.HooksState { return state }}))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryIntegrations)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	require.False(t, snapshot.Truncated, "the category fits the response budget on its own")
	return snapshot.Categories[0].Fields
}

// hooksKeys is every key the layer answers, for the assertions that have to
// hold for all of them at once rather than for the ones somebody remembered.
var hooksKeys = []string{
	diag.KeyHooksRegistered,
	diag.KeyHooksEvents,
	diag.KeyHooksTools,
	diag.KeyHooksUser,
	diag.KeyHooksProject,
	diag.KeyHooksBlocking,
	diag.KeyHooksAsync,
	diag.KeyHooksTimeout,
}

// "No hooks ran" is six situations with one symptom, fixed in six different
// places — and two of them are not problems at all.
func TestTheLifecycleSeparatesTheWaysThereCanBeNoHook(t *testing.T) {
	tests := []struct {
		name  string
		state func(diag.HooksState) diag.HooksState
		want  diag.HooksLifecycle
	}{
		{
			name:  "switched off in the environment",
			state: func(s diag.HooksState) diag.HooksState { s.Enabled = false; return s },
			want:  diag.HooksDisabled,
		},
		{
			name:  "nobody read the directories",
			state: func(s diag.HooksState) diag.HooksState { s.Loaded, s.Managed = false, false; return s },
			want:  diag.HooksNotLoaded,
		},
		{
			name: "reading them was refused",
			state: func(s diag.HooksState) diag.HooksState {
				s.LoadFailed, s.Managed = true, false
				return s
			},
			want: diag.HooksLoadFailed,
		},
		{
			name:  "the load produced no manager",
			state: func(s diag.HooksState) diag.HooksState { s.Managed = false; return s },
			want:  diag.HooksNoManager,
		},
		{
			name: "a manager that holds nothing",
			state: func(s diag.HooksState) diag.HooksState {
				s.Hooks = []diag.HookFacts{{Name: "staged", Event: "pre_tool", Discovered: true}}
				return s
			},
			want: diag.HooksEmpty,
		},
		{
			name:  "hooks are loaded and nothing has triggered one",
			state: func(s diag.HooksState) diag.HooksState { return s },
			want:  diag.HooksActive,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := fieldByKey(t, hooksFields(t, tc.state(liveHooks())), diag.KeyHooksState)
			assert.Equal(t, string(tc.want), state.Effective.Value.Str)
			assert.Equal(t, diag.ApplyReload, state.Apply,
				"every one of them is changed by reloading, not by the next turn")
		})
	}
}

// The configured layer is what the environment allows, and it is a separate
// answer from what a load then found: hooks can be on with nothing to run.
func TestTheEnvironmentsAnswerIsNotTheLoadsAnswer(t *testing.T) {
	on := fieldByKey(t, hooksFields(t, liveHooks()), diag.KeyHooksState)
	assert.Equal(t, string(diag.HooksEnabled), on.Configured.Value.Str)
	assert.Equal(t, diag.SourceDefault, on.Configured.Source.Kind, "nothing had to be set for hooks to be on")
	assert.Equal(t, string(diag.HooksLoaded), on.Loaded.Value.Str)

	off := liveHooks()
	off.Enabled = false
	field := fieldByKey(t, hooksFields(t, off), diag.KeyHooksState)
	assert.Equal(t, string(diag.HooksDisabled), field.Configured.Value.Str)
	assert.Equal(t, diag.SourceEnv, field.Configured.Source.Kind)
	assert.Equal(t, "COZYPHI_HOOKS", field.Configured.Source.Ref)
	assert.Equal(t, string(diag.HooksLoaded), field.Loaded.Value.Str,
		"a load still happened; it stopped before reading a directory")
}

// A manifest the manager was built without is the whole of what a reload has
// not been asked to do yet. The manager is what runs, and a hook added since
// is not it.
func TestTheHooksThatCanFireAreNotTheHooksOnDisk(t *testing.T) {
	registered := fieldByKey(t, hooksFields(t, liveHooks()), diag.KeyHooksRegistered)

	assert.Equal(t, []string{"audit", "deploy", "guard-bash", "staged"}, registered.Configured.Value.List)
	assert.Equal(t, diag.SourceConfigFile, registered.Configured.Source.Kind)
	assert.Contains(t, registered.Configured.Source.Ref, "does not read them again",
		"the layer says it is a snapshot, which is what makes a stale one legible")

	assert.Equal(t, []string{"audit", "deploy", "guard-bash", "in-process"}, registered.Loaded.Value.List,
		"the manager holds one the load never saw and lacks one the load did")
	assert.Equal(t, registered.Loaded.Value.List, registered.Effective.Value.List,
		"what an event is fanned out to is what the manager holds")
	assert.Contains(t, registered.Effective.Source.Ref, "this view causes no event")
}

// Which directory defined a hook and which one supplied it are two answers,
// and a name in both directories is where they differ. That difference is
// the whole of what precedence did.
func TestPrecedenceIsStatedRatherThanLeftToBeInferred(t *testing.T) {
	fields := hooksFields(t, liveHooks())

	user := fieldByKey(t, fields, diag.KeyHooksUser)
	assert.Equal(t, []string{"audit", "guard-bash"}, user.Configured.Value.List, "the user directory names both")
	assert.Equal(t, "/home/u/.cozyphi/hooks", user.Configured.Source.Ref, "the path is the origin, not the value")
	assert.Equal(t, []string{"audit"}, user.Loaded.Value.List, "and supplied one of them")

	project := fieldByKey(t, fields, diag.KeyHooksProject)
	assert.Equal(t, []string{"deploy", "guard-bash", "staged"}, project.Configured.Value.List)
	assert.Equal(t, "/w/project/.cozyphi/hooks", project.Configured.Source.Ref)
	assert.Equal(t, []string{"deploy", "guard-bash", "staged"}, project.Loaded.Value.List,
		"a project hook replaces a user hook of the same name whole")
	assert.Contains(t, project.Loaded.Source.Ref, "replaces a user hook of the same name whole")

	for _, field := range []diag.Field{user, project} {
		assert.Equal(t, diag.StateNotApplicable, field.Effective.State,
			"precedence is spent by the time a hook is registered; there is no third answer")
		assert.Contains(t, field.Effective.Source.Ref, "spent by the time a hook is registered")
	}
}

// An event that reaches nobody and a tool nothing stands in front of are the
// two questions a reader asks before wondering why a hook did not fire.
func TestTheViewNarrowsTheBuildsEventsToTheOnesThatReachSomething(t *testing.T) {
	fields := hooksFields(t, liveHooks())

	events := fieldByKey(t, fields, diag.KeyHooksEvents)
	assert.Equal(t, liveHooks().Events, events.Configured.Value.List,
		"the vocabulary is the owner's, so the two cannot drift")
	assert.Equal(t, diag.SourceBuild, events.Configured.Source.Kind)
	assert.Equal(t, []string{"pre_tool", "post_tool", "command"}, events.Loaded.Value.List,
		"in the order this build fans them out, not the order the hooks happen to be in")
	assert.Equal(t, []string{"pre_tool", "post_tool", "command"}, events.Effective.Value.List)

	tools := fieldByKey(t, fields, diag.KeyHooksTools)
	assert.Equal(t, []string{"*", "bash", "write"}, tools.Configured.Value.List)
	assert.Equal(t, []string{"*", "bash"}, tools.Loaded.Value.List,
		"the hook that would have stood in front of write is not registered")
	assert.Equal(t, diag.StateNotApplicable, tools.Effective.State,
		"which hook a call meets is settled by making the call, and this view makes none")
	assert.Contains(t, tools.Effective.Source.Ref, "this view makes none")
}

// Whether a hook can deny a call and whether its answer reaches anything are
// the two properties that decide what a failing hook costs.
func TestTheViewSaysWhichHooksCanStopTheToolLoopAndWhichAreDetached(t *testing.T) {
	fields := hooksFields(t, liveHooks())

	blocking := fieldByKey(t, fields, diag.KeyHooksBlocking)
	assert.Equal(t, []string{"guard-bash"}, blocking.Configured.Value.List, "one manifest declares it")
	assert.Equal(t, []string{"guard-bash", "in-process"}, blocking.Loaded.Value.List,
		"and the manager holds another the directories never named")
	assert.Equal(t, blocking.Loaded.Value.List, blocking.Effective.Value.List)
	assert.Contains(t, blocking.Effective.Source.Ref, "readonly tool loop runs these and no others",
		"which is also what a readonly turn narrows to")

	async := fieldByKey(t, fields, diag.KeyHooksAsync)
	assert.Equal(t, []string{"audit"}, async.Configured.Value.List)
	assert.Equal(t, []string{"audit"}, async.Effective.Value.List)
	assert.Contains(t, async.Effective.Source.Ref, "cannot deny a call")
}

// A budget belongs to the call that spends it. The default and the longest
// one in force are both known; what one run will cost is not.
func TestTheTimeoutReportsTheBudgetAndNotASpentOne(t *testing.T) {
	timeout := fieldByKey(t, hooksFields(t, liveHooks()), diag.KeyHooksTimeout)

	assert.Equal(t, "5s", timeout.Configured.Value.Str)
	assert.Equal(t, diag.SourceDefault, timeout.Configured.Source.Kind)
	assert.Equal(t, "12s", timeout.Loaded.Value.Str, "the longest any registered hook carries")
	assert.Equal(t, diag.StateNotApplicable, timeout.Effective.State)

	empty := liveHooks()
	empty.Hooks = nil
	unset := fieldByKey(t, hooksFields(t, empty), diag.KeyHooksTimeout)
	assert.Equal(t, diag.StateUnset, unset.Loaded.State, "no hook is not a budget of no time at all")
}

// How much a load skipped is worth reporting; what it said is not. A warning
// names the plugin file it was found in and quotes the text that would not
// parse, and none of that may leave the owner.
func TestTheLoadOutcomeIsACategoryAndACountRatherThanAMessage(t *testing.T) {
	warned := fieldByKey(t, hooksFields(t, liveHooks()), diag.KeyHooksLoad)
	assert.Equal(t, diag.StateNotApplicable, warned.Configured.State, "nothing configures what a load meets")
	assert.Equal(t, string(diag.HookLoadWarned), warned.Loaded.Value.Str)
	assert.Equal(t, int64(1), warned.Effective.Value.Int)
	assert.Contains(t, warned.Effective.Source.Ref, "How many, never what")

	outcomes := map[diag.HookLoad]func(diag.HooksState) diag.HooksState{
		diag.HookLoadClean:        func(s diag.HooksState) diag.HooksState { s.Warnings = 0; return s },
		diag.HookLoadFailed:       func(s diag.HooksState) diag.HooksState { s.LoadFailed = true; return s },
		diag.HookLoadNotAttempted: func(s diag.HooksState) diag.HooksState { s.Loaded = false; return s },
	}
	for want, mutate := range outcomes {
		field := fieldByKey(t, hooksFields(t, mutate(liveHooks())), diag.KeyHooksLoad)
		assert.Equal(t, string(want), field.Loaded.Value.Str)
	}
}

// The lifecycle and the load answer even when nothing else can. They are what
// a reader needs exactly when the per-hook questions have no answer, so they
// may not go blank alongside them.
func TestSwitchedOffAndNeverLoadedAreNotAnEmptyHookList(t *testing.T) {
	off := liveHooks()
	off.Enabled = false
	for _, key := range hooksKeys {
		field := fieldByKey(t, hooksFields(t, off), key)
		assert.Equal(t, diag.StateNotApplicable, field.Effective.State, key)
		assert.Contains(t, field.Effective.Source.Ref, "switched hooks off", key)
	}
	assert.Equal(t, string(diag.HooksDisabled),
		fieldByKey(t, hooksFields(t, off), diag.KeyHooksState).Effective.Value.Str,
		"the lifecycle still answers, which is the point of asking")

	// Off is reported ahead of the empty discovery: switching hooks off is
	// what stops the directories from being read, so naming the consequence
	// would hide an answer that is known behind one that is not.
	unloaded := liveHooks()
	unloaded.Loaded, unloaded.Managed = false, false
	for _, key := range hooksKeys {
		assert.Equal(t, diag.StateUnavailable, fieldByKey(t, hooksFields(t, unloaded), key).Effective.State, key)
	}
	assert.Equal(t, string(diag.HookLoadNotAttempted),
		fieldByKey(t, hooksFields(t, unloaded), diag.KeyHooksLoad).Loaded.Value.Str)

	failed := liveHooks()
	failed.LoadFailed, failed.Managed = true, false
	for _, key := range hooksKeys {
		field := fieldByKey(t, hooksFields(t, failed), key)
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, key)
		assert.Contains(t, field.Effective.Source.Ref, "never its text", key)
	}

	unwired := hooksFields(t, diag.HooksState{})
	for _, key := range append(hooksKeys, diag.KeyHooksState, diag.KeyHooksLoad) {
		field := fieldByKey(t, unwired, key)
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, key)
		assert.Empty(t, field.Revision, "an unobserved hook layer has no state to fingerprint")
	}
}

// The snapshot must not alias the owner's slices: a reader that sorts a list
// it was handed must not reorder what the manager's record holds.
func TestTheHooksSnapshotIsDetachedFromTheOwnersState(t *testing.T) {
	state := liveHooks()
	fields := hooksFields(t, state)

	fieldByKey(t, fields, diag.KeyHooksRegistered).Configured.Value.List[0] = "mutated"
	fieldByKey(t, fields, diag.KeyHooksEvents).Configured.Value.List[0] = "mutated"
	fieldByKey(t, fields, diag.KeyHooksTools).Loaded.Value.List[0] = "mutated"

	assert.Equal(t, "audit", state.Hooks[0].Name)
	assert.Equal(t, "pre_tool", state.Events[0])
	assert.Equal(t, "*", state.Hooks[0].Tool)
}

// The seam is an allowlist by construction. A script, a run path, a plugin
// file, a hook directory, an argument, an environment entry or the text of a
// warning cannot be leaked by a view that has no member to put one in — so
// the shape of the struct is the guarantee, and this is the test that notices
// when someone widens it.
func TestNoHookFactHasSomewhereForASecretToLand(t *testing.T) {
	allowed := map[string]string{
		"Name":       "string",
		"Event":      "string",
		"Origins":    "[]diag.HookOrigin",
		"Plugin":     "string",
		"Tool":       "string",
		"Timeout":    "time.Duration",
		"Discovered": "bool",
		"Registered": "bool",
		"FailClosed": "bool",
		"Async":      "bool",
	}

	facts := reflect.TypeFor[diag.HookFacts]()
	assert.Len(t, allowed, facts.NumField(),
		"a new member of this struct is a new way for a hook's script or environment to reach a transcript")
	for field := range facts.Fields() {
		want, ok := allowed[field.Name]
		require.True(t, ok, "undeclared member %q: state why it cannot carry a secret before adding it", field.Name)
		assert.Equal(t, want, field.Type.String(), field.Name)
	}
}
