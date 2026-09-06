package diag

import (
	"slices"
	"time"
)

// Hooks field keys. The hook layer takes its own namespace inside the
// integrations category for the same reason MCP and LSP do: a key is a
// stable address for Explain, and the three services grow independently.
//
//nolint:gosec // G101: field addresses, not credentials; no value is one.
const (
	KeyHooksState      = "hooks.state"
	KeyHooksRegistered = "hooks.registered"
	KeyHooksEvents     = "hooks.events"
	KeyHooksTools      = "hooks.tools"
	KeyHooksUser       = "hooks.source.user"
	KeyHooksProject    = "hooks.source.project"
	KeyHooksBlocking   = "hooks.blocking"
	KeyHooksAsync      = "hooks.async"
	KeyHooksTimeout    = "hooks.timeout"
	KeyHooksLoad       = "hooks.load"
)

// HooksLifecycle is where the hook subsystem stands. The values separate the
// things "no hooks ran" could otherwise mean, which have nothing in common
// but the symptom: switched off in the environment, never loaded, a load
// that failed, a load that produced no manager, a manager holding nothing,
// and a manager holding something that has simply not been triggered yet.
type HooksLifecycle string

// HooksLifecycle values.
const (
	// HooksEnabled means the environment leaves hooks on. It is the
	// configured layer's answer and says nothing about whether anything was
	// found or loaded.
	HooksEnabled HooksLifecycle = "enabled"
	// HooksDisabled means the environment switched hooks off, so discovery
	// read no directory and nothing below applies.
	HooksDisabled HooksLifecycle = "disabled"
	// HooksLoaded means the directories were read and a manager was built.
	HooksLoaded HooksLifecycle = "loaded"
	// HooksNotLoaded means nobody loaded hooks in this process — no
	// discovery was attempted.
	HooksNotLoaded HooksLifecycle = "not_loaded"
	// HooksLoadFailed means discovery was attempted and failed. What went
	// wrong is not reported; the failure can quote a directory.
	HooksLoadFailed HooksLifecycle = "load_failed"
	// HooksNoManager means a load happened and no manager is in force. It is
	// not the same as an empty set of hooks: nothing will run for any event,
	// including the ones a manifest declares.
	HooksNoManager HooksLifecycle = "no_manager"
	// HooksEmpty means a manager is in force and holds no entry. Every event
	// is fanned out to nothing, which is the ordinary state of a session
	// with no hooks installed.
	HooksEmpty HooksLifecycle = "empty"
	// HooksActive means a manager is in force and holds at least one entry
	// that will be fanned out to. Whether one has ever fired is a different
	// question, and this view has not made any of them fire.
	HooksActive HooksLifecycle = "active"
)

// HookOrigin names one place a hook definition came from. The first two
// values are also the precedence order: a user definition is replaced whole
// by a project one of the same name.
type HookOrigin string

// HookOrigin values, in precedence order.
const (
	// HookOriginUser is the user's own hook directory.
	HookOriginUser HookOrigin = "user"
	// HookOriginProject is the workspace's hook directory, which replaces a
	// user hook of the same name entirely.
	HookOriginProject HookOrigin = "project"
	// HookOriginProcess is an entry the process registered itself. No
	// directory defines it, so a reload does not change it and no manifest
	// says what it does.
	HookOriginProcess HookOrigin = "process"
)

// HookLoad is what the last load of the hook directories came to. It is a
// category rather than a message: a load problem names the plugin file it
// was found in and quotes the text that would not parse, and none of that
// may leave the owner.
type HookLoad string

// HookLoad values.
const (
	// HookLoadNotAttempted means nobody has loaded hooks in this process.
	HookLoadNotAttempted HookLoad = "not_attempted"
	// HookLoadClean means the load finished and met no problem.
	HookLoadClean HookLoad = "clean"
	// HookLoadWarned means the load finished and skipped something on the
	// way: a manifest that would not parse, a duplicate name, a run path
	// that would not resolve.
	HookLoadWarned HookLoad = "warned"
	// HookLoadFailed means the load itself did not finish, so what the
	// directories hold is not known here at all.
	HookLoadFailed HookLoad = "failed"
)

