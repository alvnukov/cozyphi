package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// MaxHookOutputBytes caps stdout/stderr collected from a command hook.
const MaxHookOutputBytes = 1 << 20 // 1 MiB

// ExitDeny is the hard-deny exit code from an external hook (even with empty body).
const ExitDeny = 2

// CommandHook runs an external executable as a Hook via stdin/stdout JSON.
// It does not go through a shell: RunPath is executed directly.
type CommandHook struct {
	name    string
	kind    Kind
	match   string
	runPath string
	dir     string
	timeout time.Duration
}

// NewCommandHook builds a CommandHook from a discovered manifest.
func NewCommandHook(d Discovered) *CommandHook {
	timeout := d.Manifest.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &CommandHook{
		name:    d.Manifest.Name,
		kind:    d.Manifest.Kind,
		match:   d.Manifest.Match,
		runPath: d.RunPath,
		dir:     d.Manifest.Dir,
		timeout: timeout,
	}
}

// EntryFromDiscovered wraps a discovered hook as a Manager Entry
// (Kind / FailClosed / Async come from the manifest).
func EntryFromDiscovered(d Discovered) Entry {
	return Entry{
		Hook:       NewCommandHook(d),
		Kind:       d.Manifest.Kind,
		FailClosed: d.Manifest.FailClosed,
		Async:      d.Manifest.Async,
	}
}

// EntriesFromDiscovered converts discovery results into Manager entries.
func EntriesFromDiscovered(ds []Discovered) []Entry {
	out := make([]Entry, 0, len(ds))
	for _, d := range ds {
		out = append(out, EntryFromDiscovered(d))
	}
	return out
}

// Name returns the hook's configured name.
func (h *CommandHook) Name() string { return h.name }

// Match reports whether this hook applies to the given tool name.
func (h *CommandHook) Match(tool string) bool {
	if h.match == "" || h.match == "*" {
		return true
	}
	return tool == h.match
}

// PreTool runs the hook before a tool executes; hooks of other kinds are skipped.
func (h *CommandHook) PreTool(ctx context.Context, ev Event) (PreResult, error) {
	if h.kind != KindPreTool {
		return PreResult{Action: ActionAllow}, nil
	}
	return h.runPre(ctx, ev)
}

// PostTool runs the hook after a tool executes; hooks of other kinds are skipped.
func (h *CommandHook) PostTool(ctx context.Context, ev Event) (PostResult, error) {
	if h.kind != KindPostTool {
		return PostResult{}, nil
	}
	return h.runPost(ctx, ev)
}

// Command runs the hook as a TUI slash command; other kinds return a no-op.
func (h *CommandHook) Command(ctx context.Context, ev CommandEvent) (CommandResult, error) {
	if h.kind != KindCommand {
		return CommandResult{}, nil
	}
	return h.runCommand(ctx, ev)
}

// Session runs a session lifecycle hook; other kinds return allow.
func (h *CommandHook) Session(ctx context.Context, ev SessionEvent) (SessionResult, error) {
	switch h.kind {
	case KindSessionStart, KindSessionShutdown, KindSessionBeforeSwitch:
		if h.kind != ev.Kind {
			return SessionResult{Action: ActionAllow}, nil
		}
		return h.runSession(ctx, ev)
	default:
		return SessionResult{Action: ActionAllow}, nil
	}
}

type wireIn struct {
	SessionID         string          `json:"session_id"`
	Cwd               string          `json:"cwd"`
	HookEvent         string          `json:"hook_event"`
	Tool              string          `json:"tool"`
	ToolUseID         string          `json:"tool_use_id"`
	Input             json.RawMessage `json:"input"`
	Output            string          `json:"output,omitempty"`
	Err               string          `json:"error,omitempty"`
	Command           string          `json:"command,omitempty"`
	Args              []string        `json:"args,omitempty"`
	Reason            string          `json:"reason,omitempty"`
	PreviousSessionID string          `json:"previous_session_id,omitempty"`
	TargetSessionID   string          `json:"target_session_id,omitempty"`
}

type wirePreOut struct {
	Action  string          `json:"action"`
	Input   json.RawMessage `json:"input"`
	Reason  string          `json:"reason"`
	Context string          `json:"context"`
}

type wirePostOut struct {
	Context string `json:"context"`
	Stop    bool   `json:"stop"`
	Reason  string `json:"reason"`
	Output  string `json:"output"`
}

