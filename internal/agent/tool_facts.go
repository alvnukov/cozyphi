package agent

import (
	"hash/fnv"
	"slices"
	"strconv"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tasks"
)

// Where each tool comes from, and what is missing when it is not there.
//
// These are the reasons the harness gives a user for a tool they expected and
// cannot see, so they are written by the owner that knows: this engine holds
// every manager, and the absence of one is the whole explanation. They name
// what supplies a tool, never what the tool does and never anything it was
// configured with — no server address, no directory, no command.
var (
	toolFromBuiltin = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "the built-in tool set this session was assembled with",
	}
	toolNoBuiltin = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "this session's own tool set does not carry it; a read-only role assembles a narrower one",
	}
	toolNarrowedByMode = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "plan mode narrows the built-in set to reading; the tool returns when the posture does",
	}
	toolFromSessionNaming = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "the primary session names itself",
	}
	toolNoSessionNaming = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "only the primary session names itself; a sub-agent has no title to set",
	}
	toolFromPlan = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "the durable plan this session runs under",
	}
	toolNoPlan = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "the durable plan belongs to the primary session; a sub-agent carries one job, not a plan",
	}
	toolFromQuestion = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "the approval channel this session can reach",
	}
	toolNoQuestion = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "no approval channel: this run has nobody to put a question to",
	}
	toolFromEngine = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "every engine reports and compacts its own context",
	}
	toolFromMCP = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "the MCP pool attached to this session",
	}
	toolNoMCP = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "no MCP pool is attached to this session",
	}
	toolFromLSP = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "the language-server query attached to this session",
	}
	toolNoLSP = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "no language server is attached to this session",
	}
	toolFromMemory = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "the memory store attached to this session",
	}
	toolNoMemory = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "no memory store is attached to this session",
	}
	toolFromWatches = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "the watch manager attached to this session",
	}
	toolNoWatches = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "no watch manager is attached to this session",
	}
	toolFromTasks = diag.Source{
		Kind: diag.SourceConfigFile,
		Ref:  "the task registry, at the level permissions.tasks sets",
	}
	toolNoTaskRegistry = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "no task registry was discovered for this workspace",
	}
	toolTasksOff = diag.Source{
		Kind: diag.SourceConfigFile,
		Ref:  "permissions.tasks is off, which leaves the tool out rather than denying its calls",
	}
	toolFromDeveloperMode = diag.Source{
		Kind: diag.SourceCLIFlag,
		Ref:  "--developer-mode granted this session the read-only view of the harness",
	}
	toolNoDeveloperMode = diag.Source{
		Kind: diag.SourceCLIFlag,
		Ref:  "the process was not started with --developer-mode; nothing in it can turn the view on",
	}
	toolFromJobs = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "the job manager attached to this session",
	}
	toolNoJobs = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "no job manager is attached: this session spawns no sub-agents",
	}
	toolNarrowedAfterAssembly = diag.Source{
		Kind: diag.SourceSession,
		Ref:  "the live registry does not hold it; the session's posture narrowed the set after assembly",
	}
	toolNotInCatalog = diag.Source{
		Kind: diag.SourceUnknown,
		Ref:  "supplied to this engine by its caller; this view does not know what provides it",
	}
)