// HookFacts is what the harness may say about one hook. It is an allowlist
// by construction: the name the manifest chose, the event it answers, where
// it was defined, the tool it applies to and how it behaves — with no member
// for the script, the run path, the plugin file, the hook directory, an
// argument, an environment entry or the text of an error to land in.
type HookFacts struct {
	// Name is the name the manifest declares, which is also what a deny
	// reason and the hook palette already show.
	Name string
	// Event is the event this hook answers, in the manager's own vocabulary.
	Event string
	// Origins is every source that defines this name, in precedence order,
	// so the last one is the definition in force.
	Origins []HookOrigin
	// Plugin is the enclosing plugin id, which is how one directory groups
	// several hooks. Empty for an entry no manifest accounts for.
	Plugin string
	// Tool is the tool this hook applies to: a tool name, "*" for every
	// tool, and empty for a hook that is not on the tool loop at all.
	Tool string
	// Timeout is the budget one run of this hook is given.
	Timeout time.Duration
	// Discovered is whether the last load found a manifest for it.
	Discovered bool
	// Registered is whether the manager in force actually holds it. A
	// discovered hook that is not registered is one the manager was built
	// without — a load newer than the manager, or a manager built elsewhere.
	Registered bool
	// FailClosed is whether this hook's own failure denies the call instead
	// of being skipped.
	FailClosed bool
	// Async is whether it is fired and detached, so its result reaches
	// nothing.
	Async bool
}

// HooksState is one observation of the hook layer: whether the subsystem is
// on, what the last load of the directories found, whether a manager is in
// force, one entry per hook, and what the load came to.
type HooksState struct {
	// Known is false when nobody published an observation — the layer is not
	// wired, rather than wired and empty. Every field it feeds then reports
	// unavailable instead of an empty list that would read as "no hooks are
	// configured".
	Known bool
	// Enabled is whether the environment leaves hooks on.
	Enabled bool
	// Loaded is whether a load of the directories was attempted.
	Loaded bool
	// LoadFailed is whether that load failed.
	LoadFailed bool
	// Managed is whether a manager is in force. It is a separate answer from
	// Loaded: a load can finish and leave no manager behind, and that is not
	// the same as a manager holding nothing.
	Managed bool
	// UserDir and ProjectDir are the two directories the load consulted, in
	// precedence order. They are reported as the origin of a layer rather
	// than as a value, and the registry collapses the home directory before
	// either reaches a reader.
	UserDir    string
	ProjectDir string
	// Events is the event vocabulary this build fans out, as the owner
	// spells it, so the two cannot drift apart.
	Events []string
	// DefaultTimeout is the budget a hook that declares none is given.
	DefaultTimeout time.Duration
	// Hooks is one entry per hook the load found or the manager holds.
	Hooks []HookFacts
	// Warnings is how many non-fatal problems the last load met. How many,
	// never what: a problem quotes the file it was found in.
	Warnings int
	// Revision fingerprints the state this observation describes, so two
	// snapshots taken across a reload are visibly of two different states.
	Revision string
}

