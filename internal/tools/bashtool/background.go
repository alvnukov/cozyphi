package bashtool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/shelltask"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// InteractiveTool gives an interactive session one managed Bash lifecycle.
// The ordinary tool remains synchronous when this capability is not installed.
func InteractiveTool(manager *shelltask.Manager, parentID string) tooldef.Tool {
	tool := BashTool()
	tool.Definition.Description += "\n\nSet run_in_background=true for a long build, test or server. It returns a task ID and output_file immediately. Completion arrives automatically; do not poll. Use shell_task to list/get/stop and read the output_file for details. Background commands without timeout run until stopped or CozyPhi exits; an explicit timeout is preserved. The user can send a running foreground command to the background without restarting it."
	tool.Definition.Params.Properties["timeout"] = llm.Object{
		"type":        "integer",
		"description": "Timeout in seconds, 1-3600. Foreground default: 300; background omission runs until stopped or application exit.",
	}
	tool.Definition.Params.Properties["run_in_background"] = llm.Object{
		"type":        "boolean",
		"description": "Return immediately with a task ID and output_file; completion is delivered automatically.",
	}
	tool.Run = func(ctx context.Context, raw json.RawMessage) (tooldef.Result, error) {
		return runManaged(ctx, manager, parentID, raw)
	}
	return tool
}

func runManaged(
	ctx context.Context,
	manager *shelltask.Manager,
	parentID string,
	raw json.RawMessage,
) (tooldef.Result, error) {
	var in struct {
		Command    string `json:"command"`
		Timeout    *int   `json:"timeout"`
		Background bool   `json:"run_in_background"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return tooldef.Result{}, fmt.Errorf("parse bash arguments: %w", err)
	}
	command := strings.TrimSpace(in.Command)
	if command == "" {
		return tooldef.Result{}, errors.New("empty command")
	}
	timeout := bashDefaultTimeout
	if in.Background {
		timeout = 0
	}
	if in.Timeout != nil {
		if in.Background && (*in.Timeout < 1 || *in.Timeout > 3600) {
			return tooldef.Result{}, errors.New("background bash timeout must be between 1 and 3600 seconds")
		}
		if *in.Timeout > 0 {
			timeout = min(*in.Timeout, 3600)
		}
	}
	spec, err := buildShellSpec(ctx, command)
	if err != nil {
		return tooldef.Result{}, err
	}
	result, err := manager.Run(ctx, shelltask.Request{
		ParentSessionID: parentID, ToolUseID: tooldef.ToolCallID(ctx), Command: command,
		Spec: spec, Timeout: time.Duration(timeout) * time.Second, Background: in.Background,
	})
	if err != nil {
		return tooldef.Result{}, err
	}
	if result.Snapshot.Background {
		snapshot := result.Snapshot
		text := fmt.Sprintf(
			"Background task %s (%s).\noutput_file: %s\nCompletion will arrive automatically; no polling is needed. Use shell_task to inspect or stop it.",
			snapshot.ID,
			snapshot.State,
			snapshot.OutputFile,
		)
		if snapshot.Deadline != nil {
			text += "\nDeadline: " + snapshot.Deadline.Format(time.RFC3339)
		}
		return tooldef.Result{
			Content:    text,
			Detail:     command,
			Output:     text,
			DeliveryID: "shell:" + snapshot.ID + ":started",
		}, nil
	}
	res := result.Process
	content, display := bashReport(res.Output, res.Truncated, res.ExitCode, res.Canceled)
	return tooldef.Result{Content: content, Detail: command, Output: display}, nil
}

// TaskTool controls only this conversation's existing processes. It cannot
// launch a command or promote a foreground command on the model's behalf.
func TaskTool(manager *shelltask.Manager, parentID string) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "shell_task",
			Description: "Inspect or stop an existing background Bash task from this conversation. Completion is delivered automatically; never call this in a polling loop. Read the returned output_file with the read tool for output.",
			Params: &llm.FunctionParameters{Type: "object", Properties: llm.Object{
				"action": llm.Object{"type": "string", "enum": []string{"list", "get", "stop"}},
				"id":     llm.Object{"type": "string", "description": "Task ID for get or stop."},
			}, Required: []string{"action"}},
		},
		Run: func(_ context.Context, raw json.RawMessage) (tooldef.Result, error) {
			var in struct {
				Action   string           `json:"action"`
				ID       string           `json:"id"`
				PlanStep tooldef.PlanStep `json:"plan_step"`
			}
			if err := tooldef.DecodeStrict(raw, &in); err != nil {
				return tooldef.Result{}, fmt.Errorf("shell_task: %w", err)
			}
			if in.Action != "list" && in.Action != "get" && in.Action != "stop" {
				return tooldef.Result{}, errors.New("shell_task: action must be list, get or stop")
			}
			snapshots := manager.List(parentID)
			var output strings.Builder
			for _, snapshot := range snapshots {
				if in.Action != "list" && snapshot.ID != in.ID {
					continue
				}
				if in.Action == "stop" {
					if err := manager.Stop(snapshot.ID); err != nil {
						return tooldef.Result{}, err
					}
					// The snapshot came from List before the stop request; a task that
					// was already terminal there has no process left to stop, and its
					// terminal notification has already fired — promising another one
					// would be a promise nothing keeps.
					if snapshot.State != shelltask.Running {
						return tooldef.Result{
							Content: "Task " + snapshot.ID + " already " + string(
								snapshot.State,
							) + "; no process to stop.",
							Detail: snapshot.ID,
						}, nil
					}
					return tooldef.Result{
						Content: "Stop requested for " + snapshot.ID + ". Completion will confirm that the process exited.",
						Detail:  snapshot.ID,
					}, nil
				}
				// A stopped or still-running process has no exit code of its own;
				// printing one would invent an outcome.
				exitCode := ""
				if snapshot.ExitCode != nil {
					exitCode = fmt.Sprintf(", exit_code=%d", *snapshot.ExitCode)
				}
				fmt.Fprintf(
					&output,
					"%s: %s (background=%t%s)\n%s\noutput_file: %s\n",
					snapshot.ID,
					snapshot.State,
					snapshot.Background,
					exitCode,
					snapshot.Command,
					snapshot.OutputFile,
				)
				if in.Action == "get" {
					tail, err := manager.Output(snapshot.ID, 8000)
					if err != nil {
						return tooldef.Result{}, err
					}
					fmt.Fprintf(
						&output,
						"Output tail (at most 8000 bytes; file truncated=%t):\n%s\n",
						snapshot.Truncated,
						tail,
					)
				}
			}
			if output.Len() == 0 {
				if in.Action != "list" {
					return tooldef.Result{}, errors.New("shell_task: no such task in this conversation")
				}
				output.WriteString("No shell tasks in this conversation.")
			}
			return tooldef.Result{Content: output.String(), Detail: in.Action}, nil
		},
	}
}
