package diag

import (
	"context"
	"math"
	"strconv"
)

// Plan field keys. They are declared statically so the catalog can list them
// and Explain can validate a key without observing anything.
//
// The groups answer four different questions and are deliberately not mixed:
// what posture this session runs in, what policy the plan gate compiles from,
// what the durable plan is in right now, and what the step in progress is
// allowed to do. The telemetry group is last because it is the only one whose
// numbers are session totals rather than a description of the present.
const (
	// KeyPlanEnabled is whether this session runs under a plan gate at all.
	KeyPlanEnabled = "enabled"
	// KeyPlanMode is the engine's posture: authoring a plan or working one.
	KeyPlanMode = "mode"
	// KeyPlanGatePhase is whether a miss is guidance or a refusal.
	KeyPlanGatePhase = "gate.phase"

	// KeyPlanPolicyTypes is the ordered step-type vocabulary.
	KeyPlanPolicyTypes = "policy.types"
	// KeyPlanPolicyTools is every tool the policy assigns to a step type.
	KeyPlanPolicyTools = "policy.tools"
	// KeyPlanPolicyExemptions is the tools that never need a step.
	KeyPlanPolicyExemptions = "policy.exemptions"
	// KeyPlanPolicyAuthoring is the plan-authoring grammar selector.
	KeyPlanPolicyAuthoring = "policy.authoring"
	// KeyPlanPolicyTypeModels is the per-type model pins a new plan inherits.
	KeyPlanPolicyTypeModels = "policy.type_models"
	// KeyPlanPolicyActions is the plan-scope automations a new plan inherits.
	KeyPlanPolicyActions = "policy.actions"

	// KeyPlanLifecycle is the one word for where the durable plan stands.
	KeyPlanLifecycle = "plan.lifecycle"
	// KeyPlanSchema is the authoring contract the snapshot was written under.
	KeyPlanSchema = "plan.schema"
	// KeyPlanRevision is the durable plan's own revision counter.
	KeyPlanRevision = "plan.revision"
	// KeyPlanContractEpoch counts material changes to the work contract.
	KeyPlanContractEpoch = "plan.contract_epoch"
	// KeyPlanApproved is the user-owned approval flag.
	KeyPlanApproved = "plan.approved"
	// KeyPlanResult is how a finished plan ended, if it has.
	KeyPlanResult = "plan.result"

	// KeyPlanStepsTotal is how many steps the plan carries.
	KeyPlanStepsTotal = "steps.total"
	// KeyPlanStepsRemaining is how many of them still owe work.
	KeyPlanStepsRemaining = "steps.remaining"
	// KeyPlanStepsStatuses is the count per status, in status order.
	KeyPlanStepsStatuses = "steps.statuses"

	// KeyPlanStepCurrent is the id of the step in progress.
	KeyPlanStepCurrent = "step.current"
	// KeyPlanStepType is that step's type.
	KeyPlanStepType = "step.type"
	// KeyPlanStepTools is what that step's type permits, and what the gate
	// admits from it right now.
	KeyPlanStepTools = "step.tools"
	// KeyPlanStepJIT is whether the step needs a just-in-time user grant.
	KeyPlanStepJIT = "step.jit"
	// KeyPlanStepAttempts is how many accepted tool calls it has recorded.
	KeyPlanStepAttempts = "step.attempts"
	// KeyPlanStepActions is the automations bound to that step's lifecycle.
	KeyPlanStepActions = "step.actions"
	// KeyPlanStepSkills is the step's skills by name and application state.
	KeyPlanStepSkills = "step.skills"
	// KeyPlanSkillsPending is how many loaded skill bodies wait to be sent.
	KeyPlanSkillsPending = "skills.pending"

	// KeyPlanTelemetryGate counts what the gate did with addressed calls.
	KeyPlanTelemetryGate = "telemetry.gate"
	// KeyPlanTelemetryApproval counts approval movement and its latency.
	KeyPlanTelemetryApproval = "telemetry.approval"
	// KeyPlanTelemetryProjection accounts the prompt budget the plan spends.
	KeyPlanTelemetryProjection = "telemetry.projection"
	// KeyPlanTelemetryCompletion counts how steps and plans finished.
	KeyPlanTelemetryCompletion = "telemetry.completion"
	// KeyPlanTelemetryAuthoring counts drafts and patch retries.
	KeyPlanTelemetryAuthoring = "telemetry.authoring"
)

// Plan lifecycle vocabulary. It is six values rather than a pair of booleans
// because the four middle ones are what a developer is actually asking about:
// a plan that exists but nobody approved, one approved with nothing running,
// one with a step in progress, and one already discharged behave differently
// at the gate, and collapsing them loses exactly the distinction.
const (
	// PlanLifecycleDisabled means this session has no plan gate at all — a
	// sub-agent carries one job, not a plan.
	PlanLifecycleDisabled = "disabled"
	// PlanLifecycleAbsent means the gate is there and no plan has been made.
	PlanLifecycleAbsent = "absent"
	// PlanLifecycleDraft means a plan exists and is not approved.
	PlanLifecycleDraft = "draft"
	// PlanLifecycleApproved means it is approved with no step in progress.
	PlanLifecycleApproved = "approved"
	// PlanLifecycleActive means a step is in progress.
	PlanLifecycleActive = "active"
	// PlanLifecycleClosed means the plan recorded a result; its contract is
	// discharged and the gate lets every call through.
	PlanLifecycleClosed = "closed"
)

