package agenttool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alvnukov/cozyphi/internal/tools/tooldef"

	"github.com/alvnukov/cozyphi/internal/job"

	"github.com/alvnukov/cozyphi/internal/llm"
)

const agentSummaryLimit = 12000 // bytes, keep parent context small

const agentLaunchGuidance = `Launch a specialized sub-agent. Pick a role:

- explore (default): read-only search/structure (tools: bash, read, grep, ls, find). Use when a keyword/file search is uncertain or would take many find/grep rounds.
- review: read-only + bash for diffs/checks — report findings, do not edit.
- worker: may read and write — only after you have planned an independent change block; do not use for open-ended exploration.

When NOT to use any sub-agent:
- You already know the exact file path — use read yourself
- Exact symbol like "class Foo" — use grep yourself
- Small local edit — edit/write yourself
- Prefer explore over worker unless the task is explicitly to implement a scoped change

How to use:
1. Use agent_spawn to launch a job. Interactive sessions receive terminal outcomes automatically; use agent_wait only for an explicit dependency barrier. Headless callers must use agent_wait for results. For parallel jobs, spawn all first.
2. Skills are an explicit decision on every spawn: pass via skills the installed skills that fit the sub-task — the sub-agent gets exactly those, nothing inherited. If no installed skill fits, pass skills: [] with no_skill_reason saying why (the user sees it), and suggest creating the skill or finding one online.
3. Give every new child a self-contained prompt and specify the final summary. Interactive child conversations are retained for human follow-ups; each follow-up is a linked assignment with its own job_id.
4. You only receive the final summary. Summarize for the user if needed.
5. Sub-agents cannot spawn further agents. Do not put secrets in the prompt.
6. Verify before relying on a worker's edits in follow-up work.`

// AgentDeps wires sub-agent tools to a process-level [job.Manager].
// ParentID/WorkDir are read at call time (session may change via /resume).
type AgentDeps struct {
	OwnerID  string // immutable assignment-owner scope; empty preserves legacy global access
	Manager  *job.Manager
	ParentID func() string
	WorkDir  func() string
	// Spawn optionally binds execution to a caller-owned runner snapshot.
	// It must use Manager's admission path; nil defaults to Manager.Spawn.
	Spawn func(context.Context, job.SpawnRequest) (job.Info, error)
	// ModelForRole names the agents.models pin for a role so the spawn
	// result can show it; nil or ok=false means inherit-the-session-model.
	ModelForRole func(job.Role) (string, bool)
	// SkillPath names the installed-skill catalog spawn validation resolves
	// `skills` names against; nil or empty means no catalog is installed, so
	// only `skills: []` with a no_skill_reason can pass.
	SkillPath func() string
}

// InheritModel is the spawn-result model value for a child that runs the
// session's active model: no agents.models pin, or one whose name no longer
// resolves.
const InheritModel = "inherit"

// AgentTools returns agent_spawn / list / wait / cancel.
// Depth is forced to 0; ParentID comes from ParentID(), not model args.
func AgentTools(deps AgentDeps) []tooldef.Tool {
	if deps.Manager == nil {
		return nil
	}
	if deps.Spawn == nil {
		deps.Spawn = deps.Manager.Spawn
	}
	if deps.ParentID == nil {
		deps.ParentID = func() string { return "" }
	}
	if deps.WorkDir == nil {
		deps.WorkDir = func() string { return "" }
	}
	if deps.ModelForRole == nil {
		deps.ModelForRole = func(job.Role) (string, bool) { return "", false }
	}
	if deps.SkillPath == nil {
		deps.SkillPath = func() string { return "" }
	}
	return []tooldef.Tool{
		agentSpawnTool(deps),
		agentListTool(deps),
		agentWaitTool(deps),
		agentCancelTool(deps),
	}
}

