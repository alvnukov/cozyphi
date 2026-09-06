package diag

import "context"

// Aggregate tool field keys. They are declared statically so the catalog can
// list them and Explain can validate a key without observing anything.
const (
	KeyToolsRegistered = "registered"
	KeyToolsMode       = "mode"
	KeyToolsPlanGate   = "plan_gate"
)

// toolKeyPrefix namespaces the per-tool fields. Without it a tool named
// "mode" would collide with the aggregate above, and a field key would stop
// being a stable address for Explain.
const toolKeyPrefix = "tool."

// toolCatalog is the native tool vocabulary this view knows how to explain,
// in assembly order: the built-in file and shell tools, the session's own
// utilities, then each tool an attached owner contributes.
//
// It is a fixed list of names and nothing else. No MCP server is asked what
// it offers and no server tool schema is imported to build it, so the size
// of this answer does not grow with what a user has configured — mcp_call is
// one entry here however many tools a server behind it exposes.
var toolCatalog = []string{
	"bash",
	"read",
	"write",
	"edit",
	"grep",
	"ls",
	"find",
	"session",
	"plan",
	"question",
	"context",
	"mcp_list",
	"mcp_inspect",
	"mcp_call",
	"lsp",
	"memory",
	"watch",
	"task",
	"harness",
	"agent_spawn",
	"agent_list",
	"agent_wait",
	"agent_cancel",
}

// ToolCatalog returns the native tool vocabulary in catalog order. The slice
// is detached: an owner may range over it and answer for each name without
// keeping its own second list, which is what stops the two from drifting.
func ToolCatalog() []string {
	out := make([]string, len(toolCatalog))
	copy(out, toolCatalog)
	return out
}

// ToolKey is the field key one tool is addressed by.
func ToolKey(name string) string { return toolKeyPrefix + name }

// Tool field states. They are the whole vocabulary of a tool field's value,
// and they are deliberately four rather than two: "not there", "there but
// nothing may call it now" and "there, and whether one call is allowed
// depends on that call" are three different answers, and collapsing them
// into a boolean is how a view starts promising a call will succeed.
const (
	// ToolRegistered means the tool is there and nothing is narrowing it.
	ToolRegistered = "registered"
	// ToolUnavailable means the tool is not there; the source says why.
	ToolUnavailable = "unavailable"
	// ToolRestricted means it is registered, but something narrows whether
	// the model may call it right now — the plan gate, today.
	ToolRestricted = "restricted"
	// ToolRequiresArgumentCheck means it is registered and reachable, and the
	// permission boundary decides one call from that call's own arguments.
	// No answer here holds for arguments nobody has written yet.
	ToolRequiresArgumentCheck = "requires_argument_check"
)

// toolKeys is the declared key set, in the order Collect returns them.
var toolKeys = func() []string {
	keys := make([]string, 0, 3+len(toolCatalog))
	keys = append(keys, KeyToolsRegistered, KeyToolsMode, KeyToolsPlanGate)
	for _, name := range toolCatalog {
		keys = append(keys, ToolKey(name))
	}
	return keys
}()

// toolsReason states what this category answers and what it deliberately
// leaves out.
const toolsReason = "which tools this session carries, why a known one is missing, and what " +
	"stands between the model and calling it; registration and reachability only — no tool " +
	"schema, no arguments and no result of any call, and no MCP server is asked what it offers. " +
	"Nothing here calls a tool, preflights someone else's call, asks for an approval or moves a " +
	"plan step: the tool list is read, and reading it grants nothing"

// ToolCheck says what the permission boundary reads in order to decide one
// tool's call. It is a statement about the shape of the decision, never about
// its outcome: this view holds no evaluator and runs none.
type ToolCheck string

// ToolCheck values.
const (
	// CheckToolName means the boundary decides from the tool alone. Every
	// call gets the same answer, so naming it here says something true.
	CheckToolName ToolCheck = "tool_name"
	// CheckArguments means this call's own arguments decide — a path, a shell
	// command, a server and tool target, or the registry action asked for.
	// A tool in this class is never reported as allowed: the answer belongs
	// to a call that has not been written yet.
	CheckArguments ToolCheck = "arguments"
	// CheckPolicy means a permission setting decides, the same way for every
	// call, and the arguments do not enter.
	CheckPolicy ToolCheck = "policy"
	// CheckUnknown is a tool the permission owner does not classify.
	CheckUnknown ToolCheck = "unknown"
)