// Sources for the layers whose origin is the view's own reading rather than
// an owner's record.
var (
	sourceHooksOnByDefault = Source{
		Kind: SourceDefault,
		Ref:  "hooks are on unless COZYPHI_HOOKS switches them off",
	}
	sourceHooksSwitchedOff = Source{
		Kind: SourceEnv,
		Ref:  "COZYPHI_HOOKS",
	}
	sourceHooksOff = Source{
		Kind: SourceEnv,
		Ref:  "COZYPHI_HOOKS switched hooks off, so no directory was read",
	}
	sourceHooksLoad = Source{
		Kind: SourceComputed,
		Ref:  "whether the hook directories were read when this session's manager was last built",
	}
	sourceHooksManager = Source{
		Kind: SourceSession,
		Ref:  "the hook manager this session holds",
	}
	sourceHooksLoadRefused = Source{
		Kind: SourceComputed,
		Ref: "the load did not finish, so what the hook directories hold is not known here; the failure is a " +
			"category and never its text, which can quote the directory it could not read",
	}
	sourceHooksDiscovered = Source{
		Kind: SourceConfigFile,
		Ref: "the manifests the last load found; this view does not read them again, so a directory edited " +
			"since is answered by the load",
	}
	sourceHooksEntries = Source{
		Kind: SourceSession,
		Ref: "the entries the manager in force holds; a name discovered but missing here belongs to a load " +
			"newer than the manager",
	}
	sourceHooksFanout = Source{
		Kind: SourceSession,
		Ref:  "the entries an event is fanned out to when one occurs; this view causes no event",
	}
	sourceHooksEventVocabulary = Source{
		Kind: SourceBuild,
		Ref:  "the events this build fans out; a manifest naming another one is skipped",
	}
	sourceHooksEventsDiscovered = Source{
		Kind: SourceConfigFile,
		Ref:  "the events the discovered manifests declare",
	}
	sourceHooksEventsRegistered = Source{
		Kind: SourceSession,
		Ref:  "the events the manager has an entry for; every other one is fanned out to nothing",
	}
	sourceHooksSelectorsDeclared = Source{
		Kind: SourceConfigFile,
		Ref:  "the tool each discovered tool-loop hook applies to, with \"*\" for one that names none",
	}
	sourceHooksSelectorsLoaded = Source{
		Kind: SourceSession,
		Ref:  "the tools the registered tool-loop hooks apply to",
	}
	sourceHooksSelectorAtCall = Source{
		Kind: SourceComputed,
		Ref: "which hook a call meets is settled when the call is made, by matching its tool name against " +
			"these selectors; this view makes none",
	}
	sourceHooksWins = Source{
		Kind: SourceComputed,
		Ref: "the hooks this source supplied; a project hook replaces a user hook of the same name whole, " +
			"script and settings together",
	}
	sourceHooksPrecedence = Source{
		Kind: SourceComputed,
		Ref: "precedence is spent by the time a hook is registered: a name in both directories is declared " +
			"by both and supplied by one",
	}
	sourceHooksBlockingDeclared = Source{
		Kind: SourceConfigFile,
		Ref:  "the discovered hooks whose manifest declares fail_closed",
	}
	sourceHooksBlockingLoaded = Source{
		Kind: SourceSession,
		Ref:  "the registered hooks whose own failure denies the call",
	}
	sourceHooksBlockingReadonly = Source{
		Kind: SourceComputed,
		Ref:  "a readonly tool loop runs these and no others, so an audit hook cannot stall an exploratory turn",
	}
	sourceHooksAsyncDeclared = Source{
		Kind: SourceConfigFile,
		Ref:  "the discovered hooks whose manifest declares async",
	}
	sourceHooksAsyncLoaded = Source{
		Kind: SourceSession,
		Ref:  "the registered hooks that are fired and detached",
	}
	sourceHooksAsyncDetached = Source{
		Kind: SourceComputed,
		Ref:  "a detached hook's answer reaches nothing: it cannot deny a call, rewrite a result or stop a turn",
	}
	sourceHooksTimeoutDefault = Source{
		Kind: SourceDefault,
		Ref:  "the budget one run of a hook that declares no timeout is given",
	}
	sourceHooksTimeoutLoaded = Source{
		Kind: SourceSession,
		Ref:  "the longest budget any registered hook carries; the load caps a manifest that asks for more",
	}
	sourceHooksTimeoutAtCall = Source{
		Kind: SourceComputed,
		Ref:  "a budget is spent by the call that runs a hook, and this view runs none",
	}
	sourceHooksLoadUnconfigured = Source{
		Kind: SourceComputed,
		Ref:  "nothing configures what a load meets; it is what happened",
	}
	sourceHooksLoadOutcome = Source{
		Kind: SourceComputed,
		Ref:  "what the last load of the hook directories came to",
	}
	sourceHooksWarningCount = Source{
		Kind: SourceComputed,
		Ref: "how many non-fatal problems that load met. How many, never what: a problem quotes the file it " +
			"was found in",
	}
)

