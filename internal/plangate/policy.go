package plangate

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/alvnukov/cozyphi/internal/session"
)

var stepTypeNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// The closed authoring_policy value set: the plan-mode prompt is assembled
// from these and nothing else, so plan.defaults stays an enforcement
// channel, never an instruction channel. The named type carries the
// selector end to end — a bare string cannot reach prompt assembly.
type AuthoringPolicy string

const (
	AuthoringAdaptiveMinimal AuthoringPolicy = "adaptive-minimal"
	AuthoringLegacy          AuthoringPolicy = "legacy"
)

// TypeDefaults names one ordered plan step type and the tools introduced at
// that level. Types are ordered from least to most capable; tools introduced by
// a type are inherited by every type after it.
type TypeDefaults struct {
	Name session.StepType `yaml:"name" json:"name"`
	// Model is the per-type model a newly created plan inherits into its
	// ModelsByType map when its author does not pin one; empty follows the
	// session default.
	Model string   `yaml:"model,omitempty" json:"model,omitempty"`
	Tools []string `yaml:"tools,omitempty" json:"tools,omitempty"`
	// Actions are the step-scope plan actions (step_start / step_end) a new
	// step of this type inherits when its author defines none.
	Actions []session.PlanAction `yaml:"actions,omitempty" json:"actions,omitempty"`
}

// Defaults is the editable, persisted plan-gate policy. AdditionalExemptions
// bypass only this gate; the executor's permission gate still runs.
type Defaults struct {
	Types                []TypeDefaults `yaml:"types"                           json:"types"`
	AdditionalExemptions []string       `yaml:"additional_exemptions,omitempty" json:"additional_exemptions,omitempty"`
	// Actions are the plan-scope plan actions (plan_start / plan_end) a new
	// plan inherits when its author defines none.
	Actions []session.PlanAction `yaml:"actions,omitempty" json:"actions,omitempty"`
	// AuthoringPolicy selects the plan-mode authoring grammar. It is a closed
	// enum (AuthoringAdaptiveMinimal, AuthoringLegacy); empty means the
	// adaptive grammar, so configs written before the selector existed keep
	// the prompt they already ship. It selects prompt text and nothing else.
	AuthoringPolicy AuthoringPolicy `yaml:"authoring_policy,omitempty" json:"authoring_policy,omitempty"`
}

// ModelsByType maps every configured type that carries a model pin to its
// name; types without one are absent so the session default applies.
func (d Defaults) ModelsByType() map[session.StepType]string {
	var pins map[session.StepType]string
	for _, typ := range d.Types {
		if typ.Model == "" {
			continue
		}
		if pins == nil {
			pins = make(map[session.StepType]string, len(d.Types))
		}
		pins[typ.Name] = typ.Model
	}
	return pins
}

// StepTypeNames lists the configured type names in hierarchy order.
func (d Defaults) StepTypeNames() []string {
	names := make([]string, len(d.Types))
	for i, typ := range d.Types {
		names[i] = string(typ.Name)
	}
	return names
}

// DefaultDefaults returns the built-in policy used when config.yaml has no
// plan.defaults section.
func DefaultDefaults() Defaults {
	return Defaults{Types: []TypeDefaults{
		{Name: session.StepExplore, Tools: []string{"read", "grep", "find", "ls", "lsp"}},
		{Name: session.StepEdit, Tools: []string{"write", "edit"}},
		{Name: session.StepRun, Tools: []string{"bash"}},
		{Name: session.StepDelegate, Tools: []string{"agent_spawn", "agent_wait", "agent_list", "agent_cancel"}},
		{Name: session.StepIntegrate, Tools: []string{"mcp_list", "mcp_inspect", "mcp_call"}},
	}}
}

// Runtime publishes immutable policies atomically. A caller that starts a check
// keeps one policy snapshot for that check; Apply affects the next one.
type Runtime struct {
	current atomic.Pointer[Policy]
}

// NewRuntime validates defaults and returns a ready live policy source.
func NewRuntime(defaults Defaults) (*Runtime, error) {
	policy, err := Compile(defaults)
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{}
	runtime.current.Store(policy)
	return runtime, nil
}

// Current returns the currently published immutable policy.
func (r *Runtime) Current() *Policy {
	if r == nil {
		return defaultPolicy
	}
	if policy := r.current.Load(); policy != nil {
		return policy
	}
	return defaultPolicy
}