func agentSpawnTool(deps AgentDeps) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name: "agent_spawn",
			Description: agentLaunchGuidance + `

Starts asynchronously and returns job_id immediately. Interactive sessions receive the outcome without waiting; headless callers use agent_wait. Best for parallel jobs.`,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"prompt": llm.Object{
						"type":        "string",
						"description": "Self-contained task. Include context, scope, and exactly what the final summary must return. The sub-agent cannot ask follow-ups.",
					},
					"description": llm.Object{
						"type":        "string",
						"description": "Very short label for the UI / job list (e.g. \"find auth config\").",
					},
					"role": llm.Object{
						"type":        "string",
						"description": "explore (default) | review | worker. See tool description for when to pick each.",
						"enum":        []string{"explore", "review", "worker"},
					},
					"effort": llm.Object{
						"type":        "string",
						"description": "Optional reasoning effort override for this child only. Must be supported by the user-configured role/session model; omitted or empty inherits its effort. Model selection is user-controlled.",
						"enum":        []string{"", "none", "minimal", "low", "medium", "high", "xhigh", "max"},
					},
					"skills": llm.Object{
						"type":        "array",
						"items":       llm.Object{"type": "string"},
						"description": "Explicit skill decision: the exact installed skill names this sub-agent gets (case-insensitive, at most 8, duplicates fold). Empty array requires no_skill_reason.",
					},
					"no_skill_reason": llm.Object{
						"type":        "string",
						"description": "Why no installed skill fits this sub-task; shown to the user. Required when skills is empty or omitted.",
					},
					"workdir": llm.Object{
						"type":        "string",
						"description": "Working directory for the sub-agent (default: parent session cwd). Must resolve inside the parent workspace.",
					},
					"timeout_sec": llm.Object{
						"type":        "integer",
						"description": "Optional run timeout in seconds for the job itself (not wait).",
					},
				},
				Required: []string{"prompt", "skills"},
			},
		},
		DetailFromArgs: spawnDetail,
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			in, err := parseSpawnInput(input)
			if err != nil {
				return tooldef.Result{}, err
			}
			role, err := job.ParseRole(in.Role)
			if err != nil {
				return tooldef.Result{}, err
			}
			// The skills decision is explicit on every spawn: names must resolve
			// against the installed catalog, or the call fails saying what to pass.
			skillNames := []string{}
			if len(in.Skills) > 0 {
				skillNames, err = resolveSpawnSkills(deps.SkillPath(), in.Skills)
				if err != nil {
					return tooldef.Result{}, err
				}
			} else if strings.TrimSpace(in.NoSkillReason) == "" {
				return tooldef.Result{}, fmt.Errorf(
					"agent_spawn: skills is required — pass the installed skill names that fit the sub-task (at most %d), "+
						"or skills: [] with no_skill_reason saying why none fits (the user sees it)",
					maxSpawnSkills,
				)
			}
			req := job.SpawnRequest{
				Prompt:          in.Prompt,
				Description:     in.Description,
				ParentID:        deps.ParentID(),
				OwnerID:         deps.OwnerID,
				ParentToolUseID: tooldef.ToolCallID(ctx),
				Depth:           0,
				Role:            role,
				Effort:          in.Effort,
				// WorkDir stays raw: Manager.Spawn resolves it against the
				// parent workspace and rejects escapes before any job exists.
				WorkDir:         strings.TrimSpace(in.WorkDir),
				ParentWorkspace: deps.WorkDir(),
				Skills:          skillNames,
			}
			if in.TimeoutSec > 0 {
				req.Timeout = time.Duration(in.TimeoutSec) * time.Second
			}
			info, err := deps.Spawn(ctx, req)
			if err != nil {
				return tooldef.Result{}, err
			}
			// The pin is a pure config lookup: naming it here matches what the
			// runner resolves when it builds the child, without racing it.
			model := InheritModel
			if name, ok := deps.ModelForRole(role); ok && name != "" {
				model = name
			}
			out := map[string]any{
				"job_id":      info.ID,
				"status":      info.Status,
				"role":        info.Role,
				"model":       model,
				"skills":      skillNames,
				"dir":         info.Dir,
				"result_path": info.ResultPath,
			}
			if reason := strings.TrimSpace(in.NoSkillReason); reason != "" {
				out["no_skill_reason"] = reason
			}
			body := mustJSON(out)
			return tooldef.Result{Content: body, Detail: info.ID, Output: body}, nil
		},
	}
}