// lifecycle separates the things "no hooks ran" could mean: switched off,
// never loaded, a load that failed, a load that left no manager, a manager
// holding nothing, and a manager holding something nothing has triggered.
// They are fixed in entirely different places, and two of them are not
// problems at all.
func (s HooksState) lifecycle() Field {
	field := s.field(KeyHooksState)
	if !s.Known {
		return field
	}
	setting, source := HooksEnabled, sourceHooksOnByDefault
	if !s.Enabled {
		setting, source = HooksDisabled, sourceHooksSwitchedOff
	}
	field.Configured = Present(StringValue(string(setting)), source)
	field.Loaded = Present(StringValue(string(s.loadState())), sourceHooksLoad)
	field.Effective = Present(StringValue(string(s.liveState())), sourceHooksManager)
	return field
}

// registered is the direct answer to "what can fire in this session": the
// names the last load found, the names the manager in force holds, and the
// names an event is actually fanned out to. A gap between the first two is
// the whole of what a reload has not been asked to do yet — the manager is
// what runs, and a manifest added since is not it.
func (s HooksState) registered() Field {
	field := s.field(KeyHooksRegistered)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = Present(ListValue(s.names(discoveredHook)), sourceHooksDiscovered)
	field.Loaded = Present(ListValue(s.names(registeredHook)), sourceHooksEntries)
	field.Effective = Present(ListValue(s.names(registeredHook)), sourceHooksFanout)
	return field
}

// events narrows the build's own vocabulary twice: the events some manifest
// declares, and the events the manager can actually fan out to. An event
// missing from the last list is one that happens and reaches nobody.
func (s HooksState) events() Field {
	field := s.field(KeyHooksEvents)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = Present(ListValue(s.Events), sourceHooksEventVocabulary)
	field.Loaded = Present(ListValue(s.eventSet(discoveredHook)), sourceHooksEventsDiscovered)
	field.Effective = Present(ListValue(s.eventSet(registeredHook)), sourceHooksEventsRegistered)
	return field
}

// tools is which tool calls a hook stands in front of. The selectors are
// declared by the manifests and carried into the manager unchanged; which
// hook a particular call meets is settled by matching that call's tool name,
// which is why the acting layer does not exist rather than repeating the
// list as though every one of them fires.
func (s HooksState) tools() Field {
	field := s.field(KeyHooksTools)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = Present(ListValue(s.toolSet(discoveredHook)), sourceHooksSelectorsDeclared)
	field.Loaded = Present(ListValue(s.toolSet(registeredHook)), sourceHooksSelectorsLoaded)
	field.Effective = notApplicable(sourceHooksSelectorAtCall)
	return field
}

// definedIn reports one hook directory twice over: what it defines, and what
// it actually supplied. A hook named by both directories appears in the first
// list of both and the second list of one, which is the whole of what
// precedence did — stated, rather than left to be inferred from a merge
// nobody watched.
func (s HooksState) definedIn(key string, origin HookOrigin) Field {
	field := s.field(key)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = Present(ListValue(s.names(func(h HookFacts) bool {
		return slices.Contains(h.Origins, origin)
	})), s.directory(origin))
	field.Loaded = Present(ListValue(s.names(func(h HookFacts) bool {
		return winningHookOrigin(h.Origins) == origin
	})), sourceHooksWins)
	field.Effective = notApplicable(sourceHooksPrecedence)
	return field
}

// blocking is which hooks can stop the tool loop. A hook that is not here
// fails open — its own error is logged and skipped — and one that is here
// turns that same error into a deny. It is also the set a readonly turn runs
// on its own, which is why the acting layer is a set rather than a repeat.
func (s HooksState) blocking() Field {
	return s.flagged(KeyHooksBlocking, blockingHook,
		sourceHooksBlockingDeclared, sourceHooksBlockingLoaded, sourceHooksBlockingReadonly)
}

// async is which hooks are fired and forgotten. Their answers reach nothing,
// so a detached hook cannot deny a call, rewrite a result or stop a turn
// however it ends.
func (s HooksState) async() Field {
	return s.flagged(KeyHooksAsync, detachedHook,
		sourceHooksAsyncDeclared, sourceHooksAsyncLoaded, sourceHooksAsyncDetached)
}

