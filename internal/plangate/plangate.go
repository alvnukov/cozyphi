// Package plangate decides whether a tool call may run against the current
// durable plan. It is the single deep module for plan→tool gating: the rule
// (which tools a step's type permits), the phase (hint vs deny), and the miss
// log all live here so the executor and prompt stay thin.
package plangate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tools/tooldef"
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

// exemptTools never require plan_step: they are how the model reads and
// repairs the plan itself.
var exemptTools = map[string]struct{}{
	"plan":    {},
	"context": {},
}

// IsExempt reports whether a tool never requires plan_step: it is how the
// model reads and repairs the plan itself.
func IsExempt(name string) bool {
	_, ok := exemptTools[name]
	return ok
}

// toolLevel ranks tools by how much capability they need; stepLevel ranks
// step types the same way. A tool is allowed when its level <= the step's.
var toolLevel = map[string]int{
	"read":         1,
	"grep":         1,
	"find":         1,
	"ls":           1,
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

var stepLevel = map[session.StepType]int{
	session.StepExplore:   1,
	session.StepEdit:      2,
	session.StepRun:       3,
	session.StepDelegate:  4,
	session.StepIntegrate: 5,
}

// ToolCall is the invocation under review.
type ToolCall struct {
	Name     string
	PlanStep int // 1-based index into Plan.Items; 0 means omitted
}

// Verdict is the outcome of Check.
type Verdict struct {
	Miss   bool
	Reason string
	Hint   string
	Deny   bool
}

// Checker applies the plan gate to tool calls.
type Checker struct {
	Phase    Phase
	Recorder *Recorder // nil = no logging
}

// Check is pure: it never writes. Approved plans require every gateable tool
// to name an active plan step whose type permits that tool; unapproved plans
// and exempt tools pass untouched.
func (c Checker) Check(plan session.Plan, call ToolCall) Verdict {
	if _, ok := exemptTools[call.Name]; ok {
		return Verdict{}
	}

	miss := func(reason, hint string) Verdict {
		v := Verdict{Miss: true, Reason: reason, Hint: hint}
		if c.Phase == PhaseDeny {
			v.Deny = true
		}
		return v
	}
	if !plan.Approved {
		// Unapproved plans hold the model only in Deny phase: every gateable
		// tool is blocked so the model stops. Hint phase leaves the plan alone.
		if c.Phase == PhaseDeny {
			return miss(
				ReasonPlanNotApproved,
				"Approve the plan (sidebar checkbox) before tools can run.",
			)
		}
		return Verdict{}
	}

	if call.PlanStep <= 0 || call.PlanStep > len(plan.Items) {
		return miss(
			fmt.Sprintf("plan_step %d is not a valid step in the approved plan", call.PlanStep),
			"Call plan with action=get, find the active step number, and pass it as plan_step.",
		)
	}
	item := plan.Items[call.PlanStep-1]
	if item.Status != session.PlanInProgress {
		return miss(
			fmt.Sprintf("plan step %d is %s, not an active step", call.PlanStep, item.Status),
			"Pass plan_step of the in_progress plan item.",
		)
	}
	if item.Type == "" {
		// Legacy untyped steps permit any tool.
		return Verdict{}
	}
	if toolLevel[call.Name] > stepLevel[item.Type] || toolLevel[call.Name] == 0 {
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

// Miss is one recorded model misstep, appended as JSONL for offline analysis.
type Miss struct {
	Timestamp    string `json:"timestamp"`
	Session      string `json:"session,omitempty"`
	Tool         string `json:"tool"`
	PlanStep     int    `json:"plan_step,omitempty"`
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

// DefaultLogDir returns ~/.phi/logs/plan-gate, honoring PHI_PLAN_GATE_LOG_DIR.
func DefaultLogDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PHI_PLAN_GATE_LOG_DIR")); override != "" {
		//nolint:gosec // G703: PHI_PLAN_GATE_LOG_DIR is an explicit user override
		if err := os.MkdirAll(override, 0o755); err != nil {
			return "", err
		}
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".phi", "logs", "plan-gate")
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

// InjectPlanStep returns a copy of ts with a plan_step integer parameter
// added to every gateable tool schema. Exempt tools keep their schema so the
// model can always repair the plan.
func InjectPlanStep(ts []tooldef.Tool) []tooldef.Tool {
	out := make([]tooldef.Tool, len(ts))
	for i, t := range ts {
		out[i] = t
		if _, exempt := exemptTools[t.Definition.Name]; exempt {
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
			"type":        "integer",
			"minimum":     1,
			"description": "1-based step number in the approved plan this call advances; required when the plan is approved.",
		}
		out[i].Definition.Params.Properties = props
	}
	return out
}

// PromptBlock renders the plan-gate contract and type→tool table for the
// system prompt. It is generated from the same maps Check uses, so the prompt
// cannot drift from the enforcement.
func PromptBlock(phase Phase) string {
	order := []session.StepType{
		session.StepExplore, session.StepEdit, session.StepRun, session.StepDelegate, session.StepIntegrate,
	}
	labels := map[session.StepType]string{
		session.StepExplore:   "explore",
		session.StepEdit:      "edit",
		session.StepRun:       "run",
		session.StepDelegate:  "delegate",
		session.StepIntegrate: "integrate",
	}
	names := map[session.StepType]string{
		session.StepExplore:   "read, grep, find, ls",
		session.StepEdit:      "explore tools + write, edit",
		session.StepRun:       "edit tools + bash",
		session.StepDelegate:  "run tools + agent_spawn/agent_wait/agent_list/agent_cancel",
		session.StepIntegrate: "delegate tools + mcp_list/mcp_inspect/mcp_call",
	}
	var rows strings.Builder
	for _, typ := range order {
		fmt.Fprintf(&rows, "- %s: %s\n", labels[typ], names[typ])
	}
	phaseNote := "a miss is answered with corrective feedback so you can retry correctly"
	if phase == PhaseDeny {
		phaseNote = "a miss blocks the tool and you must retry with a valid plan_step"
	}
	return fmt.Sprintf(`# Plan gate

When the durable plan is approved, every tool call must advance the plan:
pass plan_step (the 1-based number of the active step) in the tool arguments.
On the current phase, %s.

Rules:
- plan_step must reference an in_progress step; otherwise the call is a miss.
- plan and context tools never need plan_step.
- Steps may omit their type; untyped steps allow any tool.

Step type → allowed tools:
%s`, phaseNote, rows.String())
}