type wireCommandOut struct {
	Submit string       `json:"submit"`
	Toast  string       `json:"toast"`
	Reason string       `json:"reason"`
	Status *string      `json:"status"`
	List   *CommandList `json:"list"`
}

type wireSessionOut struct {
	Action string  `json:"action"`
	Reason string  `json:"reason"`
	Toast  string  `json:"toast"`
	Status *string `json:"status"`
}

func (h *CommandHook) runPre(ctx context.Context, ev Event) (PreResult, error) {
	stdout, code, err := h.invoke(ctx, KindPreTool, ev)
	if err != nil {
		return PreResult{}, err
	}
	if code == ExitDeny {
		res := PreResult{Action: ActionDeny, Reason: "hook denied (exit 2)"}
		if line := firstJSONLine(stdout); line != "" {
			var out wirePreOut
			if json.Unmarshal([]byte(line), &out) == nil {
				if out.Reason != "" {
					res.Reason = out.Reason
				}
				res.Context = out.Context
			}
		}
		return res, nil
	}
	if code != 0 {
		return PreResult{}, fmt.Errorf("hook %s exited %d", h.name, code)
	}

	line := firstJSONLine(stdout)
	if line == "" {
		return PreResult{Action: ActionAllow}, nil
	}
	var out wirePreOut
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		return PreResult{}, fmt.Errorf("hook %s invalid json: %w", h.name, err)
	}
	action, err := parseWireAction(out.Action)
	if err != nil {
		return PreResult{}, fmt.Errorf("hook %s: %w", h.name, err)
	}
	return PreResult{
		Action:  action,
		Input:   out.Input,
		Reason:  out.Reason,
		Context: out.Context,
	}, nil
}

func (h *CommandHook) runPost(ctx context.Context, ev Event) (PostResult, error) {
	stdout, code, err := h.invoke(ctx, KindPostTool, ev)
	if err != nil {
		return PostResult{}, err
	}
	if code == ExitDeny {
		res := PostResult{Stop: true, Reason: "hook denied (exit 2)"}
		if line := firstJSONLine(stdout); line != "" {
			var out wirePostOut
			if json.Unmarshal([]byte(line), &out) == nil {
				if out.Reason != "" {
					res.Reason = out.Reason
				}
				res.Context = out.Context
				if out.Stop {
					res.Stop = true
				}
			}
		}
		return res, nil
	}
	if code != 0 {
		return PostResult{}, fmt.Errorf("hook %s exited %d", h.name, code)
	}
	line := firstJSONLine(stdout)
	if line == "" {
		return PostResult{}, nil
	}
	var out wirePostOut
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		return PostResult{}, fmt.Errorf("hook %s invalid json: %w", h.name, err)
	}
	return PostResult(out), nil
}

func (h *CommandHook) runCommand(ctx context.Context, ev CommandEvent) (CommandResult, error) {
	stdout, code, err := h.spawn(ctx, wireIn{
		SessionID: ev.SessionID,
		Cwd:       ev.Cwd,
		HookEvent: string(KindCommand),
		Command:   h.name,
		Args:      ev.Args,
	})
	if err != nil {
		return CommandResult{}, err
	}
	line := firstJSONLine(stdout)
	if code != 0 {
		reason := ""
		if line != "" {
			var out wireCommandOut
			if json.Unmarshal([]byte(line), &out) == nil {
				reason = out.Reason
			}
		}
		if reason == "" {
			return CommandResult{}, fmt.Errorf("hook %s exited %d", h.name, code)
		}
		return CommandResult{}, fmt.Errorf("hook %s exited %d: %s", h.name, code, reason)
	}
	if line == "" {
		return CommandResult{}, nil
	}
	var out wireCommandOut
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		return CommandResult{}, fmt.Errorf("hook %s invalid json: %w", h.name, err)
	}
	res := CommandResult{Submit: out.Submit, Toast: out.Toast, List: out.List}
	if out.Status != nil {
		res.Status = *out.Status
		res.StatusSet = true
	}
	if res.List != nil && len(res.List.Items) == 0 {
		res.List = nil
	}
	return res, nil
}

