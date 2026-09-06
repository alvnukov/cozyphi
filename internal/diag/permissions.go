package diag

import "context"

// Permission field keys. They are declared statically so the catalog can list
// them and Explain can validate a key without observing anything.
const (
	KeyPermissionMode        = "mode"
	KeyPermissionGate        = "gate"
	KeyPermissionBypass      = "bypass"
	KeyPermissionBashDefault = "bash.default"
	KeyPermissionBashAllow   = "bash.allow"
	KeyPermissionBashDeny    = "bash.deny"
	KeyPermissionWrites      = "workspace.only_writes"
	KeyPermissionReads       = "workspace.only_reads"
	KeyPermissionSensitive   = "paths.sensitive"
	KeyPermissionMCPAllow    = "mcp.allow"
	KeyPermissionTasks       = "tasks"
	KeyPermissionMemory      = "memory"
	KeyPermissionAskTimeout  = "ask_timeout_sec"
	KeyPermissionSourceOrder = "source_order"
)

// permissionKeys is the declared key set, in the order Collect returns them.
var permissionKeys = []string{
	KeyPermissionGate,
	KeyPermissionMode,
	KeyPermissionBypass,
	KeyPermissionBashDefault,
	KeyPermissionBashAllow,
	KeyPermissionBashDeny,
	KeyPermissionWrites,
	KeyPermissionReads,
	KeyPermissionSensitive,
	KeyPermissionMCPAllow,
	KeyPermissionTasks,
	KeyPermissionMemory,
	KeyPermissionAskTimeout,
	KeyPermissionSourceOrder,
}

// permissionSourceOrder is the precedence the boundary is assembled under,
// weakest first: the built-in defaults, the config file's permissions block,
// the command line, the mode overlay a plan or a sub-agent role imposes, and
// last the session's own allow-all switch, which does not change a rule but
// stops every rule from being consulted.
var permissionSourceOrder = []string{
	string(SourceDefault),
	string(SourceConfigFile),
	string(SourceCLIFlag),
	string(SourcePlan),
	string(SourceSession),
}

// permissionReason states what this category deliberately leaves out. Rules
// are explained by shape and by count; a rule's text is not publishable,
// because a bash pattern, a sensitive path prefix and an mcp allow entry are
// written by the user and can name anything at all.
const permissionReason = "the boundary this session judges tool calls with: its mode, " +
	"the bypass in front of it, and the shape of its rules; rules are counted and " +
	"attributed, never quoted, so no bash pattern, sensitive path, mcp allow entry or " +
	"memory path has a field here, and the gate is only read — it is never handed a " +
	"request, so observing decides nothing and grants nothing"

// GateKind names the shape of an assembled permission boundary.
type GateKind string

// GateKind values.
const (
	// GateStatic is a compiled ruleset. It is the only kind whose policy can
	// be read.
	GateStatic GateKind = "static"
	// GateAllowAll is a boundary with no rules at all: every request is
	// allowed without being judged.
	GateAllowAll GateKind = "allow_all"
	// GateUnavailable is a boundary that could not be assembled, or one whose
	// assembly is incomplete. It denies, and Reason says what failed.
	GateUnavailable GateKind = "unavailable"
	// GateUnknown is a boundary this view does not recognize — something
	// decorated or replaced the gate. Its rules are not observable, and the
	// configured policy is not reported in their place.
	GateUnknown GateKind = "unknown"
)

