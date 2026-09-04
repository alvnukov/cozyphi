// Package plangate decides whether a tool call may run against the current
// durable plan, and renders the bounded projection of that plan the model
// sees. It is the single deep module for plan→tool gating: the rule (which
// tools a step's type permits), the phase (hint vs deny), and the miss log
// all live here so the executor and prompt stay thin.
package plangate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/redact"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// Phase selects how a miss is handled.
type Phase string

const (
	// PhaseHint allows the tool and appends guidance to the model result.
	PhaseHint Phase = "hint"
	// PhaseDeny blocks the tool with the miss reason.
	PhaseDeny Phase = "deny"
)

// ReasonPlanNotApproved is the deny reason for a gateable tool call while
// the durable plan has not been approved. The controller keys its resume
// logic on this string: approving hands control back to a blocked turn.
const ReasonPlanNotApproved = "the plan is not approved"

// ReasonSkillPreload opens the refusal that delivers a plan step's freshly
// loaded skills to the model: the call is not executed and the model retries
// it at once. The refusal is service choreography, not a failure — the
// transcript keys on this prefix to keep the row out of the feed.
const ReasonSkillPreload = "Plan step started and its skills are preloaded below. " +
	"This tool was not executed; retry the working call now after applying them."

// ReasonBatchSkillPreload is the same refusal for the calls behind the one
// that started the step: the batch stops so the retry runs under the new
// guidance.
const ReasonBatchSkillPreload = "A prior call in this tool batch started a plan step and preloaded its skills. " +
	"This tool was not executed; retry it in the next round after applying that guidance."

// IsSkillPreloadRefusal reports whether the run is the service refusal that
// delivered a plan step's skills. The model retries the same call right
// after, and the executed action already leaves its own "⚙ plan" row, so
// nothing true is lost when the transcript hides this one.
func IsSkillPreloadRefusal(run session.ToolRun) bool {
	if run.Status != session.ToolRejected {
		return false
	}
	return strings.HasPrefix(run.Error, ReasonSkillPreload) || run.Error == ReasonBatchSkillPreload
}

// exemptTools never require plan_step, and they pass the gate even while the
// durable plan is unapproved: they are how the model reads and repairs the
// plan itself (plan, context), asks the user (question), and the utility
// tools that must stay usable at any point while a plan is active (watch,
// memory, task).
var exemptTools = map[string]struct{}{
	"plan":     {},
	"context":  {},
	"question": {},
	"watch":    {},
	"memory":   {},
	"task":     {},
}

// IsExempt reports whether a tool never requires plan_step and so never
// clears the plan-resume-pending signal: it is how the model reads and
// repairs the plan itself, plus the utility tools that must stay usable at
// any point while a plan is active.
func IsExempt(name string) bool {
	_, ok := exemptTools[name]
	return ok
}

// toolLevel ranks tools by how much capability they need; the policy's type
// order supplies the step ranks. A tool is allowed when its level <= the step's.
var toolLevel = map[string]int{
	"read":         1,
	"grep":         1,
	"find":         1,
	"ls":           1,
	"lsp":          1,
	"write":        2,
	"edit":         2,
	"bash":         3,
	"agent_spawn":  4,
	"agent_wait":   4,
	"agent_list":   4,
	"agent_cancel": 4,
	"mcp_list":     5,
	"mcp_inspect":  5,
	"mcp_call":     5,
}

// ToolCall is the invocation under review.
type ToolCall struct {
	Name string
	Step StepRef // the step the call claims to advance
}

