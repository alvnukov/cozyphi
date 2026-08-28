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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
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

// exemptTools never require plan_step, and they pass the gate even while the
// durable plan is unapproved: they are how the model reads and repairs the
// plan itself (plan, context), asks the user (question), and the utility
// tools that must stay usable at any point while a plan is active (watch,
// memory).
var exemptTools = map[string]struct{}{
	"plan":     {},
	"context":  {},
	"question": {},
	"watch":    {},
	"memory":   {},
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
// action and its declared risk — no model context, no tool secrets.
func (d JITDemand) Question() string {
	q := fmt.Sprintf("Plan step %q is marked just-in-time.\nAction: %s", d.StepID, d.Action)
	if d.Risk != "" {
		q += "\nRisk: " + d.Risk
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
		"just-in-time approval denied for plan step %q (action: %s; risk: %s)",
		d.StepID,
		d.Action,
		risk,
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

// InjectPlanStep adds plan_step only to tools gated by this policy.
func (p *Policy) InjectPlanStep(ts []tooldef.Tool) []tooldef.Tool {
	if p == nil {
		p = defaultPolicy
	}
	out := make([]tooldef.Tool, len(ts))
	for i, t := range ts {
		out[i] = t
		if _, exempt := p.exempt[t.Definition.Name]; exempt {
			continue
		}
		if t.Definition.Params == nil {
			t.Definition.Params = &llm.FunctionParameters{Type: "object"}
		}
		props := t.Definition.Params.Properties
		if props == nil {
			props = llm.Object{}
		}
		if _, exists := props["plan_step"]; exists {
			continue
		}
		props["plan_step"] = llm.Object{
			"type":        "string",
			"description": "Stable id of the plan step this call advances (the id field in the injected <current-plan> snapshot). A pending step of the right type starts automatically; numeric step numbers are deprecated.",
		}
		out[i].Definition.Params.Properties = props
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

The durable plan is either unapproved (drafting) or approved (executing).
The authoritative current plan is injected on every inference as a <current-plan> snapshot. Replace it with plan {"steps":[...]}; the harness owns the revision.

While the plan is unapproved, %s. Draft or repair the plan with
plan {"steps":[...]}, then stop and tell the user the plan is ready for
approval. Do not keep calling other tools until the injected snapshot reports
approved: true.

Once the plan is approved, every tool call must advance the plan:
pass plan_step — the stable id of the step it advances (the id field in the
injected <current-plan> snapshot; numeric step numbers are deprecated legacy
input). A call may name the in_progress step or a
pending step whose type permits the tool; the harness starts a pending step
for you, so no separate plan call is needed. Every accepted call is recorded
as a bounded attempt on the step it named; cite one as call:<callId> in
complete evidence_refs. On the current phase, %s.

The next working call may also settle the plan in the same round: attach
"_plan" with {"complete": {stepId, outcome, evidence/evidenceRefs/
noEvidenceReason} and/or "workingContext": "..."} alongside the tool's own
arguments. The harness validates the settle, the named step and the tool
arguments before dispatch, then completes the previous step, swaps the
working context and starts the named step in one atomic write that survives
the tool's runtime failure. The settle is idempotent per tool call id; an
invalid "_plan" rejects the whole call. "_plan" appears in no tool schema —
this block is its contract.

Rules:
- plan_step must reference the in_progress step or a pending step of the
  right type; anything else is a miss.
- %s never need plan_step.
- Every non-empty plan step must use one configured type.
- A step marked "jit": true names an irreversible effect and needs
  just-in-time approval: the harness stops its call and asks the user to
  approve that step on its own. Wait for the tool result; approval is not
  yours to give or assume.
- On a miss, read the error, repair the plan with plan {"steps":[...]}, and retry
  with a valid plan_step — never repeat the identical failing call.

Step type → allowed tools:
%s`, unapprovedNote, phaseNote, exemptList, rows.String())
}
