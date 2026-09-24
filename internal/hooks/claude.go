package hooks

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alvnukov/cozyphi/internal/redact"
)

// Placeholders a Claude Code plugin hook may reference. They are set in the
// hook's environment and substituted in the args of an exec-style hook.
const (
	EnvClaudePluginRoot = "CLAUDE_PLUGIN_ROOT"
	EnvClaudePluginData = "CLAUDE_PLUGIN_DATA"
	EnvClaudeProjectDir = "CLAUDE_PROJECT_DIR"
)

// MaxPluginContextBytes caps the context one plugin hook contributes. It is
// larger than MaxContextBytes because a plugin bootstrap (superpowers'
// using-superpowers) is a whole skill body, not a note.
const MaxPluginContextBytes = 16 * 1024

// SourcePluginPrefix starts the Source of a plugin hook: "plugin:<Name>".
const SourcePluginPrefix = "plugin:"

const (
	// claudeDefaultTimeout is shorter than Claude Code's 600 s because
	// session_start runs synchronously while the session opens.
	claudeDefaultTimeout = 30 * time.Second
	pluginContextMarker  = "\n[plugin hook context truncated at 16 KiB]"
)

// PluginHooks describes one Claude Code plugin's hook files in the terms this
// package needs, so hooks never imports internal/plugin.
type PluginHooks struct {
	Name  string            // plugin name: the hook-name namespace and the source label
	Files []string          // absolute hooks.json paths
	Vars  map[string]string // EnvClaudePluginRoot, EnvClaudePluginData, EnvClaudeProjectDir
}

// claudeEvents maps the supported Claude Code events to cozyphi kinds.
var claudeEvents = map[string]Kind{
	"SessionStart": KindSessionStart,
	"SessionEnd":   KindSessionShutdown,
}

type claudeHooksFile struct {
	Hooks map[string][]claudeMatcherGroup `json:"hooks"`
}

type claudeMatcherGroup struct {
	Matcher string              `json:"matcher"`
	Hooks   []claudeHookCommand `json:"hooks"`
}

type claudeHookCommand struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Shell   string   `json:"shell"`
	Timeout float64  `json:"timeout"` // seconds
	Async   bool     `json:"async"`
}

func parseClaudeHooks(p PluginHooks) ([]Discovered, []Warning) {
	var (
		out    []Discovered
		warns  []Warning
		counts = make(map[string]int)
	)
	for _, file := range p.Files {
		warn := func(format string, args ...any) {
			warns = append(warns, Warning{Path: file, Message: fmt.Sprintf(format, args...)})
		}
		data, err := os.ReadFile(file)
		if err != nil {
			warn("read hooks file: %v — reinstall the plugin or fix the hooks path in its plugin.json", err)
			continue
		}
		var parsed claudeHooksFile
		if err := json.Unmarshal(data, &parsed); err != nil {
			warn("invalid hooks.json: %v — fix the file or disable the plugin", err)
			continue
		}
		for _, event := range slices.Sorted(maps.Keys(parsed.Hooks)) {
			kind, ok := claudeEvents[event]
			if !ok {
				warn("event %s is not supported; skipped — cozyphi runs SessionStart and SessionEnd only", event)
				continue
			}
			for _, group := range parsed.Hooks[event] {
				matcher, err := compileMatcher(group.Matcher)
				if err != nil {
					warn("%s matcher %q is not a valid regular expression: %v; group skipped — fix the matcher",
						event, group.Matcher, err)
					continue
				}
				for _, hc := range group.Hooks {
					if problem := claudeCommandProblem(hc); problem != "" {
						warn("%s: %s", event, problem)
						continue
					}
					counts[event]++
					name := fmt.Sprintf("%s%s/%s#%d", SourcePluginPrefix, p.Name, event, counts[event])
					hook := &ClaudeHook{
						name:    name,
						kind:    kind,
						matcher: matcher,
						command: hc.Command,
						args:    hc.Args,
						timeout: claudeTimeout(hc.Timeout),
						vars:    p.Vars,
					}
					out = append(out, Discovered{
						Manifest: Manifest{
							Name:    name,
							Kind:    kind,
							Match:   cmp.Or(group.Matcher, "*"),
							Run:     hc.Command,
							Timeout: hook.timeout,
							Async:   hc.Async,
							Plugin:  p.Name,
							Dir:     p.Vars[EnvClaudePluginRoot],
							Path:    file,
						},
						Source: SourcePluginPrefix + p.Name,
						claude: hook,
					})
				}
			}
		}
	}
	return out, warns
}