// ToolObservation reads this engine's tool layer for the diagnostics
// registry: what the session's ordinary posture carries, what the live
// registry holds now, what the plan gate permits now, and — for each tool —
// what the permission boundary would have to read to decide a call.
//
// It observes and nothing else. No tool is constructed, dispatched or handed
// to anybody: the registered set is read from the registry the executor is
// already bound to, so there is no cached list here to go stale and no tool
// value created to be reachable afterwards. No permission Request is built,
// no path resolved, no approval asked for, and no plan step started, moved or
// settled — the plan is read exactly as the provider view reads it.
//
// One read lock covers the whole read, so the layers of a single observation
// cannot disagree with each other.
//
// A nil engine is the "no engine yet" case, not an error: the answer says
// unknown and every layer fed from it reports unavailable.
func (engine *Engine) ToolObservation() diag.ToolState {
	if engine == nil {
		return diag.ToolState{}
	}
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	if engine.executor == nil {
		return diag.ToolState{}
	}

	registered := make(map[string]struct{}, len(engine.executor.registry))
	for name := range engine.executor.registry {
		registered[name] = struct{}{}
	}
	admitted := engine.admittedToolsLocked()
	exempt, callable := engine.planReachLocked()

	state := diag.ToolState{
		Known:      true,
		Mode:       string(normalizeMode(engine.mode)),
		PlanGate:   engine.planPhaseLocked(),
		Admitted:   toolOrder(admitted),
		Registered: toolOrder(registered),
		Tools:      make([]diag.ToolFacts, 0, len(diag.ToolCatalog())),
	}
	for name := range registered {
		if callable != nil {
			if _, ok := callable[name]; !ok {
				continue
			}
		}
		state.Callable = append(state.Callable, name)
	}
	state.Callable = toolOrder(setOf(state.Callable))

	for _, name := range diag.ToolCatalog() {
		supplied, missing := engine.toolOwnerLocked(name)
		_, isAdmitted := admitted[name]
		_, isRegistered := registered[name]
		_, isExempt := exempt[name]
		facts := diag.ToolFacts{
			Name:           name,
			Admitted:       isAdmitted,
			AdmittedFrom:   pickSource(isAdmitted, supplied, missing),
			Registered:     isRegistered,
			RegisteredFrom: registeredSource(isAdmitted, isRegistered, supplied, missing),
			Gated:          engine.planEnabled && !isExempt,
			Callable:       isRegistered && slices.Contains(state.Callable, name),
			Check:          permission.ToolCheckKind(name),
		}
		state.Tools = append(state.Tools, facts)
	}
	state.Revision = toolRevision(state)
	return state
}

// admittedToolsLocked is the tool set this session's ordinary posture
// carries: the base set it was assembled with, plus one name for each owner
// attached to the engine. It is derived from the engine's own fields rather
// than by building a tool list, so observing costs no tool value and hands
// nothing out — and the contract test in this package pins it against the
// list the real builder produces.
func (engine *Engine) admittedToolsLocked() map[string]struct{} {
	base := engine.baseTools
	if base == nil {
		base = engine.defaultTools
	}
	admitted := make(map[string]struct{}, len(base)+len(diag.ToolCatalog()))
	for i := range base {
		admitted[base[i].Definition.Name] = struct{}{}
	}
	for _, name := range diag.ToolCatalog() {
		if engine.ownerAttachedLocked(name) {
			admitted[name] = struct{}{}
		}
	}
	return admitted
}

// ownerAttachedLocked reports whether the owner that contributes one tool is
// attached to this engine. The built-in file and shell tools have no owner
// beyond the base set, so they answer false here and are admitted by that set
// alone.
func (engine *Engine) ownerAttachedLocked(name string) bool {
	switch name {
	case "session":
		return engine.sessionNaming
	case "plan":
		return engine.planEnabled
	case "question":
		return engine.questionAsk != nil
	case "context":
		return true
	case "mcp_list", "mcp_inspect", "mcp_call":
		return engine.mcp != nil
	case "lsp":
		return engine.lsp != nil
	case "memory":
		return engine.memory != nil
	case "watch":
		return engine.watches != nil
	case "task":
		return engine.taskAccess() != tasks.AccessOff
	case "harness":
		return engine.diagnostics != nil
	case "agent_spawn", "agent_list", "agent_wait", "agent_cancel":
		return engine.jobs != nil
	default:
		return false
	}
}