// Verdict is the outcome of Check.
type Verdict struct {
	Miss   bool
	Reason string
	Hint   string
	Deny   bool
	// StepID names the stable id of the step this call advances — where its
	// attempt evidence must land. Empty when the gate resolved no step:
	// exempt tools, unapproved pass-through, or a legacy plan whose steps
	// carry no ids.
	StepID string
	// StartPending reports that StepID was still pending: the harness
	// transitions it to in_progress after every gate has cleared, before
	// dispatch.
	StartPending bool
	// Note is model-facing guidance that rides a passing call (no miss): the
	// legacy numeric plan_step deprecation.
	Note string
	// JIT names the just-in-time approval the resolved step still requires:
	// the harness must hand the user this demand after every other gate
	// cleared and before dispatch. Nil for the calls that need nothing.
	JIT *JITDemand
}

// JITDemand is the user approval a call on a just-in-time step still needs.
// The gate resolved the step and its type permits the tool, but the step is
// marked irreversible and the user has not approved it at the current
// contract epoch. A demand is a handoff, not a miss: the model did nothing
// wrong, so it never rides a deny or the miss log.
type JITDemand struct {
	StepID string
	Action string
	Risk   string
}

// Question renders the human-facing approval request: exactly the step, its
// action and its declared risk — no model context, no tool secrets. The
// action and risk are model-authored, so they ride as quoted data (a newline
// or an imperative sentence cannot forge an extra line of the question) and
// stay masked in depth.
func (d JITDemand) Question() string {
	q := fmt.Sprintf("Plan step %q is marked just-in-time.\nAction: %q", d.StepID, redact.Redact(d.Action))
	if d.Risk != "" {
		q += fmt.Sprintf("\nRisk: %q", redact.Redact(d.Risk))
	}
	return q
}

// Rejected renders the deny reason for the transcript and the model: the
// same step, action and risk the user saw, plus their feedback when they
// left any. A step without a declared risk says so instead of implying none.
func (d JITDemand) Rejected(feedback string) string {
	risk := d.Risk
	if risk == "" {
		risk = "none declared"
	}
	reason := fmt.Sprintf(
		"just-in-time approval denied for plan step %q (action: %q; risk: %q)",
		d.StepID,
		redact.Redact(d.Action),
		redact.Redact(risk),
	)
	if feedback != "" {
		reason += "; user feedback: " + feedback
	}
	return reason
}

// Checker applies the plan gate to tool calls.
type Checker struct {
	Phase    Phase
	Recorder *Recorder // nil = no logging
	Policy   *Policy   // nil = built-in defaults
	Runtime  *Runtime  // when set, read once at the start of each check
}

// Check is pure: it never writes. Approved plans require every gateable tool
// to name an active plan step whose type permits that tool; unapproved plans
// and exempt tools pass untouched.
func (c Checker) Check(plan session.Plan, call ToolCall) Verdict {
	policy := c.Policy
	if c.Runtime != nil {
		policy = c.Runtime.Current()
	}
	return policy.Check(c.Phase, plan, call)
}

// Miss is one recorded model misstep, appended as JSONL for offline analysis.
type Miss struct {
	Timestamp    string `json:"timestamp"`
	Session      string `json:"session,omitempty"`
	Tool         string `json:"tool"`
	PlanStep     int    `json:"plan_step,omitempty"` // legacy numeric input
	StepID       string `json:"stepId,omitempty"`
	PlanRevision uint64 `json:"plan_revision,omitempty"`
	StepStatus   string `json:"step_status,omitempty"`
	StepType     string `json:"step_type,omitempty"`
	Reason       string `json:"reason"`
	Phase        string `json:"phase"`
}

// Recorder appends misses to plan-gate-misses.jsonl under a directory.
// The file is opened lazily on the first Record so constructing a checker
// has no filesystem side effects.
type Recorder struct {
	dir  string
	mu   sync.Mutex
	file *os.File
	err  error
}

// NewRecorder returns a lazy recorder for dir.
func NewRecorder(dir string) (*Recorder, error) {
	if dir == "" {
		return nil, errors.New("plangate: empty log dir")
	}
	return &Recorder{dir: dir}, nil
}

