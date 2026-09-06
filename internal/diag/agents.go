package diag

import (
	"context"
	"slices"
	"strconv"
)

// Agents field keys. The category answers one question — what this session
// can put to work beside itself, and what it has out right now — and every
// key is a stable address for Explain.
const (
	KeyAgentsState       = "agents.state"
	KeyAgentsRoles       = "agents.roles"
	KeyAgentsModels      = "agents.models"
	KeyAgentsDepth       = "agents.depth"
	KeyAgentsConcurrency = "agents.concurrency"
	KeyAgentsAssignments = "agents.assignments"
	KeyAgentsDeveloper   = "agents.developer"
)

// AgentModelInherited is what a role with no pin runs: the session's own
// model, chosen when the child is built. It is the word the spawn result
// already uses, so the two answers read the same.
const AgentModelInherited = "inherit"

// AgentsLifecycle is where the sub-agent layer stands. The values separate
// the things "nothing is running" could otherwise mean, which are fixed in
// entirely different places: switched off in the configuration, no manager
// in the process, a session that is itself a sub-agent and may not nest
// further, every slot in the process taken, and a session with nothing out.
type AgentsLifecycle string

// AgentsLifecycle values.
const (
	// AgentsEnabled means the configuration leaves sub-agents on. It is the
	// configured layer's answer and says nothing about whether this session
	// can actually start one.
	AgentsEnabled AgentsLifecycle = "enabled"
	// AgentsDisabled means the configuration switched sub-agents off, so the
	// session carries no spawn tool and nothing below applies.
	AgentsDisabled AgentsLifecycle = "disabled"
	// AgentsManaged means a job manager is in force for this process.
	AgentsManaged AgentsLifecycle = "managed"
	// AgentsNoManager means no job manager is in force. It is not the same
	// as a manager with nothing running: no spawn can be admitted at all.
	AgentsNoManager AgentsLifecycle = "no_manager"
	// AgentsDepthReached means this session is already as deep as the
	// nesting ceiling allows, so a spawn from here is refused however many
	// slots are free.
	AgentsDepthReached AgentsLifecycle = "depth_reached"
	// AgentsSaturated means every concurrency slot in the process is taken.
	// A spawn is refused rather than queued, which is the answer a session
	// with nothing of its own running is otherwise missing.
	AgentsSaturated AgentsLifecycle = "saturated"
	// AgentsRunning means this session has at least one assignment out.
	AgentsRunning AgentsLifecycle = "running"
	// AgentsIdle means a manager is in force with room to spawn and this
	// session has nothing out.
	AgentsIdle AgentsLifecycle = "idle"
)

// RolePin is one entry of the agents.models table: the role, the model
// reference the configuration pinned to it, and what that reference resolves
// to for this session. A pin whose name no longer resolves is not an error —
// the spawn falls back to the session model — and the pair of answers is
// what makes that visible.
type RolePin struct {
	// Role is the sub-agent role this pin addresses.
	Role string
	// Ref is the reference as configured — a model name, optionally with an
	// effort. Empty for a role the configuration leaves alone.
	Ref string
	// Model is the model name the reference resolves to against the catalog
	// this session can reach. Empty when it resolves to nothing.
	Model string
	// Resolved is whether the reference still names a model. A false here
	// with a non-empty Ref is a stale pin, which degrades to inheritance.
	Resolved bool
}

// AssignmentFacts is what the harness may say about one assignment this
// session has out. It is an allowlist by construction: a role and a
// lifecycle status, with no member for the prompt, the description, the
// summary, the result, the working directory, the child session id or the
// text of an error to land in.
type AssignmentFacts struct {
	// Role is the capability profile the child runs under.
	Role string
	// Status is where the job stands, in the manager's own vocabulary.
	Status string
}