func (h *CommandHook) runSession(ctx context.Context, ev SessionEvent) (SessionResult, error) {
	stdout, code, err := h.spawn(ctx, wireIn{
		SessionID:         ev.SessionID,
		Cwd:               ev.Cwd,
		HookEvent:         string(ev.Kind),
		Reason:            ev.Reason,
		PreviousSessionID: ev.PreviousSessionID,
		TargetSessionID:   ev.TargetSessionID,
	})
	if err != nil {
		return SessionResult{}, err
	}
	line := firstJSONLine(stdout)
	if code == ExitDeny {
		res := SessionResult{Action: ActionDeny, Reason: "hook denied (exit 2)"}
		if line != "" {
			var out wireSessionOut
			if json.Unmarshal([]byte(line), &out) == nil {
				if out.Reason != "" {
					res.Reason = out.Reason
				}
				res.Toast = out.Toast
				if out.Status != nil {
					res.Status = *out.Status
					res.StatusSet = true
				}
			}
		}
		return res, nil
	}
	if code != 0 {
		reason := ""
		if line != "" {
			var out wireSessionOut
			if json.Unmarshal([]byte(line), &out) == nil {
				reason = out.Reason
			}
		}
		if reason == "" {
			return SessionResult{}, fmt.Errorf("hook %s exited %d", h.name, code)
		}
		return SessionResult{}, fmt.Errorf("hook %s exited %d: %s", h.name, code, reason)
	}
	if line == "" {
		return SessionResult{Action: ActionAllow}, nil
	}
	var out wireSessionOut
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		return SessionResult{}, fmt.Errorf("hook %s invalid json: %w", h.name, err)
	}
	action, err := parseWireAction(out.Action)
	if err != nil {
		return SessionResult{}, fmt.Errorf("hook %s: %w", h.name, err)
	}
	res := SessionResult{Action: action, Reason: out.Reason, Toast: out.Toast}
	if out.Status != nil {
		res.Status = *out.Status
		res.StatusSet = true
	}
	return res, nil
}

func (h *CommandHook) invoke(ctx context.Context, kind Kind, ev Event) ([]byte, int, error) {
	return h.spawn(ctx, wireIn{
		SessionID: ev.SessionID,
		Cwd:       ev.Cwd,
		HookEvent: string(kind),
		Tool:      ev.Tool,
		ToolUseID: ev.ToolUseID,
		Input:     ev.Input,
		Output:    ev.Output,
		Err:       ev.Err,
	})
}

func (h *CommandHook) spawn(ctx context.Context, in wireIn) ([]byte, int, error) {
	if h.runPath == "" {
		return nil, 0, fmt.Errorf("hook %s: empty run path", h.name)
	}
	timeout := h.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload, err := json.Marshal(in)
	if err != nil {
		return nil, 0, err
	}
	payload = append(payload, '\n')

	cmd := exec.CommandContext(ctx, h.runPath) //nolint:gosec // G204: hook binaries are user-configured by design
	cmd.Dir = h.dir
	cmd.Env = sanitizeEnv(environ(), hookEnv{
		Event:      in.HookEvent,
		SessionID:  in.SessionID,
		Cwd:        in.Cwd,
		ProjectDir: in.Cwd,
	})
	cmd.Stdin = bytes.NewReader(payload)

	var stdout, stderr limitedBuffer
	stdout.limit = MaxHookOutputBytes
	stderr.limit = MaxHookOutputBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return stdout.Bytes(), 0, fmt.Errorf("hook %s timed out after %s", h.name, timeout)
	}
	if err != nil {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			return stdout.Bytes(), ee.ExitCode(), nil
		}
		return stdout.Bytes(), 0, fmt.Errorf("hook %s: %w", h.name, err)
	}
	return stdout.Bytes(), 0, nil
}

func parseWireAction(s string) (Action, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "allow":
		return ActionAllow, nil
	case "deny":
		return ActionDeny, nil
	case "modify":
		return ActionModify, nil
	default:
		return 0, fmt.Errorf("unknown action %q", s)
	}
}

func firstJSONLine(b []byte) string {
	line, _, _ := strings.Cut(string(b), "\n")
	return strings.TrimSpace(line)
}

// limitedBuffer collects up to limit bytes, discarding the rest.
type limitedBuffer struct {
	buf   bytes.Buffer
	limit int
	n     int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.limit <= 0 {
		return l.buf.Write(p)
	}
	remain := l.limit - l.n
	if remain <= 0 {
		return len(p), nil
	}
	if len(p) > remain {
		l.buf.Write(p[:remain])
		l.n = l.limit
		return len(p), nil
	}
	n, err := l.buf.Write(p)
	l.n += n
	return n, err
}

func (l *limitedBuffer) Bytes() []byte { return l.buf.Bytes() }

var (
	_ io.Writer = (*limitedBuffer)(nil)
	_ Hook      = (*CommandHook)(nil)
)