// Record writes one miss line, opening the log on first use. Failures are
// returned but callers treat them as best-effort: gating correctness must
// never depend on the log being writable.
func (r *Recorder) Record(m Miss) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil && r.err == nil {
		if err := os.MkdirAll(r.dir, 0o755); err != nil {
			r.err = err
			return err
		}
		f, err := os.OpenFile(
			filepath.Join(r.dir, "plan-gate-misses.jsonl"),
			os.O_CREATE|os.O_WRONLY|os.O_APPEND,
			0o644,
		)
		if err != nil {
			r.err = err
			return err
		}
		r.file = f
	}
	if r.file == nil {
		return r.err
	}
	if m.Timestamp == "" {
		m.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	line, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = r.file.Write(append(line, '\n'))
	return err
}

// DefaultLogDir returns ~/.cozyphi/logs/plan-gate, honoring COZYPHI_PLAN_GATE_LOG_DIR.
func DefaultLogDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("COZYPHI_PLAN_GATE_LOG_DIR")); override != "" {
		//nolint:gosec // G703: COZYPHI_PLAN_GATE_LOG_DIR is an explicit user override
		if err := os.MkdirAll(override, 0o755); err != nil {
			return "", err
		}
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".cozyphi", "logs", "plan-gate")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// NewChecker builds a checker in the given phase, wired to the default miss
// log. A log failure is non-fatal: the checker still works without a recorder.
func NewChecker(phase Phase) *Checker {
	dir, err := DefaultLogDir()
	if err != nil {
		return &Checker{Phase: phase}
	}
	rec, err := NewRecorder(dir)
	if err != nil {
		rec = nil
	}
	return &Checker{Phase: phase, Recorder: rec}
}

// InjectPlanStep returns a copy using the built-in policy.
func InjectPlanStep(ts []tooldef.Tool) []tooldef.Tool {
	return defaultPolicy.InjectPlanStep(ts)
}

// InjectPlanStep adds plan_step to tools gated by this policy and to
// additionally-exempted work tools, where the binding is voluntary. Mandatory
// exemptions never carry it: the utilities owning them are not work tools.
// On gated tools the property is required: providers sample tool arguments
// against this schema, and an optional plan_step is a property the sampler
// drops at will — the gate would then miss a step the model did name.
func (p *Policy) InjectPlanStep(ts []tooldef.Tool) []tooldef.Tool {
	if p == nil {
		p = defaultPolicy
	}
	out := make([]tooldef.Tool, len(ts))
	for i, t := range ts {
		out[i] = t
		if _, mandatory := exemptTools[t.Definition.Name]; mandatory {
			continue
		}
		if t.Definition.Params == nil {
			t.Definition.Params = &llm.FunctionParameters{Type: "object"}
		}
		props := t.Definition.Params.Properties
		if props == nil {
			props = llm.Object{}
		}
		if _, exists := props["plan_step"]; !exists {
			props["plan_step"] = llm.Object{
				"type":        "string",
				"description": "Stable id of the plan step this call advances; call plan with action get to list current ids. A pending compatible step starts automatically; on exempt tools the binding is voluntary; numeric step numbers are deprecated.",
			}
		}
		out[i].Definition.Params.Properties = props
		if _, voluntary := p.exempt[t.Definition.Name]; voluntary {
			continue
		}
		if !slices.Contains(t.Definition.Params.Required, "plan_step") {
			t.Definition.Params.Required = append(t.Definition.Params.Required, "plan_step")
		}
	}
	return out
}

// PromptBlock renders the built-in plan-gate contract.
func PromptBlock(phase Phase) string {
	return defaultPolicy.PromptBlock(phase)
}