// AgentsState is one observation of the sub-agent layer: whether the
// configuration leaves it on, whether a manager is in force, the roles and
// pins a spawn would resolve against, the ceilings it is admitted under, and
// the assignments this session has out right now.
//
// Only this session's assignments are described, and only the ones still in
// memory. A finished job is a file on disk, and reading disk is not
// something an observation does.
type AgentsState struct {
	// Known is false when nobody published an observation — the layer is not
	// wired, rather than wired and empty. Every field it feeds then reports
	// unavailable instead of a zero that would read as "sub-agents are off".
	Known bool
	// Enabled is whether the configuration leaves sub-agents on.
	Enabled bool
	// Managed is whether a job manager is in force. It is a separate answer
	// from Enabled: a configuration can leave sub-agents on in a process
	// that never built a manager, and nothing would be admitted there.
	Managed bool
	// Roles is the role vocabulary this build offers, in canonical order, as
	// the owner spells it, so the two cannot drift apart.
	Roles []string
	// Role is the role this session itself runs as. Empty for a session the
	// process opened rather than a spawn.
	Role string
	// Pins is one entry per role, whether or not the configuration pinned it.
	Pins []RolePin
	// Depth is how deep this session sits: zero for one the process opened,
	// one for a sub-agent.
	Depth int
	// MaxDepth is the nesting ceiling a spawn past which is refused.
	MaxDepth int
	// MaxConcurrent is how many assignments the process runs at once. There
	// is no queue behind it: past the ceiling a spawn is refused.
	MaxConcurrent int
	// LiveProcess is how many assignments the process has out for every
	// session together. It is an aggregate and carries nothing of what
	// another session is doing.
	LiveProcess int
	// LiveSession is how many of them are this session's, counting a spawn
	// still waiting on its slot.
	LiveSession int
	// Assignments is one entry per assignment of this session the manager
	// still holds in memory.
	Assignments []AssignmentFacts
	// Revision fingerprints the state this observation describes, so two
	// snapshots taken across a spawn are visibly of two different states.
	Revision string
}

// Sources for the layers whose origin is the view's own reading rather than
// an owner's record.
var (
	sourceAgentsConfig = Source{
		Kind: SourceConfigFile,
		Ref:  "agents.enabled",
	}
	sourceAgentsManager = Source{
		Kind: SourceComputed,
		Ref:  "whether this process built a job manager to admit spawns into",
	}
	sourceAgentsLive = Source{
		Kind: SourceSession,
		Ref:  "what this session has out right now; asking does not start, cancel or wait for one",
	}
	sourceAgentsOff = Source{
		Kind: SourceConfigFile,
		Ref:  "agents.enabled switched sub-agents off, so this session carries no spawn tool",
	}
	sourceAgentsUnmanaged = Source{
		Kind: SourceComputed,
		Ref:  "no job manager is in force in this process, so no spawn can be admitted",
	}
	sourceAgentsRoleVocabulary = Source{
		Kind: SourceBuild,
		Ref:  "the sub-agent roles this build offers; a spawn naming another one is refused",
	}
	sourceAgentsRolesSpawnable = Source{
		Kind: SourceComputed,
		Ref: "the roles a spawn from this session could name; empty when sub-agents are off or the " +
			"nesting ceiling leaves no room",
	}
	sourceAgentsRoleSelf = Source{
		Kind: SourceSession,
		Ref:  "the role this session runs under, fixed by the spawn that opened it",
	}
	sourceAgentsRoleRoot = Source{
		Kind: SourceSession,
		Ref:  "this session was opened by the process rather than by a spawn, so it runs under no role",
	}
	sourceAgentsPins = Source{
		Kind: SourceConfigFile,
		Ref:  "agents.models",
	}
	sourceAgentsPinsResolved = Source{
		Kind: SourceComputed,
		Ref: "the pins that still name a model this session can reach; one whose name no longer resolves " +
			"is absent here",
	}
	sourceAgentsPinsEffective = Source{
		Kind: SourceComputed,
		Ref: "what a spawn would run for each role: the pinned model where the pin resolves, and the " +
			"session's own model everywhere else, so a stale pin degrades to inheritance rather than " +
			"failing the spawn",
	}
	sourceAgentsDepthCeiling = Source{
		Kind: SourceDefault,
		Ref:  "how deep sub-agents may nest; a spawn at or past it is refused",
	}
	sourceAgentsDepthSelf = Source{
		Kind: SourceSession,
		Ref:  "how deep this session sits: zero for one the process opened, one for a sub-agent",
	}
	sourceAgentsDepthRoom = Source{
		Kind: SourceComputed,
		Ref: "whether the nesting ceiling still leaves room for a spawn from here; a sub-agent also " +
			"carries no spawn tool at all, so the ceiling is not the only thing that stops one",
	}
	sourceAgentsSlots = Source{
		Kind: SourceDefault,
		Ref:  "how many assignments the process runs at once; past it a spawn is refused rather than queued",
	}
	sourceAgentsSlotsUsed = Source{
		Kind: SourceComputed,
		Ref: "how many assignments the process has out for every session together — a count, carrying " +
			"nothing of what another session is doing",
	}
	sourceAgentsSlotsSession = Source{
		Kind: SourceSession,
		Ref:  "how many of them are this session's, counting a spawn still waiting on its slot",
	}
	sourceAgentsUnplanned = Source{
		Kind: SourceComputed,
		Ref:  "nothing configures what this session has out; an assignment exists because a spawn created one",
	}
	sourceAgentsByRole = Source{
		Kind: SourceSession,
		Ref: "this session's assignments by role, from the manager's own memory; a finished one is a file " +
			"on disk and this view reads no disk",
	}
	sourceAgentsByStatus = Source{
		Kind: SourceSession,
		Ref: "the same assignments by status. Roles and statuses only — no prompt, description, summary, " +
			"result, workdir, child session id or error text",
	}
	sourceAgentsDeveloperGrant = Source{
		Kind: SourceCLIFlag,
		Ref: "developer mode is granted on the command line at startup; nothing in the configuration and " +
			"no running session turns it on",
	}
	sourceAgentsDeveloperSelf = Source{
		Kind: SourceSession,
		Ref:  "this session carries the harness view, which is why this question can be asked at all",
	}
	sourceAgentsDeveloperChild = Source{
		Kind: SourceComputed,
		Ref: "a sub-agent spawned from here is built without the harness view: developer access belongs to " +
			"the session the process opened and is not a capability a spawn passes on",
	}
)