// ToolFacts is what the harness may say about one tool. It is an allowlist by
// construction: a name, four booleans and the provenance of each, with no
// member for a schema, a description, an argument or a result to land in.
type ToolFacts struct {
	// Name is the tool's registered name.
	Name string
	// Admitted is whether the session's own tool set carries this tool in its
	// ordinary posture — before plan mode narrows anything. It is what the
	// session was assembled with: a sub-agent's role ceiling and the owners
	// wired at construction both decide it.
	Admitted bool
	// AdmittedFrom is the owner's own account of that answer: what supplies
	// the tool, or what is missing. Written by the owner, because only the
	// owner knows.
	AdmittedFrom Source
	// Registered is whether the live tool registry holds it right now. It is
	// read from the registry the executor is bound to, never from a list
	// remembered when the harness view was built.
	Registered     bool
	RegisteredFrom Source
	// Gated is whether the plan gate judges this tool: a gated tool carries
	// plan_step and needs a step to admit it, an exempt one never does.
	Gated bool
	// Callable is whether the plan gate permits calling it as things stand.
	// It is read from the gate's own projection of the current plan, so a
	// step that started or finished between two observations moves it.
	Callable bool
	// Check says what the permission boundary would read to decide a call.
	Check ToolCheck
}

// ToolState is one observation of the tool layer: the posture the tools were
// assembled under, the three sets that differ from each other, and one entry
// per tool the catalog knows.
type ToolState struct {
	// Known is false when no engine has published a tool registry yet. Every
	// layer fed from it then reports unavailable rather than an empty list
	// that would read as "this session has no tools".
	Known bool
	// Mode is the engine's posture: the plan-authoring one narrows the
	// built-in set to reading, the ordinary one carries all of it.
	Mode string
	// PlanGate is the phase the plan gate stands in, or empty where there is
	// no plan gate at all — a sub-agent runs one job and has no plan.
	PlanGate string
	// Admitted, Registered and Callable are the three sets a tool field
	// reports across its layers: what the session's ordinary posture carries,
	// what the live registry holds now, and what the plan gate permits now.
	// They include every registered name, catalog member or not, so a tool
	// this view cannot explain is still listed rather than hidden.
	Admitted   []string
	Registered []string
	Callable   []string
	// Tools is one entry per catalog name, in catalog order.
	Tools []ToolFacts
	// Revision fingerprints the tool list this observation describes, so two
	// snapshots taken across a rebind are visibly of two different lists.
	Revision string
}

// Sources for the layers whose origin is the view's own assembly rather than
// an owner's. The owner writes the two absence refs; these say what the
// remaining layers mean.
var (
	sourceToolPlanGate = Source{
		Kind: SourcePlan,
		Ref:  "the plan gate: no approved step in progress admits this tool yet",
	}
	sourceToolArguments = Source{
		Kind: SourceSession,
		Ref: "the permission boundary reads this call's own arguments — a path, a command, " +
			"a server target or the registry action — so no answer holds for every call",
	}
	sourceToolPolicy = Source{
		Kind: SourceSession,
		Ref:  "a permission setting decides this tool, the same way for every call",
	}
	sourceToolName = Source{
		Kind: SourceSession,
		Ref:  "the permission boundary decides from the tool alone; its arguments do not enter",
	}
	sourceToolUnclassified = Source{
		Kind: SourceUnknown,
		Ref:  "the permission owner does not classify this tool; what a call meets is not stated here",
	}
	sourceToolAdmitted = Source{
		Kind: SourceSession,
		Ref:  "the tool set this session was assembled with",
	}
	sourceToolRegistry = Source{
		Kind: SourceSession,
		Ref:  "the tool registry the executor is bound to right now",
	}
	sourceToolCallable = Source{
		Kind: SourceSession,
		Ref:  "what the plan gate permits the model to call as things stand",
	}
	sourceToolMode = Source{
		Kind: SourceSession,
		Ref:  "the engine's posture; the plan-authoring one narrows the built-in set to reading",
	}
	sourceToolPhase = Source{
		Kind: SourceSession,
		Ref:  "the phase the plan gate stands in; deny is where an unadmitted tool stops being callable",
	}
)

// ToolDeps binds the tool collector to its one owner. The engine holds the
// registry, the posture and the plan gate together, and it is asked for all
// three at once so the fields of one snapshot describe one tool list rather
// than several taken a moment apart.
type ToolDeps struct {
	// State observes the engine's tool layer through the owner's own
	// projection. It is read, never exercised: no accessor here may dispatch
	// a tool, build a permission request, ask for an approval or move a plan
	// step.
	State func() ToolState
}

// toolCollector observes what this session can call and what stops it.
type toolCollector struct {
	deps ToolDeps
}

// NewToolCollector builds the tool collector.
func NewToolCollector(deps ToolDeps) Collector {
	return &toolCollector{deps: deps}
}

func (*toolCollector) Category() Category { return CategoryTools }

// Status is answered from the declared key set alone: listing the catalog
// reads no registry and constructs no tool.
func (*toolCollector) Status() Status {
	keys := make([]string, len(toolKeys))
	copy(keys, toolKeys)
	return Status{Availability: AvailabilityAvailable, Reason: toolsReason, Keys: keys}
}

// Collect reads the owner once and derives every field from that one read.
func (c *toolCollector) Collect(_ context.Context) ([]Field, error) {
	state := callToolState(c.deps.State)
	fields := make([]Field, 0, len(toolKeys))
	fields = append(fields, state.sets(), state.mode(), state.phase())
	byName := make(map[string]ToolFacts, len(state.Tools))
	for _, facts := range state.Tools {
		byName[facts.Name] = facts
	}
	for _, name := range toolCatalog {
		fields = append(fields, state.tool(name, byName[name]))
	}
	return fields, nil
}