// PermissionFacts is what the harness may say about one permission policy.
// It is an allowlist by construction: these members exist because they are
// safe to publish, and a bash pattern, a sensitive path prefix, an mcp allow
// entry and the memory directory have no member here to land in. The
// projection from the owner's own Policy lives with the owner, so this
// package never holds a type that carries a rule's text and no edit made here
// can start publishing one.
type PermissionFacts struct {
	// Known is false when there was no policy to read. Every layer fed from
	// it then reports unavailable rather than a zero that would read as a
	// real ruleset.
	Known bool
	// Mode is how Ask decisions are folded: interactive, readonly, autopilot
	// or headless-strict.
	Mode string
	// BashDefault is what a command matching no rule gets: allow, deny or ask.
	BashDefault string
	// BashAllow and BashDeny are how many patterns each list holds; the
	// IsDefault flags say whether the list is the built-in one or the config
	// file's, which is what makes a count explainable without quoting it.
	BashAllow          int
	BashDeny           int
	BashAllowIsDefault bool
	BashDenyIsDefault  bool
	// SensitivePaths is how many path prefixes are refused outright, and
	// MCPAllow how many mcp_call targets are pre-approved. Counts only: both
	// lists are written by the user and can name anything.
	SensitivePaths int
	MCPAllow       int
	// WorkspaceOnlyWrites and WorkspaceOnlyReads are the containment rules.
	WorkspaceOnlyWrites bool
	WorkspaceOnlyReads  bool
	// AskTimeoutSec is how long an approval prompt waits.
	AskTimeoutSec int
	// Tasks is how far the model may go with the task registry.
	Tasks string
	// Memory says only that a memory directory is bound to this policy — the
	// one write target outside the workspace. Where it is stays with the owner.
	Memory bool
	// AllowAll is the policy's own dangerously_allow_all flag. It is what was
	// asked for, not what is in force: whether the boundary is bypassing right
	// now is GateFacts.Bypassing.
	AllowAll bool
}

// GateFacts is what the harness may say about the assembled boundary: what
// shape it is, whether a bypass stands in front of it, and the ruleset it
// actually holds. It describes the gate the session judges tool calls with,
// wrappers included — never the configuration those rules were built from.
type GateFacts struct {
	// Known is false when nothing published a boundary to observe.
	Known bool
	// Kind is the shape of the boundary that decides.
	Kind GateKind
	// Reason explains a boundary whose rules cannot be read: the assembly
	// failure an unavailable gate carries, or why an unknown one is unknown.
	Reason string
	// Bypassable is true when the boundary can allow every request without
	// consulting a rule; Bypassing is true when it is doing so right now.
	Bypassable bool
	Bypassing  bool
	// Policy is the ruleset the boundary holds. Its Known is false for every
	// boundary that has none — an unknown one included, because reporting the
	// configured rules there would name rules that are not deciding.
	Policy PermissionFacts
}

// Source refs for the layers whose origin is a config key rather than the
// assembly itself. They name the key that sets the value, so an explanation
// says where to go and change it — and where there is no key, they say that
// instead of implying one exists. A value that is still the built-in default
// is reported as one: these say config file only where the observed value is
// not what an unconfigured session would hold.
var (
	sourcePermissionMode        = Source{Kind: SourceConfigFile, Ref: "permissions.mode"}
	sourcePermissionBashDefault = Source{Kind: SourceConfigFile, Ref: "permissions.bash.default"}
	sourcePermissionWrites      = Source{Kind: SourceConfigFile, Ref: "permissions.workspace_only_writes"}
	sourcePermissionReads       = Source{
		Kind: SourceDefault,
		Ref:  "the built-in read containment; no configuration key sets it",
	}
	sourcePermissionSensitive = Source{
		Kind: SourceDefault,
		Ref:  "the built-in sensitive-path list; no configuration key sets it",
	}
	sourcePermissionMCPAllow   = Source{Kind: SourceConfigFile, Ref: "permissions.mcp.allow"}
	sourcePermissionTasks      = Source{Kind: SourceConfigFile, Ref: "permissions.tasks"}
	sourcePermissionAskTimeout = Source{Kind: SourceConfigFile, Ref: "permissions.ask_timeout_sec"}
	sourcePermissionAllowAll   = Source{Kind: SourceConfigFile, Ref: "permissions.dangerously_allow_all"}
	sourcePermissionMemory     = Source{
		Kind: SourceComputed,
		Ref:  "the session's own memory directory, bound when the boundary is assembled",
	}
	sourcePermissionInstalled = Source{
		Kind: SourceComputed,
		Ref:  "the boundary this session judges tool calls with",
	}
	sourcePermissionBypassable = Source{
		Kind: SourceComputed,
		Ref:  "whether the boundary can allow every request without consulting a rule",
	}
	sourcePermissionSwitch = Source{
		Kind: SourceSession,
		Ref:  "the session's allow-all switch",
	}
	sourcePermissionUnjudged = Source{
		Kind: SourceCLIFlag,
		Ref:  "nothing is judged: the boundary allows every request",
	}
	// sourcePermissionSession is the fallback attribution for a loaded value
	// the configuration did not ask for and no overlay claimed. It says when
	// the change happened rather than naming an origin it does not know.
	sourcePermissionSession = Source{
		Kind: SourceSession,
		Ref:  "changed after the configuration was loaded",
	}
)