// lifecycle separates the things "nothing is running" could mean, answered
// in the order the answers stop mattering: switched off beats everything, a
// missing manager beats what it would have admitted, a session that may not
// nest beats a free slot it could never use, a full process beats an idle
// session that is about to be refused, and something out beats every reason
// there might not have been anything.
func (s AgentsState) lifecycle() Field {
	field := s.field(KeyAgentsState, ApplyNewSession, ScopeSession)
	if !s.Known {
		return field
	}
	setting := AgentsEnabled
	if !s.Enabled {
		setting = AgentsDisabled
	}
	managed := AgentsManaged
	if !s.Managed {
		managed = AgentsNoManager
	}
	field.Configured = Present(StringValue(string(setting)), sourceAgentsConfig)
	field.Loaded = Present(StringValue(string(managed)), sourceAgentsManager)
	field.Effective = Present(StringValue(string(s.liveState())), sourceAgentsLive)
	return field
}

// roles is the vocabulary a spawn chooses from, what this session could
// still name, and what this session is itself. The last one is the answer a
// sub-agent needs most: it explains a narrowed tool set and a narrowed
// permission ceiling that nothing in the configuration accounts for.
func (s AgentsState) roles() Field {
	field := s.field(KeyAgentsRoles, ApplyNewSession, ScopeSession)
	if !s.Known {
		return field
	}
	field.Configured = Present(ListValue(s.Roles), sourceAgentsRoleVocabulary)
	field.Loaded = Present(ListValue(s.spawnableRoles()), sourceAgentsRolesSpawnable)
	if s.Role == "" {
		field.Effective = Unset(StringValue(""), sourceAgentsRoleRoot)
		return field
	}
	field.Effective = Present(StringValue(s.Role), sourceAgentsRoleSelf)
	return field
}

// models is the role pin table read three times over: what the configuration
// asked for, which of those references still name a model, and what a spawn
// would actually run for every role. A stale pin is present in the first
// list, missing from the second and shown as inheritance in the third, which
// is the whole of what "the model I pinned is not the one it ran" means.
func (s AgentsState) models() Field {
	field := s.field(KeyAgentsModels, ApplyNextTurn, ScopeWorkspace)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	configured := make([]string, 0, len(s.Pins))
	resolved := make([]string, 0, len(s.Pins))
	effective := make([]string, 0, len(s.Pins))
	for _, pin := range s.Pins {
		if pin.Ref != "" {
			configured = append(configured, pin.Role+"="+pin.Ref)
		}
		if pin.Resolved {
			resolved = append(resolved, pin.Role+"="+pin.Model)
			effective = append(effective, pin.Role+"="+pin.Model)
			continue
		}
		effective = append(effective, pin.Role+"="+AgentModelInherited)
	}
	field.Configured = Present(ListValue(configured), sourceAgentsPins)
	field.Loaded = Present(ListValue(resolved), sourceAgentsPinsResolved)
	field.Effective = Present(ListValue(effective), sourceAgentsPinsEffective)
	return field
}