// flagged builds the declared/registered/acting triple over one property of
// a hook, so the two behavior fields cannot disagree about which hooks the
// manager holds.
func (s HooksState) flagged(key string, has func(HookFacts) bool, declared, loaded, acting Source) Field {
	field := s.field(key)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	registered := ListValue(s.names(func(h HookFacts) bool { return registeredHook(h) && has(h) }))
	field.Configured = Present(ListValue(s.names(func(h HookFacts) bool {
		return discoveredHook(h) && has(h)
	})), declared)
	field.Loaded = Present(registered, loaded)
	field.Effective = Present(registered, acting)
	return field
}

// timeout is how long one hook is given. The default answers for a manifest
// that names none, the longest budget in force says what the slowest event
// can cost this session, and the acting layer does not exist: a budget is
// spent by a call, and this view makes none.
func (s HooksState) timeout() Field {
	field := s.field(KeyHooksTimeout)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = Present(DurationValue(s.DefaultTimeout), sourceHooksTimeoutDefault)
	longest := s.longestTimeout()
	field.Loaded = Present(DurationValue(longest), sourceHooksTimeoutLoaded)
	if longest == 0 {
		field.Loaded = Unset(DurationValue(0), sourceHooksTimeoutLoaded)
	}
	field.Effective = notApplicable(sourceHooksTimeoutAtCall)
	return field
}

// load is what the last read of the hook directories came to and how much it
// had to skip. It answers even when the subsystem is off or the load failed,
// because that is exactly when a reader needs it — and it answers as a
// category and a count, never as the text of a problem, which quotes the
// file it was found in.
func (s HooksState) load() Field {
	field := s.field(KeyHooksLoad)
	if !s.Known {
		return field
	}
	field.Configured = notApplicable(sourceHooksLoadUnconfigured)
	field.Loaded = Present(StringValue(string(s.loadOutcome())), sourceHooksLoadOutcome)
	field.Effective = Present(IntValue(int64(s.Warnings)), sourceHooksWarningCount)
	return field
}

// field is the shape every hooks field starts from: all three layers
// unavailable, so a layer nobody wired degrades into an honest answer rather
// than into an empty list that would read as "nothing is configured".
//
// Every one of them applies at reload, and none of them takes an apply of its
// own. A hook is a file on disk read once: changing what one does, adding a
// name or taking one away is answered by re-reading the directories, and
// nothing here can be talked into effect any sooner or costs any more than
// that.
func (s HooksState) field(key string) Field {
	return Field{
		Key:        key,
		Configured: Unavailable(),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      ApplyReload,
		Scope:      ScopeWorkspace,
		Revision:   s.Revision,
	}
}

// absent reports whether a per-hook question can be answered at all, and what
// to say when it cannot. The cases are not the same: a subsystem switched off
// read no directory, a process that never loaded cannot say what one holds,
// and a load that failed knows less than either. Off is tested first because
// switching hooks off is what stops the directories from being read — the
// empty discovery is the consequence, and reporting it as the reason would
// hide an answer that is known for one that is not.
func (s HooksState) absent() (Observation, bool) {
	switch {
	case !s.Known:
		return Unavailable(), true
	case !s.Enabled:
		return notApplicable(sourceHooksOff), true
	case !s.Loaded:
		return Unavailable(), true
	case s.LoadFailed:
		return unknown(sourceHooksLoadRefused), true
	default:
		return Observation{}, false
	}
}

// loadState is what became of the directories: a manager, nothing, or a
// failure nobody may see the text of.
func (s HooksState) loadState() HooksLifecycle {
	switch {
	case s.LoadFailed:
		return HooksLoadFailed
	case s.Loaded:
		return HooksLoaded
	default:
		return HooksNotLoaded
	}
}

// liveState is what the hook layer amounts to right now, answered in the
// order the answers stop mattering: off beats everything, a load that failed
// beats what it would have found, a load that never happened beats a manager
// that could not exist, a missing manager beats what it would have held, and
// a held entry beats every reason there might not have been one.
func (s HooksState) liveState() HooksLifecycle {
	switch {
	case !s.Enabled:
		return HooksDisabled
	case s.LoadFailed:
		return HooksLoadFailed
	case !s.Loaded:
		return HooksNotLoaded
	case !s.Managed:
		return HooksNoManager
	case !s.any(registeredHook):
		return HooksEmpty
	default:
		return HooksActive
	}
}