func claudeCommandProblem(hc claudeHookCommand) string {
	userConfig := func(s string) bool { return strings.Contains(s, "${user_config.") }
	switch {
	case hc.Type != "command":
		return fmt.Sprintf("hook type %q is not supported; skipped — cozyphi runs command hooks only", hc.Type)
	case hc.Shell != "" && hc.Shell != "bash":
		return fmt.Sprintf("hook shell %q is not supported; skipped — use bash or omit shell", hc.Shell)
	case strings.TrimSpace(hc.Command) == "":
		return "hook has an empty command; skipped — set command in hooks.json"
	case userConfig(hc.Command) || slices.ContainsFunc(hc.Args, userConfig):
		return "hook references ${user_config.*}, which cozyphi does not provide; skipped"
	}
	return ""
}

// compileMatcher anchors a Claude matcher, so "startup|clear" matches either
// word and nothing longer. Empty and "*" match everything (nil).
func compileMatcher(m string) (*regexp.Regexp, error) {
	if m == "" || m == "*" {
		return nil, nil
	}
	return regexp.Compile("^(?:" + m + ")$")
}

func claudeTimeout(seconds float64) time.Duration {
	if seconds <= 0 {
		return claudeDefaultTimeout
	}
	if seconds >= maxTimeout.Seconds() {
		return maxTimeout
	}
	return time.Duration(seconds * float64(time.Second))
}

// ClaudeHook runs one Claude Code plugin command hook for session_start or
// session_shutdown. It is the second adapter behind Hook, next to
// CommandHook; tool and command kinds never reach it.
type ClaudeHook struct {
	name    string
	kind    Kind
	matcher *regexp.Regexp
	command string
	args    []string
	timeout time.Duration
	vars    map[string]string
}

// Name returns "plugin:<Name>/<Event>#<n>".
func (h *ClaudeHook) Name() string { return h.name }

// Match is false: a plugin hook never runs for a tool.
func (*ClaudeHook) Match(string) bool { return false }

// PreTool is a no-op allow.
func (*ClaudeHook) PreTool(context.Context, Event) (PreResult, error) {
	return PreResult{Action: ActionAllow}, nil
}

// PostTool is a no-op.
func (*ClaudeHook) PostTool(context.Context, Event) (PostResult, error) { return PostResult{}, nil }

// Command is a no-op.
func (*ClaudeHook) Command(context.Context, CommandEvent) (CommandResult, error) {
	return CommandResult{}, nil
}

// Session runs the hook when the event is its kind, the reason maps to a
// Claude source, and the matcher accepts that source.
func (h *ClaudeHook) Session(ctx context.Context, ev SessionEvent) (SessionResult, error) {
	allow := SessionResult{Action: ActionAllow}
	if ev.Kind != h.kind {
		return allow, nil
	}
	source, ok := claudeSource(h.kind, ev.Reason)
	if !ok {
		return allow, nil
	}
	if h.matcher != nil && !h.matcher.MatchString(source) {
		return allow, nil
	}
	return h.run(ctx, ev, source)
}

// claudeSource maps a cozyphi reason to SessionStart.source or
// SessionEnd.reason; SessionStart has no answer for an unknown reason.
func claudeSource(kind Kind, reason string) (string, bool) {
	if kind == KindSessionStart {
		switch reason {
		case ReasonStartup:
			return "startup", true
		case ReasonNew:
			return "clear", true
		case ReasonResume:
			return "resume", true
		case ReasonCompact:
			return "compact", true
		}
		return "", false
	}
	switch reason {
	case ReasonNew:
		return "clear", true
	case ReasonResume:
		return "resume", true
	case ReasonQuit:
		return "prompt_input_exit", true
	}
	return "other", true
}