// PermissionDeps binds the permission collector to its two owners: the loader
// answers what was configured, the assembled boundary answers what is acting.
// They stay separate on purpose — the effective layer must never be recomputed
// from the configuration, because a project's permissions block is exactly
// what stops being the truth once plan mode, a role ceiling or a bypass has
// been applied to it.
type PermissionDeps struct {
	// Configured is the permissions block the configuration resolved at
	// startup: the built-in defaults with the config file merged over them.
	// Reading it never re-reads the file.
	Configured func() PermissionFacts
	// Defaults is the built-in policy: what a session with no permissions
	// block at all would run on. A layer that matches it is attributed to the
	// default rather than to the config file, which is the only way to tell
	// the two apart without asking the loader which keys the file spelled.
	// Unwired, every value is attributed to the key that sets it.
	Defaults func() PermissionFacts
	// Gate observes the assembled boundary through the owner's own
	// projection. It is read, never asked: no accessor here may hand the gate
	// a request, run a command, resolve a path or simulate an approval.
	Gate func() GateFacts
	// Overlay names what narrowed the configured policy before the boundary
	// was assembled — plan mode, a sub-agent role ceiling, the headless
	// default. A zero Source means nothing did, and a loaded layer that still
	// differs is attributed to the session rather than to a guess.
	Overlay func() Source
}

// permissionCollector observes the access boundary: what the configuration
// asked for, what the assembled gate holds, and what is deciding right now.
type permissionCollector struct {
	deps PermissionDeps
}

// NewPermissionCollector builds the permission collector.
func NewPermissionCollector(deps PermissionDeps) Collector {
	return &permissionCollector{deps: deps}
}

func (*permissionCollector) Category() Category { return CategoryPermissions }

// Status is answered from the declared key set alone: listing the catalog
// reads neither the configuration nor the gate.
func (*permissionCollector) Status() Status {
	keys := make([]string, len(permissionKeys))
	copy(keys, permissionKeys)
	return Status{Availability: AvailabilityAvailable, Reason: permissionReason, Keys: keys}
}

// Collect reads both owners once and derives every field from that one read.
// Nothing here evaluates a request, runs a command, touches a path or
// reassembles a gate.
func (c *permissionCollector) Collect(_ context.Context) ([]Field, error) {
	p := c.layers()
	return []Field{
		p.gateField(),
		p.rule(KeyPermissionMode, ApplyReload, sourcePermissionMode, factPermissionMode),
		p.bypass(),
		p.rule(KeyPermissionBashDefault, ApplyReload, sourcePermissionBashDefault, factBashDefault),
		p.bashList(KeyPermissionBashAllow, "allow", factBashAllow, bashAllowIsDefault),
		p.bashList(KeyPermissionBashDeny, "deny", factBashDeny, bashDenyIsDefault),
		p.rule(KeyPermissionWrites, ApplyReload, sourcePermissionWrites, factWorkspaceWrites),
		p.rule(KeyPermissionReads, ApplyReload, sourcePermissionReads, factWorkspaceReads),
		p.rule(KeyPermissionSensitive, ApplyReload, sourcePermissionSensitive, factSensitivePaths),
		p.rule(KeyPermissionMCPAllow, ApplyReload, sourcePermissionMCPAllow, factMCPAllow),
		p.rule(KeyPermissionTasks, ApplyImmediate, sourcePermissionTasks, factTasks),
		p.memory(),
		p.rule(KeyPermissionAskTimeout, ApplyReload, sourcePermissionAskTimeout, factAskTimeout),
		p.sourceOrder(),
	}, nil
}