// Skill application vocabulary. A plan step names skills; this says what
// became of each name, never a byte of what the skill says.
const (
	// PlanSkillDelivered means the body already reached the model in this
	// session, so naming it again sends nothing.
	PlanSkillDelivered = "delivered"
	// PlanSkillQueued means it is loaded and parked, waiting for the next
	// prompt or pre-dispatch boundary to drain it.
	PlanSkillQueued = "queued"
	// PlanSkillPending means the step names it and nothing has loaded it yet.
	PlanSkillPending = "pending"
	// PlanSkillDisabled means the user switched this name off; the step keeps
	// listing it so the toggle can come back.
	PlanSkillDisabled = "disabled"
)

// planKeys is the declared key set, in the order Collect returns them.
var planKeys = []string{
	KeyPlanEnabled,
	KeyPlanMode,
	KeyPlanGatePhase,

	KeyPlanPolicyTypes,
	KeyPlanPolicyTools,
	KeyPlanPolicyExemptions,
	KeyPlanPolicyAuthoring,
	KeyPlanPolicyTypeModels,
	KeyPlanPolicyActions,

	KeyPlanLifecycle,
	KeyPlanSchema,
	KeyPlanRevision,
	KeyPlanContractEpoch,
	KeyPlanApproved,
	KeyPlanResult,

	KeyPlanStepsTotal,
	KeyPlanStepsRemaining,
	KeyPlanStepsStatuses,

	KeyPlanStepCurrent,
	KeyPlanStepType,
	KeyPlanStepTools,
	KeyPlanStepJIT,
	KeyPlanStepAttempts,
	KeyPlanStepActions,
	KeyPlanStepSkills,
	KeyPlanSkillsPending,

	KeyPlanTelemetryGate,
	KeyPlanTelemetryApproval,
	KeyPlanTelemetryProjection,
	KeyPlanTelemetryCompletion,
	KeyPlanTelemetryAuthoring,
}

// planReason states what this category answers and what it deliberately
// leaves out. The omissions are the point: the plan is the one place in this
// harness where the model's own prose is durable, so a view of it that
// carried any of that prose would be a transcript by another name.
const planReason = "where the durable plan stands, what policy the gate compiles from, what the " +
	"step in progress is allowed to do, and the plan's own counters; names, types, statuses and " +
	"numbers only — no goal, approach, working context, step text, note, evidence, blocker or " +
	"transition reason, no skill body, and no mutation id or replay token. Nothing here approves " +
	"a plan, starts or finishes a step, records evidence or moves the plan revision: the snapshot " +
	"is read, and reading it advances nothing"

// PlanPolicySet is one compiled plan-gate policy reduced to the names it is
// made of. Every member is a name or a list of names: there is no member for
// a command, an argument or an environment, so a policy cannot carry one out
// of here however it was configured.
type PlanPolicySet struct {
	// Known is false for a layer that does not exist — a session with no
	// plan gate has no compiled policy to describe.
	Known bool
	// Types is the step-type vocabulary in capability order, least first.
	Types []string
	// Tools is every tool assigned to a step type, in the same order: the
	// gateable vocabulary this policy knows how to admit.
	Tools []string
	// Exemptions is the tools that pass the gate without naming a step.
	Exemptions []string
	// Authoring is the plan-authoring grammar this policy selects.
	Authoring string
	// TypeModels is the per-type model pins, rendered "type=model". A new
	// plan inherits them when its author pins nothing.
	TypeModels []string
	// Actions is the plan-scope automations, rendered "event:type".
	Actions []string
}

// PlanPolicy is the same policy at the three layers the epic distinguishes.
// They are three real objects here, not one read three ways: the harness
// ships one, the configuration compiles another, and a third is baked into
// the prompt and tool schemas the model is looking at. The third lags the
// second by exactly one rebind, and that lag is the answer to "why is the
// model still being told the old step types".
type PlanPolicy struct {
	// Default is the built-in policy: what this harness gates with when no
	// configuration names a plan policy.
	Default PlanPolicySet
	// Loaded is the policy the runtime has published — what the next tool
	// call is checked against.
	Loaded PlanPolicySet
	// Projected is the policy baked into the current system prompt and tool
	// schemas: what the model has been told it may do.
	Projected PlanPolicySet
	// Revision fingerprints the published policy, so two observations taken
	// across a policy change are visibly of two different policies.
	Revision string
}

// PlanSkill is one skill a step names: the name and what became of it. There
// is no member for a body, a path or a description — the point of this field
// is that a skill can be accounted for without being quoted.
type PlanSkill struct {
	Name string
	// State is one of the PlanSkill* application states.
	State string
}

