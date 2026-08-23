package permission

import (
	"encoding/json"
	"fmt"
	"strings"
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
		var in struct {
			Path     string `json:"path"`
			FilePath string `json:"file_path"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("edit args: %w", err)
		}
		path := in.Path
		if path == "" {
			path = in.FilePath
		}
		req.Action = ActionEdit
		return withPath(req, path, cwd)

	case "grep":
		var in struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(args, &in)
		if in.Path == "" {
			in.Path = "."
		}
		req.Action = ActionGrep
		return withPath(req, in.Path, cwd)

	case "find":
		var in struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(args, &in)
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
	case len(req.Paths) > 0:
		return strings.Join(req.Paths, ", ")
	default:
		return string(req.Action)
	}
}