type spawnInput struct {
	Prompt        string          `json:"prompt"`
	Description   string          `json:"description"`
	Role          string          `json:"role"`
	Effort        string          `json:"effort"`
	Model         json.RawMessage `json:"model"`
	Skills        []string        `json:"skills"`
	NoSkillReason string          `json:"no_skill_reason"`
	WorkDir       string          `json:"workdir"`
	TimeoutSec    int             `json:"timeout_sec"`
}

func parseSpawnInput(input json.RawMessage) (spawnInput, error) {
	var in spawnInput
	if err := json.Unmarshal(input, &in); err != nil {
		return spawnInput{}, err
	}
	// Reject the obsolete selector explicitly, without changing how other
	// unknown fields (including depth and parent_id) are ignored.
	if in.Model != nil {
		return spawnInput{}, errors.New(
			"agent_spawn: model selection is user-controlled; remove model and use effort to adjust reasoning depth",
		)
	}
	effort, ok := llm.ParseReasoningEffort(in.Effort)
	if !ok {
		return spawnInput{}, fmt.Errorf(
			"agent_spawn: invalid effort %q; use none, minimal, low, medium, high, xhigh, max, or omit effort to inherit",
			in.Effort,
		)
	}
	in.Effort = string(effort)
	return in, nil
}

// SpawnTitleFromInput names a sub-agent the way every surface names it: the
// role it runs as, then the short description in parentheses. One format for
// the spawn row and for the outcome row a resumed session stands in its
// place, so the same child reads the same everywhere. It takes the spawn call
// arguments because that is what both surfaces have: the transcript row when
// it is made, and the history when it is replayed. A child with nothing to
// describe itself by leaves the role alone rather than inventing empty
// parentheses.
func SpawnTitleFromInput(input json.RawMessage) string {
	var in struct {
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
		Role        string `json:"role"`
	}
	_ = json.Unmarshal(input, &in)
	description := strings.TrimSpace(in.Description)
	if description == "" {
		description = truncateRunes(in.Prompt, 80)
	}
	role := string(job.NormalizeRole(in.Role))
	if description == "" {
		return role
	}
	return role + "(" + description + ")"
}

func spawnDetail(input json.RawMessage) string {
	var in struct {
		Skills        []string `json:"skills"`
		NoSkillReason string   `json:"no_skill_reason"`
	}
	_ = json.Unmarshal(input, &in)
	label := SpawnTitleFromInput(input)
	// The skills decision rides the row for the user: the names when the
	// child got some, the reason (kept short) when it deliberately got none.
	reason := strings.TrimSpace(in.NoSkillReason)
	switch {
	case len(in.Skills) > 0:
		label += " · skills: " + strings.Join(in.Skills, ", ")
	case reason != "":
		label += " · no skills: " + truncateRunes(reason, 60)
	}
	return label
}

func agentListTool(deps AgentDeps) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "agent_list",
			Description: `List sub-agent jobs (newest first). Each row includes status; filter client-side if needed.`,
			Params: &llm.FunctionParameters{
				Type:       "object",
				Properties: llm.Object{},
			},
		},
		Run: func(ctx context.Context, _ json.RawMessage) (tooldef.Result, error) {
			list, err := deps.Manager.ListForOwner(ctx, deps.OwnerID)
			if err != nil {
				return tooldef.Result{}, err
			}
			rows := make([]map[string]any, 0, len(list))
			for _, info := range list {
				rows = append(rows, map[string]any{
					"job_id":      info.ID,
					"status":      info.Status,
					"role":        info.Role,
					"description": info.Description,
					"dir":         info.Dir,
				})
			}
			body := mustJSON(map[string]any{"jobs": rows, "count": len(rows)})
			return tooldef.Result{Content: body, Detail: fmt.Sprintf("%d jobs", len(rows)), Output: body}, nil
		},
	}
}