// depth is how far sub-agents may nest, how far this session already is, and
// whether that leaves room. It answers even when sub-agents are off, because
// the ceiling is a property of the process rather than of the configuration
// that switched the tool away.
func (s AgentsState) depth() Field {
	field := s.field(KeyAgentsDepth, ApplyRestart, ScopeProcess)
	if !s.Known {
		return field
	}
	field.Configured = Present(IntValue(int64(s.MaxDepth)), sourceAgentsDepthCeiling)
	field.Loaded = Present(IntValue(int64(s.Depth)), sourceAgentsDepthSelf)
	field.Effective = Present(BoolValue(s.Depth < s.MaxDepth), sourceAgentsDepthRoom)
	return field
}

// concurrency is how many assignments may run at once, how many the process
// has out, and how many of them are this session's. The middle answer is an
// aggregate on purpose: a spawn is refused because the process is full, and
// a session that can only see its own would have no way to tell that from a
// bug.
func (s AgentsState) concurrency() Field {
	field := s.field(KeyAgentsConcurrency, ApplyRestart, ScopeProcess)
	if !s.Known {
		return field
	}
	field.Configured = Present(IntValue(int64(s.MaxConcurrent)), sourceAgentsSlots)
	field.Loaded = Present(IntValue(int64(s.LiveProcess)), sourceAgentsSlotsUsed)
	field.Effective = Present(IntValue(int64(s.LiveSession)), sourceAgentsSlotsSession)
	return field
}

// assignments is what this session has out, by role and by status. Both are
// counts of the manager's own memory: a finished assignment is deleted from
// it and lives on disk, and an observation does not read disk — so this is
// what is running, not a history of what ran.
func (s AgentsState) assignments() Field {
	field := s.field(KeyAgentsAssignments, ApplyImmediate, ScopeSession)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = notApplicable(sourceAgentsUnplanned)
	field.Loaded = Present(ListValue(s.countBy(s.Roles, func(a AssignmentFacts) string {
		return a.Role
	})), sourceAgentsByRole)
	field.Effective = Present(ListValue(s.countBy(nil, func(a AssignmentFacts) string {
		return a.Status
	})), sourceAgentsByStatus)
	return field
}

// developer states what a spawn does not carry. The harness view is a
// capability of the session the process opened: a child is built without a
// registry, so a sub-agent cannot ask any of these questions however it was
// spawned, and a parent that can is not evidence that its children may.
func (s AgentsState) developer() Field {
	field := s.field(KeyAgentsDeveloper, ApplyRestart, ScopeProcess)
	if !s.Known {
		return field
	}
	field.Configured = notApplicable(sourceAgentsDeveloperGrant)
	field.Loaded = Present(BoolValue(true), sourceAgentsDeveloperSelf)
	field.Effective = Present(BoolValue(false), sourceAgentsDeveloperChild)
	return field
}

// field is the shape every agents field starts from: all three layers
// unavailable, so a layer nobody wired degrades into an honest answer rather
// than into a zero that would read as "sub-agents are off". Apply and scope
// are per field — a ceiling fixed when the process started and a pin the
// next spawn reads are not changed by the same act.
func (s AgentsState) field(key string, apply Apply, scope Scope) Field {
	return Field{
		Key:        key,
		Configured: Unavailable(),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      apply,
		Scope:      scope,
		Revision:   s.Revision,
	}
}

// absent reports whether a question about what this session can put to work
// can be answered at all, and what to say when it cannot. The cases are not
// the same: a layer nobody wired knows nothing, a configuration that
// switched sub-agents off left this session without the tool, and a process
// with no manager could not admit a spawn from any session at all.
func (s AgentsState) absent() (Observation, bool) {
	switch {
	case !s.Known:
		return Unavailable(), true
	case !s.Enabled:
		return notApplicable(sourceAgentsOff), true
	case !s.Managed:
		return notApplicable(sourceAgentsUnmanaged), true
	default:
		return Observation{}, false
	}
}

// liveState is what the sub-agent layer amounts to right now. Saturation
// beats running on purpose: a session with assignments out already sees them
// in agents.assignments, and the answer it is missing is why the next spawn
// was refused.
func (s AgentsState) liveState() AgentsLifecycle {
	switch {
	case !s.Enabled:
		return AgentsDisabled
	case !s.Managed:
		return AgentsNoManager
	case s.Depth >= s.MaxDepth:
		return AgentsDepthReached
	case s.MaxConcurrent > 0 && s.LiveProcess >= s.MaxConcurrent:
		return AgentsSaturated
	case s.LiveSession > 0:
		return AgentsRunning
	default:
		return AgentsIdle
	}
}