// permissionLayers is one observation of both owners, taken once and then
// used for every field, so the fourteen fields of a snapshot describe the
// same boundary rather than fourteen slightly different ones.
type permissionLayers struct {
	configured PermissionFacts
	defaults   PermissionFacts
	gate       GateFacts
	overlay    Source
}

func (c *permissionCollector) layers() permissionLayers {
	return permissionLayers{
		configured: callPermissionFacts(c.deps.Configured),
		defaults:   callPermissionFacts(c.deps.Defaults),
		gate:       callGateFacts(c.deps.Gate),
		overlay:    callSource(c.deps.Overlay),
	}
}

// narrowing names what stands between the configured policy and the one the
// boundary holds. The entry point publishes it, because only the entry point
// knows: plan mode, a sub-agent role ceiling, the shape of a headless run.
// With nothing published, the difference is dated rather than explained — it
// happened after the configuration was loaded, and that is all this view can
// honestly say.
func (p permissionLayers) narrowing() Source {
	if p.overlay.Kind != "" {
		return p.overlay
	}
	return sourcePermissionSession
}

// bypassSource says who is holding the boundary open. A gate with no rules at
// all was assembled that way by the command line or the config flag; a
// wrapper in front of a real ruleset is the session's own switch, which the
// user can turn off again.
func (p permissionLayers) bypassSource() Source {
	if p.gate.Kind == GateAllowAll {
		return sourcePermissionUnjudged
	}
	return sourcePermissionSwitch
}

// rule builds one field of the ruleset across the three layers: what the
// configuration asked for, what the assembled boundary holds, and what is
// deciding now. A boundary whose rules cannot be read leaves both of the
// latter unavailable — never filled in from the configured layer, which is
// precisely the substitution this category exists to prevent.
func (p permissionLayers) rule(key string, apply Apply, source Source, pick permissionFact) Field {
	return p.field(key, apply,
		p.attribute(source, pick, p.configured), p.attribute(source, pick, p.gate.Policy), pick)
}

// attribute tells a rule the config file set from one nobody touched. The
// loader merges the file over the built-in policy and keeps no record of which
// keys the file spelled, so the comparison against the defaults is what is
// left: a value that is still the default is reported as the default, and the
// config key is named as what would replace it. A file that sets a key to the
// value it already had is indistinguishable here, and reads as the default it
// restates.
func (p permissionLayers) attribute(source Source, pick permissionFact, facts PermissionFacts) Source {
	if source.Kind != SourceConfigFile || !p.defaults.Known || !facts.Known {
		return source
	}
	value, _ := pick(facts)
	defaultValue, defaultSet := pick(p.defaults)
	if !defaultSet || !sameValue(value, defaultValue) {
		return source
	}
	return Source{Kind: SourceDefault, Ref: "the built-in default; " + source.Ref + " replaces it"}
}

// bashList is rule for the two lists whose provenance is part of the answer:
// a count is only explainable next to whether the list is the built-in one or
// the config file's, and that differs per layer.
func (p permissionLayers) bashList(
	key, name string, pick permissionFact, isDefault func(PermissionFacts) bool,
) Field {
	return p.field(key, ApplyReload, bashListSource(name, isDefault(p.configured)),
		bashListSource(name, isDefault(p.gate.Policy)), pick)
}

