package hooks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// sentinelScript is what every fixture hook runs. If a view ever fires one,
// the marker file it appends to is the proof — and the proof survives the
// test that made it, which a spy inside the process would not.
const sentinelScript = "#!/bin/sh\necho SENTINEL-hook-executed >> %s\nexit 0\n"

// writeHookPlugin writes one hook directory: a manifest and the script its
// entries name. The script is real and executable, because a hook the loader
// refused to accept would prove nothing about a view that declines to run it.
func writeHookPlugin(t *testing.T, dir, marker, manifest string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, PluginFileName), []byte(manifest), 0o644))
	script := []byte(strings.Replace(sentinelScript, "%s", marker, 1))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.sh"), script, 0o755))
}

// hookDirs is the arrangement worth telling apart: one name defined in both
// directories, one defined only by the user, and one defined only by the
// project. Every script would announce itself, and none of them may.
func hookDirs(t *testing.T) (userDir, projectDir, marker string) {
	t.Helper()
	marker = filepath.Join(t.TempDir(), "SENTINEL-hook-ran")
	userDir = filepath.Join(t.TempDir(), "user-hooks")
	projectDir = filepath.Join(t.TempDir(), "project-hooks")

	writeHookPlugin(t, userDir, marker, `{"hooks":[
	  {"name":"guard-bash","event":"pre_tool","match":"bash","run":"./run.sh","fail_closed":true,"timeout":"9s"},
	  {"name":"audit","event":"post_tool","run":"./run.sh","async":true}
	]}`)
	writeHookPlugin(t, projectDir, marker, `{"hooks":[
	  {"name":"guard-bash","event":"pre_tool","match":"bash","run":"./run.sh","timeout":"12s"},
	  {"name":"deploy","event":"command","run":"./run.sh"}
	]}`)
	return userDir, projectDir, marker
}

// observedHooks indexes one observation by hook name, which is how every
// assertion below reads it: the list order is the loader's and says nothing.
func observedHooks(t *testing.T, state diag.HooksState) map[string]diag.HookFacts {
	t.Helper()
	out := make(map[string]diag.HookFacts, len(state.Hooks))
	for _, hook := range state.Hooks {
		out[hook.Name] = hook
	}
	return out
}

// Where a hook came from is the question the manager cannot answer: it holds
// entries, and the merge that produced them is over. The load's record is
// what keeps precedence legible afterwards.
func TestTheViewNamesEveryHookAndWhichDirectorySuppliedIt(t *testing.T) {
	userDir, projectDir, _ := hookDirs(t)

	mgr, facts, warns, err := LoadObserved(userDir, projectDir)
	require.NoError(t, err)
	require.Empty(t, warns)
	state := Observe(mgr, facts)

	assert.True(t, state.Known)
	assert.True(t, state.Enabled)
	assert.True(t, state.Loaded)
	assert.False(t, state.LoadFailed)
	assert.True(t, state.Managed)
	assert.Equal(t, defaultTimeout, state.DefaultTimeout)
	assert.Equal(t, eventVocabulary(), state.Events, "the vocabulary comes from the manager's own kinds")
	require.Len(t, state.Hooks, 3, "two names from the project, one from the user, and the shadow is not a fourth")

	hooks := observedHooks(t, state)
	assert.Equal(t, []diag.HookOrigin{diag.HookOriginUser}, hooks["audit"].Origins)
	assert.Equal(t, []diag.HookOrigin{diag.HookOriginProject}, hooks["deploy"].Origins)
	assert.Equal(t, []diag.HookOrigin{diag.HookOriginUser, diag.HookOriginProject}, hooks["guard-bash"].Origins,
		"both directories name it, and the last one is the definition in force")

	guard := hooks["guard-bash"]
	assert.False(t, guard.FailClosed, "the project's definition replaced the user's whole, settings included")
	assert.Equal(t, 12*time.Second, guard.Timeout, "and so did its budget")
	assert.Equal(t, "bash", guard.Tool)
	assert.True(t, guard.Discovered)
	assert.True(t, guard.Registered)

	assert.True(t, hooks["audit"].Async)
	assert.Equal(t, everyTool, hooks["audit"].Tool,
		"a tool-loop hook that names no tool stands in front of all of them")
	assert.Equal(t, defaultTimeout, hooks["audit"].Timeout, "a manifest that declares no budget is given the default")

	assert.Empty(t, hooks["deploy"].Tool, "a slash command is not selected by a tool name, and none is invented for it")
	assert.Equal(t, string(KindCommand), hooks["deploy"].Event)
}

