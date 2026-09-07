package permission

import (
	"encoding/json"
	"fmt"
	"net/url"
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

	case "harness":
		// Read-only observation of cozyphi's own configuration. No paths, no
		// command, no mutation: the tool is only registered at all when the
		// user started the process with --developer-mode.
		req.Action = ActionHarness
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

	case "task":
		// The registry is found at startup and addressed by id, so the only
		// thing to judge is whether the call changes a note.
		var in struct {
			Action string `json:"action"`
			ID     string `json:"id"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return req, fmt.Errorf("task args: %w", err)
		}
		req.Target = strings.TrimSpace(in.ID)
		switch strings.ToLower(strings.TrimSpace(in.Action)) {
		case "", "current", "list", "get":
			req.Action = ActionTaskRead
		default:
			req.Action = ActionTaskWrite
		}
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

	case "web":
		return extractWeb(req, args)

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

// extractWeb maps one web call onto what the gate judges: which of the four
// actions it is, what it names (a URL, a query, a doc_id), where it goes on
// the network, and whether page text would reach the model verbatim. A read
// or find reaches no host — the document is already in the cache — so their
// Host stays empty and only the fetch that put it there was egress.
func extractWeb(req Request, args json.RawMessage) (Request, error) {
	var in struct {
		Action   string `json:"action"`
		URL      string `json:"url"`
		Query    string `json:"query"`
		DocID    string `json:"doc_id"`
		Provider string `json:"provider"`
		Raw      bool   `json:"raw"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return req, fmt.Errorf("web args: %w", err)
	}
	req.Action = ActionWeb
	req.Op = strings.ToLower(strings.TrimSpace(in.Action))
	req.Raw = in.Raw
	switch req.Op {
	case "fetch":
		req.Target = strings.TrimSpace(in.URL)
		req.Host = URLHost(req.Target)
	case "search":
		req.Target = strings.TrimSpace(in.Query)
		req.Host = SearchHost(in.Provider)
	default:
		// read and find, and anything the model misspells: named by the
		// document they open, and reaching no host of their own.
		req.Target = strings.TrimSpace(in.DocID)
	}
	return req, nil
}

// URLHost returns the lowercased host a URL reaches, or "" when the string is
// not an absolute URL the fetch could use. It is exported because the gate,
// the ask overlay and the tool must all name the same host: an allow-list
// entry that matched one spelling and a turn record that kept another would
// hand out an approval nobody can see.
func URLHost(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// SearchHost is the egress key of a search. The endpoint is the provider's,
// not the query's, so the provider id is what an allow-list entry and a
// per-turn record can both address.
func SearchHost(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return "search"
	}
	return "search:" + provider
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
