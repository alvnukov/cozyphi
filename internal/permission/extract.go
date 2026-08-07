package permission

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Extract builds a permission Request from a tool name and raw JSON args.
// Paths are absolute and cleaned. Unknown tools return Ask with a generic reason via Action "".
func Extract(toolName string, args json.RawMessage) (Request, error) {
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
		return withPath(req, in.Path)

	case "write":
		var in struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("write args: %w", err)
		}
		req.Action = ActionWrite
		return withPath(req, in.Path)

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
		return withPath(req, path)

	case "grep":
		var in struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(args, &in)
		if in.Path == "" {
			in.Path = "."
		}
		req.Action = ActionGrep
		return withPath(req, in.Path)

	case "glob":
		var in struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(args, &in)
		if in.Path == "" {
			in.Path = "."
		}
		req.Action = ActionGlob
		return withPath(req, in.Path)

	case "list":
		// Accept object or plain string path.
		var asString string
		if err := json.Unmarshal(args, &asString); err == nil && asString != "" {
			req.Action = ActionList
			return withPath(req, asString)
		}
		var in struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("list args: %w", err)
		}
		req.Action = ActionList
		return withPath(req, in.Path)

	case "fetch":
		var in struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("fetch args: %w", err)
		}
		req.Action = ActionFetch
		req.URL = strings.TrimSpace(in.URL)
		return req, nil

	case "agent_spawn", "agent_task", "agent_wait", "agent_list", "agent_log", "agent_cancel":
		req.Action = ActionAgent
		return req, nil

	default:
		req.Action = Action(toolName)
		return req, nil
	}
}

func withPath(req Request, path string) (Request, error) {
	abs, err := AbsClean(strings.TrimSpace(path))
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
	case req.URL != "":
		return truncate(req.URL, 200)
	case len(req.Paths) > 0:
		return strings.Join(req.Paths, ", ")
	default:
		return string(req.Action)
	}
}
