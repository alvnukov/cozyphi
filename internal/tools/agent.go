package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/llm"
)

const agentSummaryLimit = 12000 // bytes, keep parent context small

// AgentDeps wires sub-agent tools to a process-level [job.Manager].
// ParentID/WorkDir are read at call time (session may change via /resume).
type AgentDeps struct {
	Manager  *job.Manager
	ParentID func() string
	WorkDir  func() string
}

// AgentTools returns agent_spawn / list / wait / log / cancel.
// Depth is forced to 0; ParentID comes from ParentID(), not model args.
func AgentTools(deps AgentDeps) []Tool {
	if deps.Manager == nil {
		return nil
	}
	if deps.ParentID == nil {
		deps.ParentID = func() string { return "" }
	}
	if deps.WorkDir == nil {
		deps.WorkDir = func() string { return "" }
	}
	return []Tool{
		agentSpawnTool(deps),
		agentTaskTool(deps),
		agentListTool(deps),
		agentWaitTool(deps),
		agentLogTool(deps),
		agentCancelTool(deps),
	}
}

func agentSpawnTool(deps AgentDeps) Tool {
	return Tool{
		Definition: llm.ToolDefinition{
			Name: "agent_spawn",
			Description: `Start an isolated sub-agent job asynchronously.

Use for parallel exploration or long subtasks whose full transcript must not
enter this conversation. Returns a job_id immediately; use agent_wait for the
summary (result.md). Sub-agents cannot spawn further agents.

Do not put secrets in the prompt.`,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"prompt": llm.Object{
						"type":        "string",
						"description": "Full task instructions for the sub-agent (include paths, acceptance criteria).",
					},
					"description": llm.Object{
						"type":        "string",
						"description": "Short label shown in job metadata / UI.",
					},
					"workdir": llm.Object{
						"type":        "string",
						"description": "Working directory for the sub-agent (default: parent session cwd).",
					},
					"timeout_sec": llm.Object{
						"type":        "integer",
						"description": "Optional run timeout in seconds for the job itself (not wait).",
					},
				},
				Required: []string{"prompt"},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in struct {
				Description string `json:"description"`
				Prompt      string `json:"prompt"`
			}
			_ = json.Unmarshal(input, &in)
			if in.Description != "" {
				return in.Description
			}
			return truncateRunes(in.Prompt, 80)
		},
		Run: func(ctx context.Context, input json.RawMessage) (Result, error) {
			var in struct {
				Prompt      string `json:"prompt"`
				Description string `json:"description"`
				WorkDir     string `json:"workdir"`
				TimeoutSec  int    `json:"timeout_sec"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return Result{}, err
			}
			wd := strings.TrimSpace(in.WorkDir)
			if wd == "" {
				wd = deps.WorkDir()
			}
			req := job.SpawnRequest{
				Prompt:          in.Prompt,
				Description:     in.Description,
				ParentID:        deps.ParentID(),
				ParentToolUseID: ToolCallID(ctx),
				Depth:           0, // tool layer hard-stop; children have no agent_* tools
				WorkDir:         wd,
			}
			if in.TimeoutSec > 0 {
				req.Timeout = time.Duration(in.TimeoutSec) * time.Second
			}
			info, err := deps.Manager.Spawn(ctx, req)
			if err != nil {
				return Result{}, err
			}
			body := mustJSON(map[string]any{
				"job_id":      info.ID,
				"status":      info.Status,
				"dir":         info.Dir,
				"result_path": info.ResultPath,
			})
			return Result{Content: body, Detail: info.ID, Output: body}, nil
		},
	}
}

func agentTaskTool(deps AgentDeps) Tool {
	return Tool{
		Definition: llm.ToolDefinition{
			Name: "agent_task",
			Description: `Run an isolated sub-agent to completion and return its summary in one call.

Equivalent to agent_spawn followed by agent_wait. Prefer this for a single
blocking subtask. Prefer agent_spawn + agent_wait when you need multiple
sub-agents in parallel.`,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"prompt": llm.Object{
						"type":        "string",
						"description": "Full task instructions for the sub-agent.",
					},
					"description": llm.Object{
						"type":        "string",
						"description": "Short label for job metadata.",
					},
					"workdir": llm.Object{
						"type":        "string",
						"description": "Working directory (default: parent session cwd).",
					},
					"timeout_sec": llm.Object{
						"type":        "integer",
						"description": "Optional run timeout in seconds for the job.",
					},
				},
				Required: []string{"prompt"},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in struct {
				Description string `json:"description"`
				Prompt      string `json:"prompt"`
			}
			_ = json.Unmarshal(input, &in)
			if in.Description != "" {
				return in.Description
			}
			return truncateRunes(in.Prompt, 80)
		},
		Run: func(ctx context.Context, input json.RawMessage) (Result, error) {
			var in struct {
				Prompt      string `json:"prompt"`
				Description string `json:"description"`
				WorkDir     string `json:"workdir"`
				TimeoutSec  int    `json:"timeout_sec"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return Result{}, err
			}
			wd := strings.TrimSpace(in.WorkDir)
			if wd == "" {
				wd = deps.WorkDir()
			}
			req := job.SpawnRequest{
				Prompt:          in.Prompt,
				Description:     in.Description,
				ParentID:        deps.ParentID(),
				ParentToolUseID: ToolCallID(ctx),
				Depth:           0,
				WorkDir:         wd,
			}
			if in.TimeoutSec > 0 {
				req.Timeout = time.Duration(in.TimeoutSec) * time.Second
			}
			res, err := deps.Manager.Task(ctx, req)
			if err != nil && res.Info.ID == "" {
				return Result{}, err
			}
			summary := truncateBytes(res.Summary, agentSummaryLimit)
			body := mustJSON(map[string]any{
				"job_id":      res.Info.ID,
				"status":      res.Info.Status,
				"error":       res.Info.Error,
				"result_path": res.Info.ResultPath,
				"summary":     summary,
			})
			if err != nil {
				return Result{Content: body, Detail: string(res.Info.Status), Output: body}, err
			}
			return Result{Content: body, Detail: string(res.Info.Status), Output: body}, nil
		},
	}
}