func agentWaitTool(deps AgentDeps) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name: "agent_wait",
			Description: `Block until a sub-agent job reaches a terminal status and return its result.md summary.

timeout_sec only limits how long this wait blocks — it does NOT cancel the job.
Use agent_cancel to stop a running job.`,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"job_id": llm.Object{
						"type":        "string",
						"description": "Job id from agent_spawn.",
					},
					"timeout_sec": llm.Object{
						"type":        "integer",
						"description": "Max seconds to wait (does not cancel the job).",
					},
				},
				Required: []string{"job_id"},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in struct {
				JobID string `json:"job_id"`
			}
			_ = json.Unmarshal(input, &in)
			return in.JobID
		},
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			var args job.WaitArgs
			if err := json.Unmarshal(input, &args); err != nil {
				return tooldef.Result{}, fmt.Errorf("%w: %w", job.ErrInvalid, err)
			}
			if args.JobID == "" {
				return tooldef.Result{}, fmt.Errorf("%w: job_id is required", job.ErrInvalid)
			}
			if args.TimeoutSec > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Duration(args.TimeoutSec)*time.Second)
				defer cancel()
			}
			res, err := deps.Manager.WaitForOwner(ctx, args.JobID, deps.OwnerID)
			if err != nil {
				return tooldef.Result{}, err
			}
			summary := truncateBytes(res.Summary, agentSummaryLimit)
			body := mustJSON(map[string]any{
				"job_id":      res.Info.ID,
				"outcome_id":  res.Info.OutcomeID,
				"status":      res.Info.Status,
				"role":        res.Info.Role,
				"error":       res.Info.Error,
				"result_path": res.Info.ResultPath,
				"summary":     summary,
			})
			deliveryID := ""
			if res.Outcome != nil {
				// A receipt suppresses push delivery, so serialize its entire envelope first.
				body = mustJSON(struct {
					job.Outcome
					OutcomeID string   `json:"outcome_id"`
					Role      job.Role `json:"role"`
				}{Outcome: *res.Outcome, OutcomeID: res.Outcome.EventID, Role: res.Info.Role})
				deliveryID = res.Outcome.EventID
			}
			return tooldef.Result{
				Content:    body,
				Detail:     string(res.Info.Status),
				Output:     body,
				DeliveryID: deliveryID,
			}, nil
		},
	}
}

func agentCancelTool(deps AgentDeps) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "agent_cancel",
			Description: `Cancel a running or starting sub-agent job and wait until it stops.`,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"job_id": llm.Object{
						"type": "string",
					},
				},
				Required: []string{"job_id"},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in struct {
				JobID string `json:"job_id"`
			}
			_ = json.Unmarshal(input, &in)
			return in.JobID
		},
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			var args job.CancelArgs
			if err := json.Unmarshal(input, &args); err != nil {
				return tooldef.Result{}, fmt.Errorf("%w: %w", job.ErrInvalid, err)
			}
			if args.JobID == "" {
				return tooldef.Result{}, fmt.Errorf("%w: job_id is required", job.ErrInvalid)
			}
			if err := deps.Manager.CancelForOwner(ctx, args.JobID, deps.OwnerID); err != nil {
				return tooldef.Result{}, err
			}
			body := mustJSON(map[string]any{"ok": true})
			return tooldef.Result{Content: body, Detail: "cancelled", Output: body}, nil
		},
	}
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func truncateBytes(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "\n…(truncated)"
}

func truncateRunes(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	var b strings.Builder
	i := 0
	for _, r := range s {
		if i >= n {
			break
		}
		b.WriteRune(r)
		i++
	}
	b.WriteString("…")
	return b.String()
}