// Apply validates and atomically publishes defaults.
func (r *Runtime) Apply(defaults Defaults) error {
	if r == nil {
		return errors.New("plangate: nil runtime")
	}
	policy, err := Compile(defaults)
	if err != nil {
		return err
	}
	r.current.Store(policy)
	return nil
}

// ModelsByType exposes the type model pins compiled into this policy.
func (p *Policy) ModelsByType() map[session.StepType]string {
	if p == nil {
		return nil
	}
	return p.defaults.ModelsByType()
}

// PlanActions exposes the plan-level default actions compiled into this
// policy: what a newly created plan inherits when its author defines none.
func (p *Policy) PlanActions() []session.PlanAction {
	if p == nil {
		return nil
	}
	return session.ClonePlanActions(p.defaults.Actions)
}

// AuthoringPolicy reports the compiled authoring-grammar selector. Empty
// and AuthoringAdaptiveMinimal both mean the adaptive grammar;
// AuthoringLegacy asks the plan prompt for its pre-grammar appendix.
func (p *Policy) AuthoringPolicy() AuthoringPolicy {
	if p == nil {
		p = defaultPolicy
	}
	return p.defaults.AuthoringPolicy
}

// ActionsByType maps every configured type carrying step-scope default
// actions to its list; types without one are absent so the session default
// (no actions) applies.
func (p *Policy) ActionsByType() map[session.StepType][]session.PlanAction {
	var actions map[session.StepType][]session.PlanAction
	for _, typ := range p.defaults.Types {
		if len(typ.Actions) == 0 {
			continue
		}
		if actions == nil {
			actions = make(map[session.StepType][]session.PlanAction, len(p.defaults.Types))
		}
		actions[typ.Name] = session.ClonePlanActions(typ.Actions)
	}
	return actions
}

// Policy is an immutable compiled plan-gate policy. It is safe to share across
// goroutines and keeps validation, enforcement, and prompt projection on the
// same source of truth.
type Policy struct {
	defaults    Defaults
	typeRank    map[session.StepType]int
	minimumRank map[string]int
	exempt      map[string]struct{}
}

var defaultPolicy = mustCompile(DefaultDefaults())

func mustCompile(defaults Defaults) *Policy {
	policy, err := Compile(defaults)
	if err != nil {
		panic(err)
	}
	return policy
}

// Compile validates and freezes editable defaults. Tool assignments are
// exclusive additions: assigning one tool at two levels is ambiguous and
// rejected. An omitted tool is denied at every typed step.
func Compile(defaults Defaults) (*Policy, error) {
	planActions, err := session.NormalizePlanDefaultActions(defaults.Actions)
	if err != nil {
		return nil, fmt.Errorf("plangate: plan actions: %w", err)
	}
	authoringPolicy := AuthoringPolicy(strings.TrimSpace(string(defaults.AuthoringPolicy)))
	switch authoringPolicy {
	case "", AuthoringAdaptiveMinimal, AuthoringLegacy:
	default:
		return nil, fmt.Errorf("plangate: invalid authoring_policy %q (allowed: %s, %s)",
			defaults.AuthoringPolicy, AuthoringAdaptiveMinimal, AuthoringLegacy)
	}
	policy := &Policy{
		defaults: Defaults{
			Types:                make([]TypeDefaults, len(defaults.Types)),
			AdditionalExemptions: slices.Clone(defaults.AdditionalExemptions),
			Actions:              planActions,
			AuthoringPolicy:      authoringPolicy,
		},
		typeRank:    make(map[session.StepType]int, len(defaults.Types)),
		minimumRank: make(map[string]int),
		exempt:      make(map[string]struct{}, len(exemptTools)+len(defaults.AdditionalExemptions)),
	}
	for name := range exemptTools {
		policy.exempt[name] = struct{}{}
	}

	for i, typ := range defaults.Types {
		name := session.StepType(strings.TrimSpace(string(typ.Name)))
		if !stepTypeNamePattern.MatchString(string(name)) {
			return nil, fmt.Errorf("plangate: invalid step type %q (use a lowercase slug, 1-32 characters)", typ.Name)
		}
		if _, exists := policy.typeRank[name]; exists {
			return nil, fmt.Errorf("plangate: duplicate step type %q", name)
		}
		policy.typeRank[name] = i + 1
		typeActions, err := session.NormalizeStepDefaultActions(typ.Actions)
		if err != nil {
			return nil, fmt.Errorf("plangate: step type %q: %w", name, err)
		}
		policy.defaults.Types[i] = TypeDefaults{
			Name:    name,
			Model:   strings.TrimSpace(typ.Model),
			Tools:   slices.Clone(typ.Tools),
			Actions: typeActions,
		}
		for _, rawTool := range typ.Tools {
			tool := strings.TrimSpace(rawTool)
			if err := validateAssignableTool(tool); err != nil {
				return nil, fmt.Errorf("plangate: step type %q: %w", name, err)
			}
			if _, exists := policy.minimumRank[tool]; exists {
				return nil, fmt.Errorf("plangate: tool %q is assigned to more than one step type", tool)
			}
			policy.minimumRank[tool] = i + 1
		}
	}

	seenExempt := make(map[string]struct{}, len(defaults.AdditionalExemptions))
	for i, rawTool := range defaults.AdditionalExemptions {
		tool := strings.TrimSpace(rawTool)
		if err := validateAssignableTool(tool); err != nil {
			return nil, fmt.Errorf("plangate: additional exemption: %w", err)
		}
		if _, mandatory := exemptTools[tool]; mandatory {
			return nil, fmt.Errorf("plangate: tool %q is already a mandatory exemption", tool)
		}
		if _, duplicate := seenExempt[tool]; duplicate {
			return nil, fmt.Errorf("plangate: duplicate additional exemption %q", tool)
		}
		if _, assigned := policy.minimumRank[tool]; assigned {
			return nil, fmt.Errorf("plangate: exempt tool %q must not also be assigned to a step type", tool)
		}
		seenExempt[tool] = struct{}{}
		policy.exempt[tool] = struct{}{}
		policy.defaults.AdditionalExemptions[i] = tool
	}
	return policy, nil
}