func agentListTool(deps AgentDeps) Tool {
	return Tool{
		Definition: llm.ToolDefinition{
			Name:        "agent_list",
			Description: `List sub-agent jobs (newest first). Optional status filter: starting, running, completed, failed, cancelled, timed_out.`,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"status": llm.Object{
						"type":        "string",
						"description": "Optional status filter.",
					},
				},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in struct {
				Status string `json:"status"`
			}
			_ = json.Unmarshal(input, &in)
			return in.Status
		},
		Run: func(ctx context.Context, input json.RawMessage) (Result, error) {
			if len(input) == 0 {
				input = json.RawMessage(`{}`)
			}
			list, err := deps.Manager.HandleList(ctx, input)
			if err != nil {
				return Result{}, err
			}
			rows := make([]map[string]any, 0, len(list))
			for _, info := range list {
				rows = append(rows, map[string]any{
					"job_id":      info.ID,
					"status":      info.Status,
					"description": info.Description,
					"dir":         info.Dir,
				})
			}
			body := mustJSON(map[string]any{"jobs": rows, "count": len(rows)})
			return Result{Content: body, Detail: fmt.Sprintf("%d jobs", len(rows)), Output: body}, nil
		},
	}
}

func agentWaitTool(deps AgentDeps) Tool {
	return Tool{
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
		Run: func(ctx context.Context, input json.RawMessage) (Result, error) {
			res, err := deps.Manager.HandleWait(ctx, input)
			if err != nil {
				return Result{}, err
			}
			summary := truncateBytes(res.Summary, agentSummaryLimit)
			body := mustJSON(map[string]any{
				"job_id":      res.Info.ID,
				"status":      res.Info.Status,
				"error":       res.Info.Error,
				"result_path": res.Info.ResultPath,
				"summary":     summary,
			})
			return Result{Content: body, Detail: string(res.Info.Status), Output: body}, nil
		},
	}
}

func agentLogTool(deps AgentDeps) Tool {
	return Tool{
		Definition: llm.ToolDefinition{
			Name:        "agent_log",
			Description: `Read the last N log lines from a sub-agent job (events.jsonl).`,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"job_id": llm.Object{
						"type": "string",
					},
					"limit": llm.Object{
						"type":        "integer",
						"description": "Max lines from the end (0 = all).",
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
		Run: func(ctx context.Context, input json.RawMessage) (Result, error) {
			events, err := deps.Manager.HandleLog(ctx, input)
			if err != nil {
				return Result{}, err
			}
			lines := make([]string, 0, len(events))
			for _, ev := range events {
				lines = append(lines, ev.Message)
			}
			body := mustJSON(map[string]any{"events": lines, "count": len(lines)})
			return Result{Content: body, Detail: fmt.Sprintf("%d lines", len(lines)), Output: body}, nil
		},
	}
}

func agentCancelTool(deps AgentDeps) Tool {
	return Tool{
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
		Run: func(ctx context.Context, input json.RawMessage) (Result, error) {
			if err := deps.Manager.HandleCancel(ctx, input); err != nil {
				return Result{}, err
			}
			body := mustJSON(map[string]any{"ok": true})
			return Result{Content: body, Detail: "cancelled", Output: body}, nil
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