// spawnableRoles is what a spawn from this session could still name. It is
// the whole vocabulary or none of it: a role is refused by the ceiling and
// the configuration, never one role at a time.
func (s AgentsState) spawnableRoles() []string {
	if !s.Enabled || !s.Managed || s.Depth >= s.MaxDepth {
		return []string{}
	}
	return s.Roles
}

// countBy tallies this session's assignments by one of their two safe
// members and renders the tally as "name=count". Order is the owner's
// canonical one where there is one, and first-seen otherwise, so two
// snapshots of one state answer the same way whatever order a map handed
// them back in.
func (s AgentsState) countBy(canonical []string, of func(AssignmentFacts) string) []string {
	order := make([]string, 0, len(canonical))
	counts := make(map[string]int, len(s.Assignments))
	for _, assignment := range s.Assignments {
		name := of(assignment)
		if name == "" {
			continue
		}
		if counts[name] == 0 && !slices.Contains(canonical, name) {
			order = append(order, name)
		}
		counts[name]++
	}
	slices.Sort(order)
	out := make([]string, 0, len(counts))
	for _, name := range append(slices.Clone(canonical), order...) {
		if counts[name] > 0 {
			out = append(out, name+"="+strconv.Itoa(counts[name]))
		}
	}
	return out
}

// agentsReason states what this category answers and what it deliberately
// leaves out.
const agentsReason = "what this session can put to work beside itself: whether sub-agents are on, " +
	"which roles a spawn may name and which one this session is itself, which model each role is " +
	"pinned to and which pins still resolve, the nesting and concurrency ceilings a spawn is admitted " +
	"under, and how many assignments this session has out by role and by status. " +
	"Roles, model names, counts and states only — no prompt, description, summary, result, transcript, " +
	"workdir, job id, child session id or error text, and nothing of what another session has out " +
	"beyond a process-wide count. Nothing here spawns, cancels, recovers or waits for an assignment, " +
	"and only what the manager holds in memory is described: a finished job is a file on disk, and " +
	"this view reads no disk"

// AgentDeps binds the agents collector to the owner of this session's
// sub-agent layer. It is optional: a process without one reports the
// category unavailable rather than making it fail.
type AgentDeps struct {
	// State observes the job manager and this session's binding to it
	// through the owner's own projection. It is read, never exercised: no
	// accessor here may spawn a job, cancel one, recover one, wait for one
	// or read a job's directory.
	State func() AgentsState
}

// agentCollector observes what this session can delegate to.
type agentCollector struct {
	deps AgentDeps
}

// NewAgentCollector builds the agents collector.
func NewAgentCollector(deps AgentDeps) Collector {
	return &agentCollector{deps: deps}
}

func (*agentCollector) Category() Category { return CategoryAgents }

// Status is answered from the declared key set alone: listing the catalog
// reaches no manager and reads no job. The keys are static for the same
// reason — a key per assignment would make the catalog depend on what is
// running, and would address a job by an id this view does not report.
func (*agentCollector) Status() Status {
	return Status{
		Availability: AvailabilityAvailable,
		Reason:       agentsReason,
		Keys: []string{
			KeyAgentsState,
			KeyAgentsRoles,
			KeyAgentsModels,
			KeyAgentsDepth,
			KeyAgentsConcurrency,
			KeyAgentsAssignments,
			KeyAgentsDeveloper,
		},
	}
}

// Collect reads the owner once and derives every field from that one read,
// so the fields of one answer describe one moment rather than several.
func (c *agentCollector) Collect(_ context.Context) ([]Field, error) {
	agents := callAgentsState(c.deps.State)
	return []Field{
		agents.lifecycle(),
		agents.roles(),
		agents.models(),
		agents.depth(),
		agents.concurrency(),
		agents.assignments(),
		agents.developer(),
	}, nil
}

// callAgentsState reads the optional accessor. A nil accessor is a wiring
// gap, and every layer it feeds reports unavailable rather than crashing the
// snapshot.
func callAgentsState(accessor func() AgentsState) AgentsState {
	if accessor == nil {
		return AgentsState{}
	}
	return accessor()
}
