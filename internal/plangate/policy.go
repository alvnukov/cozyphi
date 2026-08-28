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

// TypeDefaults names one ordered plan step type and the tools introduced at
// that level. Types are ordered from least to most capable; tools introduced by
// a type are inherited by every type after it.
type TypeDefaults struct {
	Name  session.StepType `yaml:"name"            json:"name"`
	Tools []string         `yaml:"tools,omitempty" json:"tools,omitempty"`
}

// Defaults is the editable, persisted plan-gate policy. AdditionalExemptions
// bypass only this gate; the executor's permission gate still runs.
type Defaults struct {
	Types                []TypeDefaults `yaml:"types"                           json:"types"`
	AdditionalExemptions []string       `yaml:"additional_exemptions,omitempty" json:"additional_exemptions,omitempty"`
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
	policy := &Policy{
		defaults: Defaults{
			Types:                make([]TypeDefaults, len(defaults.Types)),
			AdditionalExemptions: slices.Clone(defaults.AdditionalExemptions),
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
		policy.defaults.Types[i] = TypeDefaults{Name: name, Tools: slices.Clone(typ.Tools)}
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
	out.Types = make([]TypeDefaults, len(p.defaults.Types))
	for i, typ := range p.defaults.Types {
		out.Types[i] = TypeDefaults{Name: typ.Name, Tools: slices.Clone(typ.Tools)}
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

// Check applies this immutable policy to one tool call.
func (p *Policy) Check(phase Phase, plan session.Plan, call ToolCall) Verdict {
	if p == nil {
		p = defaultPolicy
	}
	if _, ok := p.exempt[call.Name]; ok {
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
	if call.PlanStep <= 0 || call.PlanStep > len(plan.Items) {
		return miss(
			fmt.Sprintf("plan_step %d is not a valid step in the approved plan", call.PlanStep),
			"Use the injected <current-plan> snapshot to find the active in_progress step, then pass it as plan_step.",
		)
	}
	item := plan.Items[call.PlanStep-1]
	if item.Status != session.PlanInProgress {
		return miss(
			fmt.Sprintf("plan step %d is %s, not an active step", call.PlanStep, item.Status),
			"Pass plan_step of the in_progress plan item.",
		)
	}
	rank, knownType := p.typeRank[item.Type]
	if !knownType {
		return miss(
			fmt.Sprintf("plan step %d has unknown step type %q", call.PlanStep, item.Type),
			"Replace the plan with a configured step type before retrying.",
		)
	}
	minimum, assigned := p.minimumRank[call.Name]
	if !assigned || rank < minimum {
		return miss(
			fmt.Sprintf("tool %q is not allowed on a %s step", call.Name, item.Type),
			fmt.Sprintf(
				"Step %d is typed %s; use a tool that step allows or widen the step type via plan.",
				call.PlanStep,
				item.Type,
			),
		)
	}
	return Verdict{}
}

// VisibleTools returns the tool set a provider may see for this plan state:
// the exempt tools, plus every gateable tool whose minimum level is met by at
// least one in_progress step of a known type. An unapproved plan narrows the
// answer to the exempt set — mirroring Check's deny-phase semantics, so the
// tool list a provider sees never promises more than the gate allows.
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
	for _, item := range plan.Items {
		rank, known := p.typeRank[item.Type]
		if item.Status != session.PlanInProgress || !known {
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