// The question must be free to ask. A view that fired a hook to find out
// whether it is loaded would let a developer deny their own tool call by
// looking at a list.
func TestObservingHooksRunsNoneOfThem(t *testing.T) {
	userDir, projectDir, marker := hookDirs(t)
	mgr, facts, _, err := LoadObserved(userDir, projectDir)
	require.NoError(t, err)

	var last diag.HooksState
	for range 5 {
		last = Observe(mgr, facts)
		require.Len(t, last.Hooks, 3)
		assert.Equal(t, "d3.r3.p0.w0", last.Revision, "five reads of one state are five reads of one state")
	}

	_, err = os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "no hook script ran, so nothing wrote the marker")

	// And the scripts themselves stay with the owner: what leaves is a name,
	// an event, an origin, a selector, a budget and two flags.
	for _, hook := range last.Hooks {
		assert.NotContains(t, hook.Name, "run.sh")
		assert.NotContains(t, hook.Tool, "run.sh")
		assert.NotContains(t, hook.Plugin, marker)
	}
}

// Observing hooks is not the tool loop's own pre/post hooks. The manager the
// executor fans out through — and the readonly view of it — must be exactly
// what it was before anybody looked.
func TestTheToolLoopKeepsItsOwnHooksAfterAnObservation(t *testing.T) {
	var pre, post int
	mgr := NewManager(
		Entry{Hook: FuncHook{
			HookName: "guard",
			Pre: func(context.Context, Event) (PreResult, error) {
				pre++
				return PreResult{Action: ActionAllow}, nil
			},
		}, Kind: KindPreTool, FailClosed: true},
		Entry{Hook: FuncHook{
			HookName: "audit",
			Post: func(context.Context, Event) (PostResult, error) {
				post++
				return PostResult{Context: "seen"}, nil
			},
		}, Kind: KindPostTool},
	)

	for range 3 {
		state := Observe(mgr, LoadFacts{attempted: true})
		require.Len(t, state.Hooks, 2)
	}
	assert.Zero(t, pre, "an observation is not an event")
	assert.Zero(t, post)

	ev := Event{Tool: "bash", Input: json.RawMessage(`{}`)}
	assert.False(t, mgr.PreTool(t.Context(), ev).Denied)
	assert.Equal(t, "seen", mgr.PostTool(t.Context(), ev).Context)
	assert.Equal(t, 1, pre, "the tool loop's own hooks still run")
	assert.Equal(t, 1, post)

	// The readonly view the executor swaps in is untouched too: it still
	// carries the fail-closed hook and still leaves the other one out.
	readonly := mgr.FailClosedOnly()
	assert.False(t, readonly.PreTool(t.Context(), ev).Denied)
	assert.Empty(t, readonly.PostTool(t.Context(), ev).Context)
	assert.Equal(t, 2, pre)
	assert.Equal(t, 1, post, "a readonly turn was already skipping the audit hook, and still is")
}

// A load that skipped something is neither clean nor failed, and the count is
// the whole of what may be said: a warning quotes the file it was found in
// and the text that would not parse.
func TestALoadThatSkippedSomethingSaysHowMuchAndNeverWhat(t *testing.T) {
	userDir, projectDir, _ := hookDirs(t)
	broken := filepath.Join(userDir, "SENTINEL-broken")
	require.NoError(t, os.MkdirAll(broken, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(broken, PluginFileName),
		[]byte(`{"hooks": [SENTINEL-secret-token]}`), 0o644))

	mgr, facts, warns, err := LoadObserved(userDir, projectDir)
	require.NoError(t, err, "one unreadable manifest is a warning, not a failed load")
	require.Len(t, warns, 1)
	require.Contains(t, warns[0].String(), "SENTINEL-broken",
		"the warning does name the file it could not read, which is exactly why it stays here")

	state := Observe(mgr, facts)
	assert.Equal(t, 1, state.Warnings)
	assert.Equal(t, "d3.r3.p0.w1", state.Revision, "and the count is part of what a reader compares across a reload")

	flattened := sprintState(state)
	assert.NotContains(t, flattened, "SENTINEL-broken", "nothing of the file the load skipped travels")
	assert.NotContains(t, flattened, "SENTINEL-secret-token")
}

// sprintState flattens everything a HooksState can carry as text, so one
// assertion covers every member rather than the members somebody remembered.
func sprintState(state diag.HooksState) string {
	parts := []string{state.UserDir, state.ProjectDir, state.Revision}
	parts = append(parts, state.Events...)
	for _, hook := range state.Hooks {
		parts = append(parts, hook.Name, hook.Event, hook.Plugin, hook.Tool)
		for _, origin := range hook.Origins {
			parts = append(parts, string(origin))
		}
	}
	return strings.Join(parts, " ")
}