// PlanStatusCount is how many steps stand in one status.
type PlanStatusCount struct {
	Status string
	Count  int
}

// PlanStep is the step in progress, reduced to what may be said about it: its
// stable id, its type, what that type reaches, and the automations and skills
// bound to it. The prose a step carries — what it is, why, what would prove
// it done, what blocked it, what it found — has no member here.
type PlanStep struct {
	// Known is false when no step is in progress. Every field fed from it
	// then reports unset rather than an empty id that would read as a step.
	Known bool
	// ID is the step's stable slug: the identity a plan_step names. It is
	// the one model-authored string this view carries, and its shape — a
	// lowercase slug of at most 64 characters — is what makes it sayable.
	ID string
	// Type is the step's declared type.
	Type string
	// Tools is what that type permits under the published policy, in policy
	// order. It is the step's ceiling, not a promise about one call.
	Tools []string
	// JIT is whether the step is gated on a just-in-time user grant, and
	// JITGranted whether the user has given one at the current epoch.
	JIT        bool
	JITGranted bool
	// Attempts is how many accepted tool calls the harness recorded against
	// this step — a count of its evidence, never the evidence.
	Attempts int
	// Actions is the step-scope automations, rendered "event:type".
	Actions []string
	// Skills is one entry per name the step's inject_skill action carries.
	Skills []PlanSkill
}

// PlanTelemetry mirrors the harness's plan telemetry. Every member is a
// counter or a bounded millisecond duration, and the fixed numeric schema is
// the leak contract: no string, map or slice member can carry plan content,
// a tool result or an unbounded label.
type PlanTelemetry struct {
	// Known is false when no session has published telemetry yet, which is
	// not the same as a session whose counters are all zero.
	Known bool

	Misses              uint64
	TransitionConflicts uint64
	IdempotentRetries   uint64
	StandaloneStarts    uint64
	PlanOnlyRounds      uint64

	ApprovalChurn       uint64
	MaterialRevisions   uint64
	MaterialReapprovals uint64
	ApprovalLatency1s   uint64
	ApprovalLatency10s  uint64
	ApprovalLatency60s  uint64
	ApprovalLatencySlow uint64

	ProjectionInjections uint64
	ProjectionBytes      uint64
	ProjectionBytesLast  uint64

	CompletionsSuccess         uint64
	CompletionsAbandoned       uint64
	CompletionsWithoutEvidence uint64
	Archives                   uint64
	ArchiveLatencyLastMS       uint64
	ArchiveLatencyMaxMS        uint64

	DraftsAdaptive uint64
	DraftsLegacy   uint64
	PatchRetries   uint64
}

// PlanState is one observation of the plan layer: the posture, the policy at
// its three layers, the durable snapshot's own lifecycle, the step in
// progress, and the session's counters.
type PlanState struct {
	// Known is false when no engine has published a plan layer yet. Every
	// field fed from it then reports unavailable rather than a disabled gate
	// that would read as a decision somebody made.
	Known bool
	// Enabled is whether this session runs under a plan gate. A sub-agent
	// does not, and neither does a session assembled with its own tool set.
	Enabled bool
	// Mode is the engine's posture: the plan-authoring one narrows the
	// built-in tools to reading, the ordinary one works the plan.
	Mode string
	// GatePhase is whether a miss is guidance or a refusal, or empty where
	// there is no gate to stand in a phase.
	GatePhase string
	// Policy is the compiled gate policy at its three layers.
	Policy PlanPolicy
	// Present is whether a durable plan exists at all.
	Present bool
	// Schema names the authoring contract the snapshot was written under.
	Schema string
	// Approved is the user-owned approval flag; Result is how a finished
	// plan ended, empty while it is still open.
	Approved bool
	Result   string
	// Revision and ContractEpoch are the plan's own counters: the snapshot
	// generation, and how many times the work contract materially moved.
	Revision      uint64
	ContractEpoch uint64
	// StepsTotal is how many steps the plan carries and StepsRemaining how
	// many are not terminal; Statuses is the breakdown in status order.
	StepsTotal     int
	StepsRemaining int
	Statuses       []PlanStatusCount
	// Step is the step in progress.
	Step PlanStep
	// SkillsPending is how many loaded skill bodies are parked waiting for
	// the next prompt or pre-dispatch boundary.
	SkillsPending int
	// Telemetry is the session's plan counters.
	Telemetry PlanTelemetry
}