// sets publishes the three tool sets side by side. It is the direct answer to
// "what does this session actually carry": the ordinary posture's set, the
// live registry, and what may be called as things stand. Every registered
// name is here, including one the catalog has no field for.
func (s ToolState) sets() Field {
	field := Field{
		Key:        KeyToolsRegistered,
		Configured: Unavailable(),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      ApplyNextTurn,
		Scope:      ScopeSession,
		Revision:   s.Revision,
	}
	if !s.Known {
		return field
	}
	field.Configured = Present(ListValue(s.Admitted), sourceToolAdmitted)
	field.Loaded = Present(ListValue(s.Registered), sourceToolRegistry)
	field.Effective = Present(ListValue(s.Callable), sourceToolCallable)
	return field
}

// mode names the posture the built-in set was assembled under. Nothing in a
// config file sets it — the user enters and leaves plan mode in the session —
// so the configured layer does not exist rather than reporting a default.
func (s ToolState) mode() Field {
	observation := Unavailable()
	if s.Known && s.Mode != "" {
		observation = Present(StringValue(s.Mode), sourceToolMode)
	}
	return Field{
		Key:        KeyToolsMode,
		Configured: NotApplicable(SourceSession),
		Loaded:     observation,
		Effective:  observation,
		Apply:      ApplyNextTurn,
		Scope:      ScopeSession,
		Revision:   s.Revision,
	}
}

// phase names the plan gate the tool fields are read against. A session
// without one — a sub-agent carries a job, not a plan — reports unset rather
// than a phase it is not in.
func (s ToolState) phase() Field {
	observation := Unavailable()
	if s.Known {
		observation = Unset(StringValue(""), sourceToolPhase)
		if s.PlanGate != "" {
			observation = Present(StringValue(s.PlanGate), sourceToolPhase)
		}
	}
	return Field{
		Key:        KeyToolsPlanGate,
		Configured: NotApplicable(SourceSession),
		Loaded:     observation,
		Effective:  observation,
		Apply:      ApplyNextTurn,
		Scope:      ScopeSession,
		Revision:   s.Revision,
	}
}

// tool builds one tool's field across the three layers the epic
// distinguishes. They answer three different questions on purpose, and the
// ticket asks for exactly that separation: configured is whether this
// session's shape admits the tool at all, loaded is whether the live registry
// holds it, and effective is what a call would meet right now — the plan
// gate first, and then whether the permission boundary can answer from the
// tool's name or has to read the call.
func (s ToolState) tool(name string, facts ToolFacts) Field {
	field := Field{
		Key:        ToolKey(name),
		Configured: Unavailable(),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      ApplyNextTurn,
		Scope:      ScopeSession,
		Revision:   s.Revision,
	}
	if !s.Known || facts.Name == "" {
		return field
	}
	if facts.Gated {
		// A gated tool's reach turns on the step in progress, so the answer
		// is only good for as long as that step is.
		field.Scope = ScopeStep
	}
	field.Configured = toolLayer(facts.Admitted, facts.AdmittedFrom)
	field.Loaded = toolLayer(facts.Registered, facts.RegisteredFrom)
	field.Effective = toolEffective(facts)
	return field
}

// toolLayer reports one presence layer. An absent tool is a real observation,
// not a missing one: the value says unavailable and the owner's own ref says
// what would have supplied it.
func toolLayer(present bool, source Source) Observation {
	if present {
		return Present(StringValue(ToolRegistered), source)
	}
	return Present(StringValue(ToolUnavailable), source)
}

// toolEffective says what a call meets now, in the order the call would meet
// it: a tool that is not registered cannot be called at all, one the plan
// gate does not admit is stopped before the boundary sees it, and one that
// gets that far is reported by what deciding it costs — never as allowed,
// because for most tools the answer belongs to arguments nobody has written.
func toolEffective(facts ToolFacts) Observation {
	switch {
	case !facts.Registered:
		return Present(StringValue(ToolUnavailable), facts.RegisteredFrom)
	case !facts.Callable:
		return Present(StringValue(ToolRestricted), sourceToolPlanGate)
	case facts.Check == CheckArguments:
		return Present(StringValue(ToolRequiresArgumentCheck), sourceToolArguments)
	case facts.Check == CheckPolicy:
		return Present(StringValue(ToolRegistered), sourceToolPolicy)
	case facts.Check == CheckToolName:
		return Present(StringValue(ToolRegistered), sourceToolName)
	default:
		return Present(StringValue(ToolRegistered), sourceToolUnclassified)
	}
}

// callToolState reads the optional accessor. A nil accessor is a wiring gap,
// and every layer it feeds reports unavailable rather than crashing the
// snapshot.
func callToolState(accessor func() ToolState) ToolState {
	if accessor == nil {
		return ToolState{}
	}
	return accessor()
}
