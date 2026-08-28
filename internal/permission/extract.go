package permission

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// Extract builds a permission Request from a tool name and raw JSON args.
// Paths are absolute and cleaned against the process cwd.
func Extract(toolName string, args json.RawMessage) (Request, error) {
	return ExtractAt(toolName, args, "")
}

// ExtractAt is Extract with an explicit cwd for relative paths (session / job WorkDir).
func ExtractAt(toolName string, args json.RawMessage, cwd string) (Request, error) {
	req := Request{Tool: toolName}
	switch toolName {
	case "bash":
		var in struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("bash args: %w", err)
		}
		req.Action = ActionBash
		req.Command = strings.TrimSpace(in.Command)
		return req, nil

	case "read":
		var in struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("read args: %w", err)
		}
		req.Action = ActionRead
		return withPath(req, in.Path, cwd)

	case "write":
		var in struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("write args: %w", err)
		}
		req.Action = ActionWrite
		return withPath(req, in.Path, cwd)

	case "edit":
		var in tooldef.PathArgs
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("edit args: %w", err)
		}
		req.Action = ActionEdit
		return withPath(req, in.Resolved(), cwd)

	case "grep":
		var in struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("grep args: %w", err)
		}
		if in.Path == "" {
			in.Path = "."
		}
		req.Action = ActionGrep
		return withPath(req, in.Path, cwd)

	case "find":
		var in struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("find args: %w", err)
		}
		if in.Path == "" {
			in.Path = "."
		}
		req.Action = ActionFind
		return withPath(req, in.Path, cwd)

	case "ls":
		// Accept object or plain string path.
		var asString string
		if err := json.Unmarshal(args, &asString); err == nil && asString != "" {
			req.Action = ActionLs
			return withPath(req, asString, cwd)
		}
		var in struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("ls args: %w", err)
		}
		req.Action = ActionLs
		return withPath(req, in.Path, cwd)

	case "agent_spawn", "agent_wait", "agent_list", "agent_cancel":
		// No path-bearing arguments for the gate to vet: spawn workdir
		// confinement is validated at job.Spawn against the parent workspace.
		req.Action = ActionAgent
		return req, nil

	case "context":
		// Usage report + own-context compaction: no path-bearing arguments
		// and no external effects for the gate to vet.
		req.Action = ActionContext
		return req, nil

	case "plan":
		// Session-local structured state only. Tool-side validation and
		// revision checks protect its integrity.
		req.Action = ActionPlan
		return req, nil

	case "lsp":
		// Read-only code intelligence. Only file-bearing operations carry a
		// path for the read policy to vet; languages carries none.
		var in struct {
			File *string `json:"file"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("lsp args: %w", err)
		}
		req.Action = ActionLSP
		if in.File == nil || strings.TrimSpace(*in.File) == "" {
			return req, nil
		}
		return withPath(req, *in.File, cwd)

	case "memory":
		req.Action = ActionMemory
		return req, nil

	case "watch":
		// Only starting a watch carries a command; list, log and stop address
		// a watch by id and have nothing for the bash policy to judge.
		var in struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("watch args: %w", err)
		}
		req.Action = ActionWatch
		req.Command = strings.TrimSpace(in.Command)
		return req, nil

	case "question":
		// The question tool is itself the ask: it renders the model's prompt
		// to the user and returns their choice, so the gate's own Ask would
		// only put an approval overlay in front of the question.
		req.Action = ActionQuestion
		return req, nil

	case "mcp_list":
		req.Action = ActionMCPList
		return req, nil

	case "mcp_inspect":
		req.Action = ActionMCPInspect
		return req, nil

	case "mcp_call":
		var in struct {
			Server string `json:"server"`
			Tool   string `json:"tool"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("mcp_call args: %w", err)
		}
		req.Action = ActionMCPCall
		if in.Server != "" {
			req.Target = in.Server + "/" + in.Tool
		}
		return req, nil

	default:
		req.Action = Action(toolName)
		return req, nil
	}
}

func withPath(req Request, path, cwd string) (Request, error) {
	abs, err := AbsCleanAt(strings.TrimSpace(path), cwd)
	if err != nil {
		return req, err
	}
	req.Paths = []string{abs}
	return req, nil
}

// Summarize returns a short human-readable summary of the request for UI.
func Summarize(req Request) string {
	switch {
	case req.Command != "":
		return truncate(req.Command, 200)
	case req.Target != "":
		return req.Target
	case len(req.Paths) > 0:
		return strings.Join(req.Paths, ", ")
	default:
		return string(req.Action)
	}
}