// Sources for the plan layers. They name what decides a value, never the
// value: no config text, no plan prose and no skill body reaches a ref.
var (
	sourcePlanSession = Source{
		Kind: SourceSession,
		Ref:  "this session runs the durable plan the gate checks against",
	}
	sourcePlanNoGate = Source{
		Kind: SourceSession,
		Ref: "no plan gate: a session assembled with its own tool set, or a sub-agent, " +
			"carries one job rather than a plan",
	}
	sourcePlanMode = Source{
		Kind: SourceSession,
		Ref:  "the engine's posture; the plan-authoring one narrows the built-in tools to reading",
	}
	sourcePlanPhase = Source{
		Kind: SourceSession,
		Ref:  "the phase the plan gate stands in; deny is where a miss stops being guidance",
	}
	sourcePlanBuiltinPolicy = Source{
		Kind: SourceDefault,
		Ref:  "the built-in plan policy this harness gates with when no configuration names one",
	}
	sourcePlanLoadedPolicy = Source{
		Kind: SourceConfigFile,
		Ref:  "plan.defaults as the runtime compiled and published it; the next tool call is checked against this",
	}
	sourcePlanProjectedPolicy = Source{
		Kind: SourceSession,
		Ref: "the policy baked into the current system prompt and tool schemas — what the model " +
			"has been told it may do, which lags a published change by one rebind",
	}
	sourcePlanNoPolicy = Source{
		Kind: SourceSession,
		Ref:  "no policy is compiled for this layer; nothing is being gated against it",
	}
	sourcePlanSnapshot = Source{
		Kind: SourcePlan,
		Ref:  "the durable plan snapshot, read as the harness owner holds it",
	}
	sourcePlanNoPlan = Source{
		Kind: SourcePlan,
		Ref:  "no durable plan has been made in this session",
	}
	sourcePlanApproval = Source{
		Kind: SourceSession,
		Ref:  "the user-owned approval flag; the model cannot set it and this view cannot move it",
	}
	sourcePlanOpen = Source{
		Kind: SourcePlan,
		Ref:  "the plan is still open; a result is recorded only when it finishes",
	}
	sourcePlanStep = Source{
		Kind: SourcePlan,
		Ref:  "the step in progress in the durable plan",
	}
	sourcePlanNoStep = Source{
		Kind: SourcePlan,
		Ref:  "no step is in progress; nothing is being advanced right now",
	}
	sourcePlanStepCeiling = Source{
		Kind: SourcePlan,
		Ref:  "what the step's type permits under the published policy — its ceiling, not a verdict on one call",
	}
	sourcePlanStepUnapproved = Source{
		Kind: SourcePlan,
		Ref:  "the plan is not approved, so the step's type admits nothing beyond the exempt tools",
	}
	sourcePlanJITGranted = Source{
		Kind: SourceSession,
		Ref:  "the user granted this just-in-time step at the current contract epoch",
	}
	sourcePlanJITWanted = Source{
		Kind: SourceSession,
		Ref:  "a just-in-time step waiting on a user grant; a material change to the contract expires one",
	}
	sourcePlanNotJIT = Source{
		Kind: SourcePlan,
		Ref:  "the step is not just-in-time; approving the plan approved it",
	}
	sourcePlanEvidenceCount = Source{
		Kind: SourcePlan,
		Ref:  "how many accepted tool calls the harness filed against this step; the evidence itself stays in the plan",
	}
	sourcePlanSkillState = Source{
		Kind: SourcePlan,
		Ref: "the skills the step names and what became of each — delivered, queued, pending or " +
			"switched off; no skill body is read or carried here",
	}
	sourcePlanSkillQueue = Source{
		Kind: SourceSession,
		Ref:  "skill bodies already loaded and parked for the next prompt boundary",
	}
	sourcePlanTelemetry = Source{
		Kind: SourceComputed,
		Ref: "the session's plan counters: numbers this harness incremented on its own operations, " +
			"never derived by reading back the transcript",
	}
	sourcePlanNoTelemetry = Source{
		Kind: SourceSession,
		Ref:  "no session has published plan telemetry yet, which is not the same as counters at zero",
	}
)

// PlanDeps binds the plan collector to its one owner. The engine holds the
// posture, the gate, the policy runtime and the session's plan together, and
// it is asked for all of them at once so one snapshot describes one plan
// rather than several taken a moment apart.
type PlanDeps struct {
	// State observes the engine's plan layer through the owner's own
	// projection. It is read, never exercised: no accessor here may approve
	// a plan, start or finish a step, file evidence, patch a snapshot or
	// spend a mutation id.
	State func() PlanState
}

// planCollector observes the durable plan and the gate that reads it.
type planCollector struct {
	deps PlanDeps
}

// NewPlanCollector builds the plan collector.
func NewPlanCollector(deps PlanDeps) Collector {
	return &planCollector{deps: deps}
}

func (*planCollector) Category() Category { return CategoryPlan }

// Status is answered from the declared key set alone: listing the catalog
// reads no plan and compiles no policy.
func (*planCollector) Status() Status {
	keys := make([]string, len(planKeys))
	copy(keys, planKeys)
	return Status{Availability: AvailabilityAvailable, Reason: planReason, Keys: keys}
}