// loadOutcome is what the last load came to, as a category. A load that
// finished having skipped something is neither clean nor failed, and the
// difference is the one a reader is looking for.
func (s HooksState) loadOutcome() HookLoad {
	switch {
	case !s.Loaded:
		return HookLoadNotAttempted
	case s.LoadFailed:
		return HookLoadFailed
	case s.Warnings > 0:
		return HookLoadWarned
	default:
		return HookLoadClean
	}
}

// directory names the hook directory one origin stands for. The path is the
// origin of the layer rather than its value, and the registry collapses the
// home directory before it reaches a reader.
func (s HooksState) directory(origin HookOrigin) Source {
	dir := s.UserDir
	if origin == HookOriginProject {
		dir = s.ProjectDir
	}
	if dir == "" {
		return Source{Kind: SourceConfigFile, Ref: "no " + string(origin) + " hook directory was consulted"}
	}
	return Source{Kind: SourceConfigFile, Ref: dir}
}

// discoveredHook reports whether the last load found a manifest for a hook.
func discoveredHook(h HookFacts) bool { return h.Discovered }

// registeredHook reports whether the manager in force holds a hook.
func registeredHook(h HookFacts) bool { return h.Registered }

// blockingHook reports whether a hook's own failure denies the call.
func blockingHook(h HookFacts) bool { return h.FailClosed }

// detachedHook reports whether a hook is fired and forgotten.
func detachedHook(h HookFacts) bool { return h.Async }

// any reports whether any hook matches keep.
func (s HooksState) any(keep func(HookFacts) bool) bool {
	return slices.ContainsFunc(s.Hooks, keep)
}

// names collects the names of the hooks matching keep, in the order the owner
// reported them and without repeating a name two events share.
func (s HooksState) names(keep func(HookFacts) bool) []string {
	out := make([]string, 0, len(s.Hooks))
	for _, hook := range s.Hooks {
		if !keep(hook) || hook.Name == "" || slices.Contains(out, hook.Name) {
			continue
		}
		out = append(out, hook.Name)
	}
	return out
}

// eventSet collects the events of the hooks matching keep, in the order this
// build fans them out rather than the order the hooks happen to be in, so two
// loads of the same directory answer the same way.
func (s HooksState) eventSet(keep func(HookFacts) bool) []string {
	out := make([]string, 0, len(s.Events))
	for _, event := range s.Events {
		for _, hook := range s.Hooks {
			if keep(hook) && hook.Event == event {
				out = append(out, event)
				break
			}
		}
	}
	return out
}

// toolSet collects the tool selectors of the tool-loop hooks matching keep,
// in the order the owner reported them and without repeating one two hooks
// share. A hook that is not on the tool loop carries no selector and adds
// nothing here.
func (s HooksState) toolSet(keep func(HookFacts) bool) []string {
	out := make([]string, 0)
	for _, hook := range s.Hooks {
		if !keep(hook) || hook.Tool == "" || slices.Contains(out, hook.Tool) {
			continue
		}
		out = append(out, hook.Tool)
	}
	return out
}

// longestTimeout is the largest budget any registered hook carries, and zero
// when the manager holds nothing — which is a layer nothing set rather than a
// budget of no time at all.
func (s HooksState) longestTimeout() time.Duration {
	var longest time.Duration
	for _, hook := range s.Hooks {
		if hook.Registered && hook.Timeout > longest {
			longest = hook.Timeout
		}
	}
	return longest
}

// winningHookOrigin is the source whose definition is in force. Discovery
// records sources in precedence order, so the last one recorded is the one
// that replaced the others.
func winningHookOrigin(origins []HookOrigin) HookOrigin {
	if len(origins) == 0 {
		return ""
	}
	return origins[len(origins)-1]
}

// callHooksState reads the optional accessor. A nil accessor is a wiring gap,
// and every layer it feeds reports unavailable rather than crashing the
// snapshot.
func callHooksState(accessor func() HooksState) HooksState {
	if accessor == nil {
		return HooksState{}
	}
	return accessor()
}