func validateAssignableTool(tool string) error {
	if tool == "" {
		return errors.New("empty tool name")
	}
	if _, mandatory := exemptTools[tool]; mandatory {
		return fmt.Errorf("mandatory exemption %q cannot be assigned to a step type", tool)
	}
	if _, known := toolLevel[tool]; !known {
		return fmt.Errorf("unknown tool %q", tool)
	}
	return nil
}

// Defaults returns a detached copy suitable for an editable draft.
func (p *Policy) Defaults() Defaults {
	if p == nil {
		p = defaultPolicy
	}
	out := Defaults{AdditionalExemptions: slices.Clone(p.defaults.AdditionalExemptions)}
	out.Actions = session.ClonePlanActions(p.defaults.Actions)
	out.AuthoringPolicy = p.defaults.AuthoringPolicy
	out.Types = make([]TypeDefaults, len(p.defaults.Types))
	for i, typ := range p.defaults.Types {
		out.Types[i] = TypeDefaults{
			Name:    typ.Name,
			Model:   typ.Model,
			Tools:   slices.Clone(typ.Tools),
			Actions: session.ClonePlanActions(typ.Actions),
		}
	}
	return out
}

// StepTypes returns configured machine names in their capability order.
func (p *Policy) StepTypes() []string {
	if p == nil {
		p = defaultPolicy
	}
	names := make([]string, len(p.defaults.Types))
	for i, typ := range p.defaults.Types {
		names[i] = string(typ.Name)
	}
	return names
}

// ValidateItems verifies the configured type contract for newly authored plan
// items: every non-empty step carries a configured type.
func (p *Policy) ValidateItems(items []session.PlanItem) error {
	if p == nil {
		p = defaultPolicy
	}
	if len(items) == 0 {
		return nil
	}
	if len(p.typeRank) == 0 {
		return errors.New("plangate: plan creation is disabled because no step types are configured")
	}
	for i, item := range items {
		if item.Type == "" {
			return fmt.Errorf("plangate: step %d type is required", i+1)
		}
		if _, ok := p.typeRank[item.Type]; !ok {
			return fmt.Errorf("plangate: step %d has unknown step type %q", i+1, item.Type)
		}
	}
	return nil
}