// Collect reads the owner once and derives every field from that one read.
func (c *planCollector) Collect(_ context.Context) ([]Field, error) {
	state := callPlanState(c.deps.State)
	return []Field{
		state.enabled(),
		state.mode(),
		state.phase(),

		state.policyList(KeyPlanPolicyTypes, func(set PlanPolicySet) []string { return set.Types }),
		state.policyList(KeyPlanPolicyTools, func(set PlanPolicySet) []string { return set.Tools }),
		state.policyList(KeyPlanPolicyExemptions, func(set PlanPolicySet) []string { return set.Exemptions }),
		state.policyAuthoring(),
		state.policyList(KeyPlanPolicyTypeModels, func(set PlanPolicySet) []string { return set.TypeModels }),
		state.policyList(KeyPlanPolicyActions, func(set PlanPolicySet) []string { return set.Actions }),

		state.lifecycle(),
		state.schema(),
		state.count(KeyPlanRevision, clampCounter(state.Revision)),
		state.count(KeyPlanContractEpoch, clampCounter(state.ContractEpoch)),
		state.approved(),
		state.result(),

		state.count(KeyPlanStepsTotal, int64(state.StepsTotal)),
		state.count(KeyPlanStepsRemaining, int64(state.StepsRemaining)),
		state.statuses(),

		state.stepID(),
		state.stepType(),
		state.stepTools(),
		state.stepJIT(),
		state.stepAttempts(),
		state.stepActions(),
		state.stepSkills(),
		state.skillsPending(),

		state.telemetry(KeyPlanTelemetryGate, state.Telemetry.gate()),
		state.telemetry(KeyPlanTelemetryApproval, state.Telemetry.approval()),
		state.telemetry(KeyPlanTelemetryProjection, state.Telemetry.projection()),
		state.telemetry(KeyPlanTelemetryCompletion, state.Telemetry.completion()),
		state.telemetry(KeyPlanTelemetryAuthoring, state.Telemetry.authoring()),
	}, nil
}

// clampCounter carries one of the plan's own unsigned counters into the
// signed value this package publishes. Neither can plausibly reach the
// ceiling — a revision moves once per durable write — but the two widths meet
// here and nowhere else, so the saturating answer is written once and the
// conversion is never done unguarded.
func clampCounter(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
}

// enabled is the first question every other field depends on: a session
// without a plan gate has no plan, no policy and no step, and saying so once
// is better than thirty fields each reporting an absence.
func (s PlanState) enabled() Field {
	field := s.field(KeyPlanEnabled, ApplyNewSession, ScopeSession, s.revision())
	if !s.Known {
		return field
	}
	source := sourcePlanSession
	if !s.Enabled {
		source = sourcePlanNoGate
	}
	observation := Present(BoolValue(s.Enabled), source)
	field.Loaded = observation
	field.Effective = observation
	return field
}

// mode names the posture the session is in. Nothing in a configuration file
// sets it — the user enters and leaves plan mode in the session — so the
// configured layer does not exist rather than reporting a default.
func (s PlanState) mode() Field {
	field := s.field(KeyPlanMode, ApplyNextTurn, ScopeSession, s.revision())
	field.Configured = NotApplicable(SourceSession)
	if !s.Known || s.Mode == "" {
		return field
	}
	observation := Present(StringValue(s.Mode), sourcePlanMode)
	field.Loaded = observation
	field.Effective = observation
	return field
}

// phase names the gate this session's calls meet. A session without a gate
// reports unset rather than a phase it is not standing in.
func (s PlanState) phase() Field {
	field := s.field(KeyPlanGatePhase, ApplyNextTurn, ScopeSession, s.revision())
	field.Configured = NotApplicable(SourceSession)
	if !s.Known {
		return field
	}
	observation := Unset(StringValue(""), sourcePlanNoGate)
	if s.GatePhase != "" {
		observation = Present(StringValue(s.GatePhase), sourcePlanPhase)
	}
	field.Loaded = observation
	field.Effective = observation
	return field
}

// policyList publishes one policy list at all three layers. This is where the
// ticket's separation earns its keep: the configured layer is what the
// harness ships, the loaded layer is what configuration compiled, and the
// effective layer is what the model is currently looking at — which is the
// loaded one only until a policy change lands and before the prompt rebinds.
func (s PlanState) policyList(key string, pick func(PlanPolicySet) []string) Field {
	field := s.field(key, ApplyReload, ScopeSession, s.Policy.Revision)
	if !s.Known {
		return field
	}
	field.Configured = policyLayer(s.Policy.Default, pick, sourcePlanBuiltinPolicy)
	field.Loaded = policyLayer(s.Policy.Loaded, pick, sourcePlanLoadedPolicy)
	field.Effective = policyLayer(s.Policy.Projected, pick, sourcePlanProjectedPolicy)
	return field
}

// policyAuthoring is the one policy field that is a name rather than a list.
// An empty selector is a real answer — it means the grammar this harness
// defaults to — so it reports unset with its own provenance rather than
// borrowing the name of a value nobody wrote.
func (s PlanState) policyAuthoring() Field {
	field := s.field(KeyPlanPolicyAuthoring, ApplyReload, ScopeSession, s.Policy.Revision)
	if !s.Known {
		return field
	}
	field.Configured = authoringLayer(s.Policy.Default, sourcePlanBuiltinPolicy)
	field.Loaded = authoringLayer(s.Policy.Loaded, sourcePlanLoadedPolicy)
	field.Effective = authoringLayer(s.Policy.Projected, sourcePlanProjectedPolicy)
	return field
}