func (p permissionLayers) field(
	key string, apply Apply, configuredSource, loadedSource Source, pick permissionFact,
) Field {
	configuredValue, configuredSet := pick(p.configured)
	loadedValue, loadedSet := pick(p.gate.Policy)
	field := Field{
		Key:        key,
		Configured: layerObservation(p.configured.Known, configuredSet, configuredValue, configuredSource),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      apply,
		Scope:      ScopeSession,
	}
	if !p.gate.Policy.Known {
		return field
	}
	if !p.configured.Known || !sameValue(configuredValue, loadedValue) {
		loadedSource = p.narrowing()
	}
	field.Loaded = layerObservation(true, loadedSet, loadedValue, loadedSource)
	field.Effective = field.Loaded
	if p.gate.Bypassing {
		field.Effective = suspended(p.bypassSource())
	}
	return field
}

// gateField names the boundary itself. Nothing configures a shape — it is
// what the assembly produced — so the configured layer does not exist for it,
// and the source ref carries the reason a boundary that cannot be read gives
// for itself.
func (p permissionLayers) gateField() Field {
	observation := Unavailable()
	if p.gate.Known && p.gate.Kind != "" {
		source := sourcePermissionInstalled
		if p.gate.Reason != "" {
			source = Source{Kind: SourceComputed, Ref: p.gate.Reason}
		}
		observation = Present(StringValue(string(p.gate.Kind)), source)
	}
	return Field{
		Key:        KeyPermissionGate,
		Configured: NotApplicable(SourceComputed),
		Loaded:     observation,
		Effective:  observation,
		Apply:      ApplyImmediate,
		Scope:      ScopeSession,
	}
}

// bypass separates the three things "allow all" can mean. Configured is what
// the file asked for; loaded is whether the assembled boundary is able to
// allow everything at all; effective is whether it is doing so right now,
// which the user can change from the palette without touching a rule.
func (p permissionLayers) bypass() Field {
	loaded := Unavailable()
	effective := Unavailable()
	if p.gate.Known {
		loaded = Present(BoolValue(p.gate.Bypassable), sourcePermissionBypassable)
		effective = Present(BoolValue(p.gate.Bypassing), p.bypassSource())
	}
	return Field{
		Key: KeyPermissionBypass,
		Configured: layerObservation(p.configured.Known, p.configured.AllowAll,
			BoolValue(p.configured.AllowAll), sourcePermissionAllowAll),
		Loaded:    loaded,
		Effective: effective,
		Apply:     ApplyImmediate,
		Scope:     ScopeSession,
	}
}

// memory reports whether the boundary has a memory directory bound to it —
// the one write target outside the workspace. A config file cannot set it:
// the entry point binds the session's own directory while assembling the
// gate, so the configured layer does not exist rather than reporting a false
// that would read as "memory is off".
func (p permissionLayers) memory() Field {
	loaded := Unavailable()
	effective := Unavailable()
	if p.gate.Policy.Known {
		loaded = Present(BoolValue(p.gate.Policy.Memory), sourcePermissionMemory)
		effective = loaded
		if p.gate.Bypassing {
			effective = suspended(p.bypassSource())
		}
	}
	return Field{
		Key:        KeyPermissionMemory,
		Configured: NotApplicable(SourceComputed),
		Loaded:     loaded,
		Effective:  effective,
		Apply:      ApplyNewSession,
		Scope:      ScopeSession,
	}
}

// sourceOrder publishes the precedence the other thirteen fields are read
// against. It is compiled into the build, so it has no configured or loaded
// layer to report.
func (permissionLayers) sourceOrder() Field {
	return Field{
		Key:        KeyPermissionSourceOrder,
		Configured: NotApplicable(SourceBuild),
		Loaded:     NotApplicable(SourceBuild),
		Effective: Present(ListValue(permissionSourceOrder),
			Source{Kind: SourceBuild, Ref: "permission assembly precedence, weakest first"}),
		Apply: ApplyRestart,
		Scope: ScopeSession,
	}
}