// PromptBlock renders this policy's plan-gate contract and type hierarchy.
func (p *Policy) PromptBlock(phase Phase) string {
	if p == nil {
		p = defaultPolicy
	}
	var rows strings.Builder
	allowed := make([]string, 0, len(p.minimumRank))
	for _, typ := range p.defaults.Types {
		allowed = append(allowed, typ.Tools...)
		fmt.Fprintf(&rows, "- %s: %s\n", typ.Name, strings.Join(allowed, ", "))
	}
	if len(p.defaults.Types) == 0 {
		rows.WriteString("- none configured: non-empty plans are disabled\n")
	}
	exempt := make([]string, 0, len(p.exempt))
	for name := range p.exempt {
		exempt = append(exempt, name)
	}
	sort.Strings(exempt)
	exemptList := strings.Join(exempt, ", ")

	phaseNote := "a miss is answered with corrective feedback so you can retry correctly"
	unapprovedNote := "gateable tools run and receive plan-gate guidance instead of being blocked"
	if phase == PhaseDeny {
		phaseNote = "a miss blocks the tool and you must retry with a valid plan_step. " +
			"Tools absent from your tool list are the same gate: no step (or approval) permits them yet"
		unapprovedNote = "every gateable tool is blocked; only " + exemptList + " pass"
	}
	return fmt.Sprintf(`# Plan gate

The harness owns the durable plan's revisions, lifecycle, retry ids, audit and
approval; keep them out of tool arguments. plan action get is the authoritative state.

## Draft and approval

Without an approved plan, create the smallest complete contract:
plan {"action":"create","goal":"...","approach":"...","successCriteria":["..."],"steps":[{"id":"stable-slug","content":"...","type":"...","why":"...","doneWhen":"..."}]}
Normally omit step status; create defaults it to pending. Set jit only for an
irreversible effect. After create, tell the user the draft is ready and stop; do
not execute it until the result reports approved:true. While unapproved, %s.

## Step skills

Use the complete step's smallest necessary-and-sufficient set in steps[].skills
or update_step.skills; null or [] clears it, and unknown names fail. Once injected,
selected skills are workflow constraints unless projection says "off":true; never
re-enable a user-disabled skill. Skills grant no tool capabilities. Choose the
least-capable type covering the complete step and its selected workflows.

If step start adds selected skill guidance not yet in context, the harness
withholds the triggering call and later calls in that batch while injecting it.
This is service choreography, not an approach failure. Apply the workflow; reissue
only calls that remain appropriate, with the arguments and ordering it requires.

## Execute

Each non-exempt working tool call needs plan_step: the stable id of the in_progress
step or a compatible pending step. The harness starts a pending
step automatically; never call plan start first. Numeric plan_step is deprecated.
A batch or parallel wrapper has no shared plan
binding: each non-exempt child has plan_step in its own argument object. Two
parallel reads need plan_step twice.

A successful accepted call becomes a bounded attempt; cite call:<callId> in completion
evidenceRefs. To complete the current step and run the next in one round, add this
envelope to the next tool's arguments and set its plan_step to the next id:
"_plan":{"complete":{"stepId":"current-id","outcome":"...","evidenceRefs":["call:<callId>"]}}
The harness atomically validates completion, optional workingContext and auto-start
before dispatch and derives retry identity from the call. _plan is intentionally
absent from tool schemas.

After the final working call, use plan complete with id, outcome, evidence/evidenceRefs and
planResult:"success" (or "abandoned"). Omit mutationId. For plan patch, omit
expected_revision unless intentionally requesting compare-and-swap.

A jit step needs separate just-in-time approval: wait for the harness/user result;
never grant or assume approval yourself. Current gate behavior: %s.

## Recovery and policy

Follow a corrected or blocked result (or action get), then retry with corrected
arguments. Skill-preload choreography follows Step skills above. plan_step names an
in_progress or compatible pending step. These tools never need plan_step: %s. An
exempt work tool still accepts a voluntary plan_step: naming an active step starts
it before dispatch, applies its model pin and step_start actions, and files evidence.

Step type -> allowed tools (later rows include earlier capabilities):
%s`, unapprovedNote, phaseNote, exemptList, rows.String())
}