// policyLayer reports one list of one policy layer. A layer that does not
// exist says so; an existing layer with an empty list is a present, empty
// answer, because "this policy assigns no tools" is a real policy.
func policyLayer(set PlanPolicySet, pick func(PlanPolicySet) []string, source Source) Observation {
	if !set.Known {
		return Unset(ListValue(nil), sourcePlanNoPolicy)
	}
	return Present(ListValue(pick(set)), source)
}

func authoringLayer(set PlanPolicySet, source Source) Observation {
	if !set.Known {
		return Unset(StringValue(""), sourcePlanNoPolicy)
	}
	if set.Authoring == "" {
		return Unset(StringValue(""), source)
	}
	return Present(StringValue(set.Authoring), source)
}

// lifecycle is the one word the rest of this category hangs off. It is
// derived here rather than by the owner so the six states are decided in one
// place against the same fields the other plan fields report, which is what
// stops a snapshot saying "approved" next to a result.
func (s PlanState) lifecycle() Field {
	field := s.field(KeyPlanLifecycle, ApplyImmediate, ScopeSession, s.revision())
	field.Configured = NotApplicable(SourcePlan)
	if !s.Known {
		return field
	}
	observation := Present(StringValue(s.lifecycleValue()), s.lifecycleSource())
	field.Loaded = observation
	field.Effective = observation
	return field
}

func (s PlanState) lifecycleValue() string {
	switch {
	case !s.Enabled:
		return PlanLifecycleDisabled
	case !s.Present:
		return PlanLifecycleAbsent
	case s.Result != "":
		return PlanLifecycleClosed
	case !s.Approved:
		return PlanLifecycleDraft
	case s.Step.Known:
		return PlanLifecycleActive
	default:
		return PlanLifecycleApproved
	}
}

func (s PlanState) lifecycleSource() Source {
	switch {
	case !s.Enabled:
		return sourcePlanNoGate
	case !s.Present:
		return sourcePlanNoPlan
	default:
		return sourcePlanSnapshot
	}
}

// schema names the authoring contract the stored snapshot was written under.
// No plan means no schema — reporting the current one would describe a file
// that does not exist.
func (s PlanState) schema() Field {
	field := s.field(KeyPlanSchema, ApplyNewSession, ScopeSession, s.revision())
	field.Configured = NotApplicable(SourcePlan)
	if !s.Known {
		return field
	}
	observation := Unset(StringValue(""), sourcePlanNoPlan)
	if s.Present && s.Schema != "" {
		observation = Present(StringValue(s.Schema), sourcePlanSnapshot)
	}
	field.Loaded = observation
	field.Effective = observation
	return field
}

// count publishes one number the plan snapshot carries. Nothing configures
// any of them — they are what the plan grew into — so the configured layer
// does not exist rather than reporting a limit that is not one.
func (s PlanState) count(key string, value int64) Field {
	field := s.field(key, ApplyImmediate, ScopeSession, s.revision())
	field.Configured = NotApplicable(SourcePlan)
	if !s.Known {
		return field
	}
	source := sourcePlanSnapshot
	if !s.Present {
		source = sourcePlanNoPlan
	}
	field.Effective = Present(IntValue(value), source)
	return field
}

// approved is the user's flag and nothing else. It is reported at the loaded
// and effective layers only: no source configures approval, and the harness
// never invents one — an unapproved plan is a plan somebody has not approved.
func (s PlanState) approved() Field {
	field := s.field(KeyPlanApproved, ApplyImmediate, ScopeSession, s.revision())
	field.Configured = NotApplicable(SourceSession)
	if !s.Known {
		return field
	}
	observation := Unset(BoolValue(false), sourcePlanNoPlan)
	if s.Present {
		observation = Present(BoolValue(s.Approved), sourcePlanApproval)
	}
	field.Loaded = observation
	field.Effective = observation
	return field
}

// result says how a finished plan ended. An open plan has no result, and a
// plan that never existed has no result for a different reason; the two
// report the same unset value with different provenance.
func (s PlanState) result() Field {
	field := s.field(KeyPlanResult, ApplyImmediate, ScopeSession, s.revision())
	field.Configured = NotApplicable(SourcePlan)
	if !s.Known {
		return field
	}
	observation := Unset(StringValue(""), sourcePlanNoPlan)
	switch {
	case s.Result != "":
		observation = Present(StringValue(s.Result), sourcePlanSnapshot)
	case s.Present:
		observation = Unset(StringValue(""), sourcePlanOpen)
	}
	field.Loaded = observation
	field.Effective = observation
	return field
}