// suspended marks a rule that is loaded but is not deciding anything: the
// boundary is allowing every request, so the rule's value is not the answer
// to "what happens now". Reporting the value here instead would describe a
// restriction that is not being applied.
func suspended(source Source) Observation {
	return Observation{State: StateNotApplicable, Value: NoValue(), Source: source}
}

// bashListSource attributes one bash list. A list the config file replaced is
// the file's; an untouched one is the built-in list, and saying so is how a
// count explains a restriction without quoting a pattern.
func bashListSource(name string, isDefault bool) Source {
	if isDefault {
		return Source{
			Kind: SourceDefault,
			Ref:  "the built-in bash " + name + "list; permissions.bash." + name + " replaces it whole",
		}
	}
	return Source{Kind: SourceConfigFile, Ref: "permissions.bash." + name}
}

// sameValue compares two observed values without touching the list member:
// every permission fact is a scalar, and a Value is not comparable with ==.
func sameValue(a, b Value) bool {
	return a.Kind == b.Kind && a.Str == b.Str && a.Int == b.Int && a.Bool == b.Bool
}

// permissionFact reads one member out of the facts and says whether anything
// set it. Counts and flags are set whenever the policy could be read at all:
// zero bash allow patterns is a real restriction, not a missing answer.
type permissionFact func(PermissionFacts) (Value, bool)

func factPermissionMode(f PermissionFacts) (Value, bool) {
	return StringValue(f.Mode), f.Mode != ""
}

func factBashDefault(f PermissionFacts) (Value, bool) {
	return StringValue(f.BashDefault), f.BashDefault != ""
}

func factBashAllow(f PermissionFacts) (Value, bool) {
	return IntValue(int64(f.BashAllow)), f.Known
}

func factBashDeny(f PermissionFacts) (Value, bool) {
	return IntValue(int64(f.BashDeny)), f.Known
}

func factWorkspaceWrites(f PermissionFacts) (Value, bool) {
	return BoolValue(f.WorkspaceOnlyWrites), f.Known
}

func factWorkspaceReads(f PermissionFacts) (Value, bool) {
	return BoolValue(f.WorkspaceOnlyReads), f.Known
}

func factSensitivePaths(f PermissionFacts) (Value, bool) {
	return IntValue(int64(f.SensitivePaths)), f.Known
}

func factMCPAllow(f PermissionFacts) (Value, bool) {
	return IntValue(int64(f.MCPAllow)), f.Known
}

func factTasks(f PermissionFacts) (Value, bool) { return StringValue(f.Tasks), f.Tasks != "" }

// factAskTimeout treats a non-positive timeout as unset: the loader refuses
// one, so a zero here means nothing set it rather than "no wait at all".
func factAskTimeout(f PermissionFacts) (Value, bool) {
	return IntValue(int64(f.AskTimeoutSec)), f.AskTimeoutSec > 0
}

// bashAllowIsDefault and bashDenyIsDefault select one list's attribution, so
// bashList can ask the same question of the configured facts and of the
// boundary's own.
func bashAllowIsDefault(f PermissionFacts) bool { return f.BashAllowIsDefault }

func bashDenyIsDefault(f PermissionFacts) bool { return f.BashDenyIsDefault }

// callPermissionFacts and its siblings read optional accessors. A nil
// accessor is a wiring gap, and the layers it feeds report unavailable rather
// than crashing the snapshot.
func callPermissionFacts(accessor func() PermissionFacts) PermissionFacts {
	if accessor == nil {
		return PermissionFacts{}
	}
	return accessor()
}

func callGateFacts(accessor func() GateFacts) GateFacts {
	if accessor == nil {
		return GateFacts{}
	}
	return accessor()
}