// toolOwnerLocked names what supplies one tool and what is missing when it is
// absent. Both refs are returned whatever the state, so the caller attributes
// a layer without having to know which of the two applies.
func (engine *Engine) toolOwnerLocked(name string) (supplied, missing diag.Source) {
	switch name {
	case "bash", "read", "write", "edit", "grep", "ls", "find":
		return toolFromBuiltin, toolNoBuiltin
	case "session":
		return toolFromSessionNaming, toolNoSessionNaming
	case "plan":
		return toolFromPlan, toolNoPlan
	case "question":
		return toolFromQuestion, toolNoQuestion
	case "context":
		return toolFromEngine, toolFromEngine
	case "mcp_list", "mcp_inspect", "mcp_call":
		return toolFromMCP, toolNoMCP
	case "lsp":
		return toolFromLSP, toolNoLSP
	case "memory":
		return toolFromMemory, toolNoMemory
	case "watch":
		return toolFromWatches, toolNoWatches
	case "task":
		if engine.tasks == nil {
			return toolFromTasks, toolNoTaskRegistry
		}
		return toolFromTasks, toolTasksOff
	case "harness":
		return toolFromDeveloperMode, toolNoDeveloperMode
	case "agent_spawn", "agent_list", "agent_wait", "agent_cancel":
		return toolFromJobs, toolNoJobs
	default:
		return toolNotInCatalog, toolNotInCatalog
	}
}

// planReachLocked reads the plan gate the way the provider view reads it: the
// policy's exempt set, and — only while the gate denies — the tools the
// current plan admits. A nil callable set means the gate is not narrowing
// anything, so everything registered is reachable.
//
// The plan is read, never moved: no step is started, completed or approved by
// looking at it.
func (engine *Engine) planReachLocked() (exempt, callable map[string]struct{}) {
	if !engine.planEnabled || engine.planRuntime == nil {
		return nil, nil
	}
	policy := engine.planRuntime.Current()
	exempt = policy.VisibleTools(session.Plan{})
	if engine.planGate == nil || engine.planGate.Phase != plangate.PhaseDeny {
		return exempt, nil
	}
	var plan session.Plan
	if engine.session != nil {
		plan = engine.session.Plan()
	}
	return exempt, policy.VisibleTools(plan)
}

// planPhaseLocked names the phase the plan gate stands in, or nothing at all
// where there is no plan gate to stand in one.
func (engine *Engine) planPhaseLocked() string {
	if !engine.planEnabled || engine.planGate == nil {
		return ""
	}
	return string(engine.planGate.Phase)
}

// pickSource selects the owner's account of one layer.
func pickSource(present bool, supplied, missing diag.Source) diag.Source {
	if present {
		return supplied
	}
	return missing
}

// registeredSource attributes the loaded layer. A tool the session never
// admitted is absent for the owner's reason; one it admitted and the registry
// does not hold was narrowed after assembly, and plan mode is the narrowing
// that does it to the built-ins.
func registeredSource(admitted, registered bool, supplied, missing diag.Source) diag.Source {
	switch {
	case registered:
		return supplied
	case !admitted:
		return missing
	case missing == toolNoBuiltin:
		return toolNarrowedByMode
	default:
		return toolNarrowedAfterAssembly
	}
}

// toolOrder lists a tool set in catalog order, with any name the catalog does
// not know sorted after it. Two observations of the same set are then
// byte-comparable, and a caller-supplied tool is listed rather than hidden.
func toolOrder(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for _, name := range diag.ToolCatalog() {
		if _, ok := set[name]; ok {
			out = append(out, name)
		}
	}
	rest := make([]string, 0, len(set))
	for name := range set {
		if !slices.Contains(diag.ToolCatalog(), name) {
			rest = append(rest, name)
		}
	}
	slices.Sort(rest)
	return append(out, rest...)
}

func setOf(names []string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
}

// toolRevision fingerprints the tool list an observation describes: the
// posture, the plan-gate phase and the registered and callable sets. Two
// snapshots taken across a rebind, a model swap or a step transition carry
// two different revisions, which is how a reader tells a fresh answer from a
// repeat of an old one. It digests tool names only — never an argument, a
// schema or anything a tool was configured with.
func toolRevision(state diag.ToolState) string {
	digest := fnv.New64a()
	_, _ = digest.Write([]byte(state.Mode))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(state.PlanGate))
	for _, set := range [][]string{state.Registered, state.Callable} {
		_, _ = digest.Write([]byte{0})
		for _, name := range set {
			_, _ = digest.Write([]byte(name))
			_, _ = digest.Write([]byte{0x1f})
		}
	}
	return strconv.FormatUint(digest.Sum64(), 16)
}
