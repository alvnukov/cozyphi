package agenttool

import (
	"context"
	"encoding/json"
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
1. Use agent_spawn to launch a job, then agent_wait to block for its summary. For parallel jobs, spawn all first, then wait each.
2. Stateless: put a highly detailed, self-contained prompt and say what the final summary must include.
3. You only receive the final summary. Summarize for the user if needed.
4. Sub-agents cannot spawn further agents. Do not put secrets in the prompt.
5. Verify before relying on a worker's edits in follow-up work.`

// AgentDeps wires sub-agent tools to a process-level [job.Manager].
// ParentID/WorkDir are read at call time (session may change via /resume).
type AgentDeps struct {
	Manager  *job.Manager
	ParentID func() string
	WorkDir  func() string
	// ModelForRole names the agents.models pin for a role so the spawn
	// result can show it; nil or ok=false means inherit-the-session-model.
	ModelForRole func(job.Role) (string, bool)
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
	if deps.ParentID == nil {
		deps.ParentID = func() string { return "" }
	}
	if deps.WorkDir == nil {
		deps.WorkDir = func() string { return "" }
	}
	if deps.ModelForRole == nil {
		deps.ModelForRole = func(job.Role) (string, bool) { return "", false }
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

Starts asynchronously and returns job_id immediately. Use agent_wait for the summary. Best for parallel jobs.`,
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
					"workdir": llm.Object{
						"type":        "string",
						"description": "Working directory for the sub-agent (default: parent session cwd). Must resolve inside the parent workspace.",
					},
					"timeout_sec": llm.Object{
						"type":        "integer",
						"description": "Optional run timeout in seconds for the job itself (not wait).",
					},
				},
				Required: []string{"prompt"},
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
			req := job.SpawnRequest{
				Prompt:          in.Prompt,
				Description:     in.Description,
				ParentID:        deps.ParentID(),
				ParentToolUseID: tooldef.ToolCallID(ctx),
				Depth:           0,
				Role:            role,
				// WorkDir stays raw: Manager.Spawn resolves it against the
				// parent workspace and rejects escapes before any job exists.
				WorkDir:         strings.TrimSpace(in.WorkDir),
				ParentWorkspace: deps.WorkDir(),
			}
			if in.TimeoutSec > 0 {
				req.Timeout = time.Duration(in.TimeoutSec) * time.Second
			}
			info, err := deps.Manager.Spawn(ctx, req)
			if err != nil {
				return tooldef.Result{}, err
			}
			// The pin is a pure config lookup: naming it here matches what the
			// runner resolves when it builds the child, without racing it.
			model := InheritModel
			if name, ok := deps.ModelForRole(role); ok && name != "" {
				model = name
			}
			body := mustJSON(map[string]any{
				"job_id":      info.ID,
				"status":      info.Status,
				"role":        info.Role,
				"model":       model,
				"dir":         info.Dir,
				"result_path": info.ResultPath,
			})
			return tooldef.Result{Content: body, Detail: info.ID, Output: body}, nil
		},
	}
}

type spawnInput struct {
	Prompt      string `json:"prompt"`
	Description string `json:"description"`
	Role        string `json:"role"`
	WorkDir     string `json:"workdir"`
	TimeoutSec  int    `json:"timeout_sec"`
}

func parseSpawnInput(input json.RawMessage) (spawnInput, error) {
	var in spawnInput
	if err := json.Unmarshal(input, &in); err != nil {
		return spawnInput{}, err
	}
	return in, nil
}

func spawnDetail(input json.RawMessage) string {
	var in struct {
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
		Role        string `json:"role"`
	}
	_ = json.Unmarshal(input, &in)
	label := in.Description
	if label == "" {
		label = truncateRunes(in.Prompt, 80)
	}
	if r := strings.TrimSpace(in.Role); r != "" && r != "explore" {
		return r + ": " + label
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
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			list, err := deps.Manager.HandleList(ctx, input)
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
			res, err := deps.Manager.HandleWait(ctx, input)
			if err != nil {
				return tooldef.Result{}, err
			}
			summary := truncateBytes(res.Summary, agentSummaryLimit)
			body := mustJSON(map[string]any{
				"job_id":      res.Info.ID,
				"status":      res.Info.Status,
				"role":        res.Info.Role,
				"error":       res.Info.Error,
				"result_path": res.Info.ResultPath,
				"summary":     summary,
			})
			return tooldef.Result{Content: body, Detail: string(res.Info.Status), Output: body}, nil
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
			if err := deps.Manager.HandleCancel(ctx, input); err != nil {
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