// statuses is the step breakdown, one entry per status the plan actually
// uses. A status no step stands in is left out rather than reported as zero:
// the list is what the plan is made of, not the vocabulary it could use.
func (s PlanState) statuses() Field {
	field := s.field(KeyPlanStepsStatuses, ApplyImmediate, ScopeSession, s.revision())
	field.Configured = NotApplicable(SourcePlan)
	if !s.Known {
		return field
	}
	items := make([]string, 0, len(s.Statuses))
	for _, entry := range s.Statuses {
		items = append(items, entry.Status+"="+strconv.Itoa(entry.Count))
	}
	source := sourcePlanSnapshot
	if !s.Present {
		source = sourcePlanNoPlan
	}
	field.Effective = Present(ListValue(items), source)
	return field
}

// stepID is the identity a plan_step names, and the only model-authored
// string this category carries. It is sayable because its shape is: a
// lowercase slug bounded at sixty-four characters cannot hold prose.
func (s PlanState) stepID() Field {
	field := s.field(KeyPlanStepCurrent, ApplyImmediate, ScopeStep, s.revision())
	field.Configured = NotApplicable(SourcePlan)
	if !s.Known {
		return field
	}
	observation := Unset(StringValue(""), sourcePlanNoStep)
	if s.Step.Known && s.Step.ID != "" {
		observation = Present(StringValue(s.Step.ID), sourcePlanStep)
	}
	field.Loaded = observation
	field.Effective = observation
	return field
}

func (s PlanState) stepType() Field {
	field := s.field(KeyPlanStepType, ApplyImmediate, ScopeStep, s.revision())
	field.Configured = NotApplicable(SourcePlan)
	if !s.Known {
		return field
	}
	observation := Unset(StringValue(""), sourcePlanNoStep)
	if s.Step.Known && s.Step.Type != "" {
		observation = Present(StringValue(s.Step.Type), sourcePlanStep)
	}
	field.Loaded = observation
	field.Effective = observation
	return field
}

// stepTools is the applied restriction the ticket asks to see next to the
// policy: the loaded layer is what the step's type permits under the
// published policy, and the effective layer is what that comes to right now.
// They part company on an unapproved plan, where the type still permits its
// tools and the gate admits none of them.
func (s PlanState) stepTools() Field {
	field := s.field(KeyPlanStepTools, ApplyImmediate, ScopeStep, s.revision())
	field.Configured = NotApplicable(SourcePlan)
	if !s.Known {
		return field
	}
	if !s.Step.Known {
		observation := Unset(ListValue(nil), sourcePlanNoStep)
		field.Loaded = observation
		field.Effective = observation
		return field
	}
	field.Loaded = Present(ListValue(s.Step.Tools), sourcePlanStepCeiling)
	if !s.Approved {
		field.Effective = Unset(ListValue(nil), sourcePlanStepUnapproved)
		return field
	}
	field.Effective = Present(ListValue(s.Step.Tools), sourcePlanStepCeiling)
	return field
}

// stepJIT separates the demand from the grant: the loaded layer is whether
// this step needs a just-in-time approval at all, and the effective layer is
// whether the user has given one at the current contract epoch. A material
// change to the contract expires a grant, which is exactly the case the two
// layers exist to make visible.
func (s PlanState) stepJIT() Field {
	field := s.field(KeyPlanStepJIT, ApplyImmediate, ScopeStep, s.revision())
	field.Configured = NotApplicable(SourceSession)
	if !s.Known {
		return field
	}
	if !s.Step.Known {
		observation := Unset(BoolValue(false), sourcePlanNoStep)
		field.Loaded = observation
		field.Effective = observation
		return field
	}
	if !s.Step.JIT {
		observation := Present(BoolValue(false), sourcePlanNotJIT)
		field.Loaded = observation
		field.Effective = observation
		return field
	}
	field.Loaded = Present(BoolValue(true), sourcePlanJITWanted)
	source := sourcePlanJITWanted
	if s.Step.JITGranted {
		source = sourcePlanJITGranted
	}
	field.Effective = Present(BoolValue(s.Step.JITGranted), source)
	return field
}

func (s PlanState) stepAttempts() Field {
	field := s.field(KeyPlanStepAttempts, ApplyImmediate, ScopeStep, s.revision())
	field.Configured = NotApplicable(SourcePlan)
	if !s.Known {
		return field
	}
	if !s.Step.Known {
		field.Effective = Unset(IntValue(0), sourcePlanNoStep)
		return field
	}
	field.Effective = Present(IntValue(int64(s.Step.Attempts)), sourcePlanEvidenceCount)
	return field
}

func (s PlanState) stepActions() Field {
	field := s.field(KeyPlanStepActions, ApplyImmediate, ScopeStep, s.revision())
	field.Configured = NotApplicable(SourcePlan)
	if !s.Known {
		return field
	}
	if !s.Step.Known {
		field.Effective = Unset(ListValue(nil), sourcePlanNoStep)
		return field
	}
	field.Effective = Present(ListValue(s.Step.Actions), sourcePlanStep)
	return field
}