// Check applies this immutable policy to one tool call. On an approved plan a
// gateable tool must name a step whose type permits it: the in_progress step
// it continues, or a still-pending step the harness then starts
// (Verdict.StartPending). The resolved Verdict.StepID is the step the call
// advances, whatever its status. A finished plan is not gate state — its
// contract is discharged — so every call passes through until the plan is
// reopened or replaced.
func (p *Policy) Check(phase Phase, plan session.Plan, call ToolCall) Verdict {
	if p == nil {
		p = defaultPolicy
	}
	if _, ok := p.exempt[call.Name]; ok {
		return exemptBinding(plan, call)
	}
	if plan.Result != "" {
		return Verdict{}
	}
	miss := func(reason, hint string) Verdict {
		verdict := Verdict{Miss: true, Reason: reason, Hint: hint}
		if phase == PhaseDeny {
			verdict.Deny = true
		}
		return verdict
	}
	if !plan.Approved {
		if phase == PhaseDeny {
			return miss(ReasonPlanNotApproved, "Approve the plan (sidebar checkbox) before tools can run.")
		}
		return Verdict{}
	}
	item, ok := call.Step.Find(plan)
	if !ok {
		return miss(
			fmt.Sprintf("plan_step %s is not a valid step in the approved plan", call.Step),
			"Call plan with action get, take the id field of a step, and pass it as plan_step.",
		)
	}
	startPending := false
	switch item.Status {
	case session.PlanInProgress:
	case session.PlanPending:
		startPending = true
	default:
		return miss(
			fmt.Sprintf("plan step %s is %s, not an active step", call.Step, item.Status),
			"Pass plan_step of the in_progress plan item, or of a pending step you are starting.",
		)
	}
	rank, knownType := p.typeRank[item.Type]
	if !knownType {
		return miss(
			fmt.Sprintf("plan step %s has unknown step type %q", call.Step, item.Type),
			"Replace the plan with a configured step type before retrying.",
		)
	}
	minimum, assigned := p.minimumRank[call.Name]
	if !assigned || rank < minimum {
		return miss(
			fmt.Sprintf("tool %q is not allowed on a %s step", call.Name, item.Type),
			fmt.Sprintf(
				"Step %s is typed %s; use a tool that step allows or widen the step type via plan.",
				call.Step,
				item.Type,
			),
		)
	}
	verdict := Verdict{StepID: item.ID, StartPending: startPending}
	// A just-in-time step clears the plan gate only with a user grant at
	// the current contract epoch; the demand rides the verdict for the
	// executor's user handoff and never counts as a miss.
	if item.JIT && !plan.JITGranted(item.ID) {
		verdict.JIT = &JITDemand{StepID: item.ID, Action: item.Content, Risk: item.Risk}
	}
	if call.Step.Ordinal > 0 {
		verdict.Note = legacyStepNote
	}
	return verdict
}

// exemptBinding resolves the plan_step of an exempt tool. Exemption lifts the
// requirement, never the pass: a binding that names an active step rides the
// verdict — the harness starts the step before dispatch and files the call's
// evidence there — while a binding that does not resolve is guidance for the
// model, never a miss or a deny. No type check applies: the tool already
// passed by exemption, and a step only receives work it can interpret.
func exemptBinding(plan session.Plan, call ToolCall) Verdict {
	if call.Step.ID == "" && call.Step.Ordinal <= 0 {
		return Verdict{}
	}
	if plan.Result != "" || !plan.Approved {
		return Verdict{}
	}
	item, ok := call.Step.Find(plan)
	if !ok || (item.Status != session.PlanPending && item.Status != session.PlanInProgress) {
		return Verdict{Note: fmt.Sprintf(
			"plan_step %s on exempt tool %q does not name an active step; the call ran unbound. "+
				"Call plan with action get to list current ids.",
			call.Step, call.Name,
		)}
	}
	verdict := Verdict{StepID: item.ID, StartPending: item.Status == session.PlanPending}
	if call.Step.Ordinal > 0 {
		verdict.Note = legacyStepNote
	}
	return verdict
}

// VisibleTools returns the tool set a provider may see for this plan state:
// the exempt tools, plus every gateable tool whose minimum level is met by at
// least one pending or in_progress step of a known type — pending steps are
// startable by naming them, so the first call of a step must already see its
// tools. An unapproved plan narrows the answer to the exempt set — mirroring
// Check's deny-phase semantics, so the tool list a provider sees never
// promises more than the gate allows. A finished plan narrows the same way:
// all its steps are terminal, so nothing typed is startable.
func (p *Policy) VisibleTools(plan session.Plan) map[string]struct{} {
	if p == nil {
		p = defaultPolicy
	}
	visible := make(map[string]struct{}, len(p.exempt))
	for name := range p.exempt {
		visible[name] = struct{}{}
	}
	if !plan.Approved {
		return visible
	}
	if plan.Result != "" {
		return visible
	}
	for _, item := range plan.Items {
		rank, known := p.typeRank[item.Type]
		if !known || (item.Status != session.PlanInProgress && item.Status != session.PlanPending) {
			continue
		}
		for name, minimum := range p.minimumRank {
			if minimum <= rank {
				visible[name] = struct{}{}
			}
		}
	}
	return visible
}