// claudeWireIn is Claude Code's session hook input. transcript_path is left
// out on purpose: cozyphi's session file is not a Claude transcript.
type claudeWireIn struct {
	SessionID     string `json:"session_id"`
	Cwd           string `json:"cwd"`
	HookEventName string `json:"hook_event_name"`
	Source        string `json:"source,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

func (h *ClaudeHook) run(ctx context.Context, ev SessionEvent, source string) (SessionResult, error) {
	in := claudeWireIn{SessionID: ev.SessionID, Cwd: ev.Cwd, HookEventName: "SessionEnd", Reason: source}
	if h.kind == KindSessionStart {
		in = claudeWireIn{SessionID: ev.SessionID, Cwd: ev.Cwd, HookEventName: "SessionStart", Source: source}
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return SessionResult{}, fmt.Errorf("hook %s: encode input: %w", h.name, err)
	}
	if data := h.vars[EnvClaudePluginData]; data != "" {
		if err := os.MkdirAll(data, 0o700); err != nil {
			return SessionResult{}, fmt.Errorf("hook %s: create %s: %w — check its permissions", h.name, data, err)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	cmd := h.newCmd(ctx)
	cmd.Dir = cmp.Or(h.vars[EnvClaudeProjectDir], ev.Cwd)
	cmd.Env = h.env(ev)
	cmd.Stdin = bytes.NewReader(append(payload, '\n'))
	var stdout, stderr limitedBuffer
	stdout.limit, stderr.limit = MaxHookOutputBytes, MaxHookOutputBytes
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// A hook that backgrounds a child leaves the pipes open after it exits;
	// WaitDelay stops waiting for them one second later.
	cmd.WaitDelay = time.Second

	err = cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return SessionResult{}, fmt.Errorf("hook %s timed out after %s — raise its timeout (max %s) or fix the script",
			h.name, h.timeout, maxTimeout)
	}
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			return SessionResult{}, fmt.Errorf("hook %s exited %d: %s",
				h.name, ee.ExitCode(), redact.Redact(firstLine(string(stderr.Bytes()))))
		}
		return SessionResult{}, fmt.Errorf("hook %s: %w", h.name, err)
	}
	return claudeResult(stdout.Bytes()), nil
}

// newCmd runs args as exec with placeholders substituted, or the command
// through bash -c. The shell form is not substituted textually: bash expands
// ${CLAUDE_PLUGIN_ROOT} from the environment itself, so a path holding a
// quote or a dollar sign cannot rewrite the command.
func (h *ClaudeHook) newCmd(ctx context.Context) *exec.Cmd {
	if len(h.args) == 0 {
		return exec.CommandContext(ctx, "bash", "-c", h.command) //nolint:gosec // G204: the user enabled this plugin
	}
	expand := varReplacer(h.vars)
	args := make([]string, len(h.args))
	for i, a := range h.args {
		args[i] = expand.Replace(a)
	}
	name := expand.Replace(h.command)
	return exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: the user enabled this plugin
}

func (h *ClaudeHook) env(ev SessionEvent) []string {
	env := sanitizeEnv(environ(), hookEnv{
		Event:      string(h.kind),
		SessionID:  ev.SessionID,
		Cwd:        ev.Cwd,
		ProjectDir: cmp.Or(h.vars[EnvClaudeProjectDir], ev.Cwd),
	})
	for _, k := range slices.Sorted(maps.Keys(h.vars)) {
		env = append(env, k+"="+h.vars[k]) // later duplicates win in os/exec
	}
	return env
}

func varReplacer(vars map[string]string) *strings.Replacer {
	pairs := make([]string, 0, 2*len(vars))
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		pairs = append(pairs, "${"+k+"}", vars[k])
	}
	return strings.NewReplacer(pairs...)
}

type claudeWireOut struct {
	HookSpecificOutput struct {
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
	AdditionalContext      string `json:"additionalContext"`
	AdditionalContextSnake string `json:"additional_context"`
	SystemMessage          string `json:"systemMessage"`
}

// claudeResult reads exit-0 stdout: a JSON object supplies context and a
// toast; any other non-empty text is context verbatim.
func claudeResult(stdout []byte) SessionResult {
	res := SessionResult{Action: ActionAllow}
	text := strings.TrimSpace(string(stdout))
	if text == "" {
		return res
	}
	var out claudeWireOut
	if strings.HasPrefix(text, "{") && json.Unmarshal([]byte(text), &out) == nil {
		res.Context = capContext(cmp.Or(
			out.HookSpecificOutput.AdditionalContext, out.AdditionalContext, out.AdditionalContextSnake))
		res.Toast = out.SystemMessage
		return res
	}
	res.Context = capContext(text)
	return res
}

func capContext(s string) string {
	if len(s) <= MaxPluginContextBytes {
		return s
	}
	cut := MaxPluginContextBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + pluginContextMarker
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