// stepSkills answers the skill question the way the ticket asks it: by name
// and application state. Each entry is "name=state", so a skill the step
// names is accounted for whether or not its body was ever loaded, and no
// entry can hold a line of what the skill says.
func (s PlanState) stepSkills() Field {
	field := s.field(KeyPlanStepSkills, ApplyImmediate, ScopeStep, s.revision())
	field.Configured = NotApplicable(SourcePlan)
	if !s.Known {
		return field
	}
	if !s.Step.Known {
		field.Effective = Unset(ListValue(nil), sourcePlanNoStep)
		return field
	}
	items := make([]string, 0, len(s.Step.Skills))
	for _, skill := range s.Step.Skills {
		items = append(items, skill.Name+"="+skill.State)
	}
	field.Effective = Present(ListValue(items), sourcePlanSkillState)
	return field
}

func (s PlanState) skillsPending() Field {
	field := s.field(KeyPlanSkillsPending, ApplyImmediate, ScopeTurn, s.revision())
	field.Configured = NotApplicable(SourceSession)
	if !s.Known {
		return field
	}
	field.Effective = Present(IntValue(int64(s.SkillsPending)), sourcePlanSkillQueue)
	return field
}

// telemetry publishes one group of counters. The groups exist so the plan's
// twenty-four numbers cost five fields rather than twenty-four, and each
// entry is "name=number": the schema behind them has no string member, so
// nothing textual can arrive here however the plan was written.
//
// These fields carry no revision on purpose. The counters are monotonic
// session totals, not a description of a generation of anything, and stamping
// them with the plan's revision would claim a relationship that does not hold.
func (s PlanState) telemetry(key string, items []string) Field {
	field := Field{
		Key:        key,
		Configured: NotApplicable(SourceComputed),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      ApplyImmediate,
		Scope:      ScopeSession,
	}
	if !s.Known {
		return field
	}
	if !s.Telemetry.Known {
		field.Effective = Unset(ListValue(nil), sourcePlanNoTelemetry)
		return field
	}
	field.Effective = Present(ListValue(items), sourcePlanTelemetry)
	return field
}

func (t PlanTelemetry) gate() []string {
	return []string{
		counter("plan_misses", t.Misses),
		counter("transition_conflicts", t.TransitionConflicts),
		counter("idempotent_retries", t.IdempotentRetries),
		counter("standalone_starts", t.StandaloneStarts),
		counter("plan_only_rounds", t.PlanOnlyRounds),
	}
}

func (t PlanTelemetry) approval() []string {
	return []string{
		counter("approval_churn", t.ApprovalChurn),
		counter("material_revisions", t.MaterialRevisions),
		counter("material_reapprovals", t.MaterialReapprovals),
		counter("latency_under_1s", t.ApprovalLatency1s),
		counter("latency_under_10s", t.ApprovalLatency10s),
		counter("latency_under_60s", t.ApprovalLatency60s),
		counter("latency_slow", t.ApprovalLatencySlow),
	}
}

func (t PlanTelemetry) projection() []string {
	return []string{
		counter("injections", t.ProjectionInjections),
		counter("bytes_total", t.ProjectionBytes),
		counter("bytes_last", t.ProjectionBytesLast),
	}
}

func (t PlanTelemetry) completion() []string {
	return []string{
		counter("success", t.CompletionsSuccess),
		counter("abandoned", t.CompletionsAbandoned),
		counter("without_evidence", t.CompletionsWithoutEvidence),
		counter("archives", t.Archives),
		counter("archive_last_ms", t.ArchiveLatencyLastMS),
		counter("archive_max_ms", t.ArchiveLatencyMaxMS),
	}
}

func (t PlanTelemetry) authoring() []string {
	return []string{
		counter("drafts_adaptive", t.DraftsAdaptive),
		counter("drafts_legacy", t.DraftsLegacy),
		counter("patch_retries", t.PatchRetries),
	}
}

// counter renders one telemetry entry. The name is a literal from this file
// and the value is a number, so the rendered string is bounded by
// construction rather than by the registry having to cut it.
func counter(name string, value uint64) string {
	return name + "=" + strconv.FormatUint(value, 10)
}

// revision is the plan's own generation marker: the snapshot revision and the
// contract epoch together, because a plan can be revised without the contract
// moving and the two answer different questions. A session with no plan
// carries the zero generation rather than an empty revision that would read
// as "not known".
func (s PlanState) revision() string {
	if !s.Known {
		return ""
	}
	return "r" + strconv.FormatUint(s.Revision, 10) + ".e" + strconv.FormatUint(s.ContractEpoch, 10)
}

// field is the shape every plan field starts from: all three layers
// unavailable, so a state the owner could not observe degrades into an honest
// answer rather than into a value nobody produced. It reads nothing off the
// state — the caller has already decided what this field's generation is —
// so the receiver is here to keep the nineteen call sites reading as one.
func (PlanState) field(key string, apply Apply, scope Scope, revision string) Field {
	return Field{
		Key:        key,
		Configured: Unavailable(),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      apply,
		Scope:      scope,
		Revision:   revision,
	}
}

// callPlanState reads the optional accessor. A nil accessor is a wiring gap,
// and every field it feeds reports unavailable rather than crashing the
// snapshot.
func callPlanState(accessor func() PlanState) PlanState {
	if accessor == nil {
		return PlanState{}
	}
	return accessor()
}