// "No hooks" is several unrelated situations, and the ones a reader has to
// separate are a load that produced no manager and a manager holding nothing.
func TestAMissingManagerIsNotAnEmptySetOfHooks(t *testing.T) {
	userDir, projectDir, _ := hookDirs(t)
	_, facts, _, err := LoadObserved(userDir, projectDir)
	require.NoError(t, err)

	none := Observe(nil, facts)
	assert.True(t, none.Loaded, "the directories were read")
	assert.False(t, none.Managed, "and nothing became of what they held")
	require.Len(t, none.Hooks, 3)
	for _, hook := range none.Hooks {
		assert.True(t, hook.Discovered, hook.Name)
		assert.False(t, hook.Registered, hook.Name)
	}

	empty := Observe(NewManager(), facts)
	assert.True(t, empty.Managed, "a manager exists; it simply holds nothing")
	for _, hook := range empty.Hooks {
		assert.False(t, hook.Registered, hook.Name)
	}

	unloaded := Observe(nil, LoadFacts{})
	assert.False(t, unloaded.Known, "nobody loaded hooks in this process, which is not the same as finding none")
	assert.Empty(t, unloaded.Hooks)
}

// An entry no manifest accounts for is reported as itself. A built-in policy
// or a hook a test registered is exactly what a reader wants to see, and
// dropping it would describe a session that is not the one running.
func TestAnEntryNoManifestAccountsForIsReportedAsTheProcessesOwn(t *testing.T) {
	userDir, projectDir, _ := hookDirs(t)
	_, facts, _, err := LoadObserved(userDir, projectDir)
	require.NoError(t, err)

	mgr := NewManager(Entry{
		Hook: FuncHook{HookName: "in-process"},
		Kind: KindPreTool,
	})
	state := Observe(mgr, facts)
	require.Len(t, state.Hooks, 4)

	extra := observedHooks(t, state)["in-process"]
	assert.Equal(t, []diag.HookOrigin{diag.HookOriginProcess}, extra.Origins)
	assert.False(t, extra.Discovered, "no directory defines it, so a reload does not change it")
	assert.True(t, extra.Registered)
	assert.Empty(t, extra.Plugin)
	assert.Equal(t, "d3.r1.p1.w0", state.Revision,
		"three discovered, one of them registered, and one entry from somewhere else")
}

// Switching hooks off reads no directory at all, and the empty discovery that
// follows is the consequence rather than the answer.
func TestSwitchingHooksOffIsNotAnEmptyHookDirectory(t *testing.T) {
	userDir, projectDir, marker := hookDirs(t)
	t.Setenv(EnvHooks, "off")

	mgr, facts, warns, err := LoadObserved(userDir, projectDir)
	require.NoError(t, err)
	assert.Empty(t, warns)

	state := Observe(mgr, facts)
	assert.True(t, state.Known)
	assert.False(t, state.Enabled)
	assert.True(t, state.Loaded, "a load happened; it stopped before reading anything")
	assert.Empty(t, state.Hooks)

	_, err = os.Stat(marker)
	assert.True(t, os.IsNotExist(err))
}

// The configured layer is the load's snapshot, not the disk. A directory
// edited since is answered by the load — which is what makes the answer worth
// having, and what the reload advice on every field is for.
func TestTheRecordOutlivesTheDirectoryItRead(t *testing.T) {
	userDir, projectDir, marker := hookDirs(t)
	mgr, facts, _, err := LoadObserved(userDir, projectDir)
	require.NoError(t, err)
	before := Observe(mgr, facts)
	require.Len(t, before.Hooks, 3)

	writeHookPlugin(t, filepath.Join(projectDir, "added"), marker,
		`{"hooks":[{"name":"added","event":"post_turn","run":"./run.sh"}]}`)

	assert.Len(t, Observe(mgr, facts).Hooks, 3, "the view does not read the directory again")
	assert.Equal(t, before.Revision, Observe(mgr, facts).Revision)

	reloaded, reloadedFacts, _, err := LoadObserved(userDir, projectDir)
	require.NoError(t, err)
	after := Observe(reloaded, reloadedFacts)
	assert.Len(t, after.Hooks, 4, "and a reload is what changes the answer")
	assert.NotEqual(t, before.Revision, after.Revision, "two snapshots across a reload are visibly of two states")
}

// The record is an allowlist by construction: a run path, a plugin file, a
// hook directory or the text of a warning cannot be leaked by a struct that
// has no member to put one in. This is the test that notices a widening.
func TestNoManifestRecordHasSomewhereForAScriptToLand(t *testing.T) {
	allowed := map[string]string{
		"name":       "string",
		"event":      "hooks.Kind",
		"origins":    "[]string",
		"plugin":     "string",
		"match":      "string",
		"timeout":    "time.Duration",
		"failClosed": "bool",
		"async":      "bool",
	}

	facts := reflect.TypeFor[manifestFacts]()
	assert.Len(t, allowed, facts.NumField(),
		"a new member here is a new way for a hook's script or arguments to reach a transcript")
	for field := range facts.Fields() {
		want, ok := allowed[field.Name]
		require.True(t, ok, "undeclared member %q: state why it cannot carry a secret before adding it", field.Name)
		assert.Equal(t, want, field.Type.String(), field.Name)
	}
}
