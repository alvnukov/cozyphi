package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/redact"
)

// claudePlugin plants a plugin root with hooks/hooks.json and returns the
// PluginHooks that describes it, as internal/plugin would.
func claudePlugin(t *testing.T, name, hooksJSON string) PluginHooks {
	t.Helper()
	root := t.TempDir()
	file := filepath.Join(root, "hooks", "hooks.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
	require.NoError(t, os.WriteFile(file, []byte(hooksJSON), 0o600))
	return PluginHooks{
		Name:  name,
		Files: []string{file},
		Vars: map[string]string{
			EnvClaudePluginRoot: root,
			EnvClaudePluginData: filepath.Join(t.TempDir(), "data"),
			EnvClaudeProjectDir: t.TempDir(),
		},
	}
}

// oneHook renders a hooks.json with one command hook under event/matcher.
func oneHook(t *testing.T, event, matcher string, hook map[string]any) string {
	t.Helper()
	hook["type"] = "command"
	raw, err := json.Marshal(map[string]any{"hooks": map[string]any{
		event: []any{map[string]any{"matcher": matcher, "hooks": []any{hook}}},
	}})
	require.NoError(t, err)
	return string(raw)
}

func pluginManager(t *testing.T, plugins ...PluginHooks) *Manager {
	t.Helper()
	t.Setenv(EnvHooks, "")
	mgr, warns, err := Load("", "", plugins...)
	require.NoError(t, err)
	require.Empty(t, warns)
	return mgr
}

// pluginHook returns the single hook the plugin declares, for tests that
// need the error a manager would only log.
func pluginHook(t *testing.T, p PluginHooks) Hook {
	t.Helper()
	t.Setenv(EnvHooks, "")
	found, warns, err := Discover("", "", p)
	require.NoError(t, err)
	require.Empty(t, warns)
	require.Len(t, found, 1)
	return EntryFromDiscovered(found[0]).Hook
}

func startWith(mgr *Manager, reason string) SessionOutcome {
	return mgr.SessionStart(context.Background(), SessionEvent{SessionID: "s1", Cwd: "/tmp", Reason: reason})
}

func TestClaudeHookJSONContextSurvivesBackgroundChild(t *testing.T) {
	cmd := `echo '{'; echo '"hookSpecificOutput": {"hookEventName": "SessionStart",'; ` +
		`echo '"additionalContext": "BOOT"},'; echo '"systemMessage": "hello"}'; sleep 10 &`
	mgr := pluginManager(t, claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{"command": cmd})))

	began := time.Now()
	out := startWith(mgr, ReasonStartup)
	require.Less(t, time.Since(began), 5*time.Second, "a background child must not hold the session open")
	require.Equal(t, "BOOT", out.Context)
	require.Equal(t, "hello", out.Toast)
}

func TestClaudeHookAcceptsEveryContextSpelling(t *testing.T) {
	for _, body := range []string{
		`{"hookSpecificOutput":{"additionalContext":"X"}}`, `{"additionalContext":"X"}`, `{"additional_context":"X"}`, `X`,
	} {
		cmd := "printf '%s' '" + body + "'"
		mgr := pluginManager(
			t,
			claudePlugin(t, "demo", oneHook(t, "SessionStart", "*", map[string]any{"command": cmd})),
		)
		require.Equal(t, "X", startWith(mgr, ReasonStartup).Context, body)
	}
}

func TestClaudeHookMatcherAppliesToMappedSource(t *testing.T) {
	mgr := pluginManager(t, claudePlugin(t, "demo",
		oneHook(t, "SessionStart", "startup|clear|compact", map[string]any{"command": "echo hit"})))
	for reason, want := range map[string]string{
		ReasonStartup: "hit", ReasonNew: "hit", ReasonCompact: "hit", ReasonResume: "", "reload": "",
	} {
		require.Equal(t, want, startWith(mgr, reason).Context, reason)
	}
}

func TestClaudeHookStdinCarriesClaudeFields(t *testing.T) {
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{
		"command": `cat > "$CLAUDE_PLUGIN_DATA/in.json"`,
	}))
	startWith(pluginManager(t, p), ReasonNew)

	raw, err := os.ReadFile(filepath.Join(p.Vars[EnvClaudePluginData], "in.json"))
	require.NoError(t, err, "the data dir is created before the first run")
	var in map[string]any
	require.NoError(t, json.Unmarshal(raw, &in))
	require.Equal(t, map[string]any{
		"session_id": "s1", "cwd": "/tmp", "hook_event_name": "SessionStart", "source": "clear",
	}, in)
}

func TestClaudeSessionEndMapsQuitReason(t *testing.T) {
	p := claudePlugin(t, "demo", oneHook(t, "SessionEnd", "", map[string]any{
		"command": `cat > "$CLAUDE_PLUGIN_DATA/end.json"`,
	}))
	pluginManager(t, p).SessionShutdown(t.Context(),
		SessionEvent{SessionID: "s1", Cwd: "/tmp", Reason: ReasonQuit})

	raw, err := os.ReadFile(filepath.Join(p.Vars[EnvClaudePluginData], "end.json"))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"reason":"prompt_input_exit"`)
	require.Contains(t, string(raw), `"hook_event_name":"SessionEnd"`)
}

func TestClaudeHookEnvironmentIsSanitizedAndRooted(t *testing.T) {
	t.Setenv("DEMO_API_KEY", "leak")
	t.Setenv(EnvClaudePluginRoot, "/outer/claude/plugin") // cozyphi itself running inside Claude Code
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{
		"command": `echo "key=${DEMO_API_KEY:-unset} root=$CLAUDE_PLUGIN_ROOT pwd=$(pwd -P) ` +
			`roots=$(env | grep -c '^CLAUDE_PLUGIN_ROOT=')"`,
	}))
	project, err := filepath.EvalSymlinks(p.Vars[EnvClaudeProjectDir])
	require.NoError(t, err)

	got := startWith(pluginManager(t, p), ReasonStartup).Context
	require.Equal(t, "key=unset root="+p.Vars[EnvClaudePluginRoot]+" pwd="+project+" roots=1", got)
}

func TestClaudeHookDropsInheritedClaudeVarsAndSensitiveVars(t *testing.T) {
	t.Setenv(EnvClaudePluginData, "/outer/claude/data")
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{
		"command": `echo "data=${CLAUDE_PLUGIN_DATA:-unset} token=${DEMO_TOKEN:-unset}"`,
	}))
	delete(p.Vars, EnvClaudePluginData)
	p.Vars["DEMO_TOKEN"] = "leak"

	got := startWith(pluginManager(t, p), ReasonStartup).Context
	require.Equal(t, "data=unset token=unset", got,
		"an outer Claude Code plugin's data dir must not leak in, and a sensitive var is never set")
}

func TestClaudeHookArgsRunWithoutShell(t *testing.T) {
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{
		"command": "printf",
		"args":    []string{"%s|", "one two", "${CLAUDE_PLUGIN_ROOT}", "$HOME"},
	}))
	got := startWith(pluginManager(t, p), ReasonStartup).Context
	require.Equal(t, "one two|"+p.Vars[EnvClaudePluginRoot]+"|$HOME|", got, "no shell expansion, only placeholders")
}

func TestClaudeHookFailureIsNonBlockingAndRedacted(t *testing.T) {
	token := "ghp_" + strings.Repeat("a1B2", 9)
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{
		"command": "echo 'boom " + token + "' >&2; exit 2",
	}))

	_, err := pluginHook(t, p).Session(t.Context(),
		SessionEvent{Kind: KindSessionStart, SessionID: "s1", Cwd: "/tmp", Reason: ReasonStartup})
	require.ErrorContains(t, err, "exited 2")
	require.ErrorContains(t, err, redact.Marker)
	require.NotContains(t, err.Error(), token)

	out := startWith(pluginManager(t, p), ReasonStartup)
	require.False(t, out.Denied, "exit 2 never blocks a session")
	require.Empty(t, out.Context)
}

func TestClaudeHookExitWithoutStderrEndsCleanly(t *testing.T) {
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{"command": "exit 3"}))
	_, err := pluginHook(t, p).Session(t.Context(),
		SessionEvent{Kind: KindSessionStart, Reason: ReasonStartup, Cwd: "/tmp"})
	require.ErrorContains(t, err, "hook plugin:demo/SessionStart#1 exited 3 — ")
	require.NotContains(t, err.Error(), "exited 3:")
}

func TestClaudeHookCancellationIsNotATimeout(t *testing.T) {
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{"command": "sleep 5"}))
	hook := pluginHook(t, p)
	cancelled, cancel := context.WithCancel(t.Context())
	time.AfterFunc(200*time.Millisecond, cancel)
	expired, stop := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer stop()

	for name, ctx := range map[string]context.Context{"cancelled": cancelled, "parent deadline": expired} {
		began := time.Now()
		_, err := hook.Session(ctx, SessionEvent{Kind: KindSessionStart, Reason: ReasonStartup, Cwd: "/tmp"})
		require.ErrorContains(t, err, "cancelled", name)
		require.NotContains(t, err.Error(), "exited", name)
		require.NotContains(t, err.Error(), "timed out", name)
		require.Less(t, time.Since(began), 4*time.Second, name)
	}
}

func TestClaudeHookTimeoutIsCapped(t *testing.T) {
	t.Setenv(EnvHooks, "")
	for declared, want := range map[float64]time.Duration{
		0: 30 * time.Second, -1: 30 * time.Second, 1.5: 1500 * time.Millisecond,
		60: 60 * time.Second, 600: 60 * time.Second,
	} {
		p := claudePlugin(
			t,
			"demo",
			oneHook(t, "SessionStart", "", map[string]any{"command": "true", "timeout": declared}),
		)
		found, _, err := Discover("", "", p)
		require.NoError(t, err)
		require.Len(t, found, 1)
		require.Equal(t, want, found[0].Manifest.Timeout, "timeout %v", declared)
	}
}

func TestClaudeHookTimeout(t *testing.T) {
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{"command": "sleep 5", "timeout": 1}))
	began := time.Now()
	_, err := pluginHook(t, p).Session(t.Context(),
		SessionEvent{Kind: KindSessionStart, Reason: ReasonStartup, Cwd: "/tmp"})
	require.ErrorContains(t, err, "timed out after 1s")
	require.ErrorContains(t, err, "raise its timeout")
	require.Less(t, time.Since(began), 4*time.Second)
}

func TestClaudeHookContextIsCappedOnARuneBoundary(t *testing.T) {
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{
		"command": `cat "$CLAUDE_PLUGIN_ROOT/big.txt"`,
	}))
	big := "a" + strings.Repeat("é", 9000) // é starts at odd offsets, so byte 16384 is mid-rune
	require.NoError(t, os.WriteFile(filepath.Join(p.Vars[EnvClaudePluginRoot], "big.txt"), []byte(big), 0o600))

	got := startWith(pluginManager(t, p), ReasonStartup).Context
	const marker = "\n[plugin hook context truncated at 16 KiB]"
	require.True(t, strings.HasSuffix(got, marker))
	body := strings.TrimSuffix(got, marker)
	require.True(t, utf8.ValidString(body))
	require.LessOrEqual(t, len(body), MaxPluginContextBytes)
	require.Greater(t, len(body), MaxPluginContextBytes-4)
}

func TestClaudeHooksWarnAndSkipWhatCozyphiCannotRun(t *testing.T) {
	t.Setenv(EnvHooks, "")
	p := claudePlugin(t, "demo", `{"hooks":{
	  "PreToolUse":[{"hooks":[{"type":"command","command":"true"}]}],
	  "SessionStart":[
	    {"matcher":"(","hooks":[{"type":"command","command":"true"}]},
	    {"hooks":[
	      {"type":"prompt","command":"true"},
	      {"type":"command","shell":"powershell","command":"true"},
	      {"type":"command","command":"  "},
	      {"type":"command","command":"echo ${user_config.token}"}
	    ]}
	  ]}}`)
	found, warns, err := Discover("", "", p)
	require.NoError(t, err)
	require.Empty(t, found)
	require.Len(t, warns, 6)
	text := make([]string, 0, len(warns))
	for _, w := range warns {
		text = append(text, w.String())
	}
	all := strings.Join(text, "\n")
	for _, want := range []string{
		"event PreToolUse is not supported", "not a valid regular expression", `hook type "prompt"`,
		`hook shell "powershell"`, "empty command",
		"${user_config.*}, which cozyphi does not provide; skipped — disable the plugin or remove the reference",
	} {
		require.Contains(t, all, want)
	}
}

func TestClaudeHooksInvalidJSONIsAWarning(t *testing.T) {
	t.Setenv(EnvHooks, "")
	found, warns, err := Discover("", "", claudePlugin(t, "demo", "{"))
	require.NoError(t, err)
	require.Empty(t, found)
	require.Len(t, warns, 1)
	require.Contains(t, warns[0].Message, "invalid hooks.json")
	require.Contains(t, warns[0].Message, "fix the file or disable the plugin")
}

func TestClaudeHookContextsJoinInEntryOrder(t *testing.T) {
	a := claudePlugin(t, "alpha", oneHook(t, "SessionStart", "", map[string]any{"command": "echo A"}))
	b := claudePlugin(t, "beta", oneHook(t, "SessionStart", "", map[string]any{"command": "echo B"}))
	require.Equal(t, "A\n\nB", startWith(pluginManager(t, a, b), ReasonStartup).Context)
}

func TestClaudeHookNamesAndSource(t *testing.T) {
	t.Setenv(EnvHooks, "")
	p := claudePlugin(
		t,
		"demo",
		oneHook(t, "SessionStart", "startup", map[string]any{"command": "true", "async": true}),
	)
	found, _, err := Discover("", "", p)
	require.NoError(t, err)
	require.Len(t, found, 1)
	d := found[0]
	require.Equal(t, "plugin:demo/SessionStart#1", d.Manifest.Name)
	require.Equal(t, "plugin:demo", d.Source)
	require.Equal(t, "demo", d.Manifest.Plugin)
	require.Equal(t, KindSessionStart, d.Manifest.Kind)
	require.Equal(t, "startup", d.Manifest.Match)
	require.Equal(t, 30*time.Second, d.Manifest.Timeout)
	require.True(t, EntryFromDiscovered(d).Async)
}

func TestClaudeHooksHonourHooksOff(t *testing.T) {
	t.Setenv(EnvHooks, "off")
	found, warns, err := Discover("", "", claudePlugin(t, "demo", "{"))
	require.NoError(t, err)
	require.Empty(t, found)
	require.Empty(t, warns)
}

func TestClaudeHooksSkippedInFailClosedOnlyMode(t *testing.T) {
	mgr := pluginManager(
		t,
		claudePlugin(t, "demo", oneHook(t, "SessionStart", "", map[string]any{"command": "echo hit"})),
	)
	require.Equal(t, "hit", startWith(mgr, ReasonStartup).Context)
	require.Empty(t, startWith(mgr.FailClosedOnly(), ReasonStartup).Context, "a plugin hook is never fail-closed")
}

func TestManagerFailuresReportThePluginHookLastRun(t *testing.T) {
	p := claudePlugin(t, "demo", oneHook(t, "SessionStart", "startup|clear", map[string]any{
		"command": `if [ -f "$CLAUDE_PLUGIN_DATA/ok" ]; then echo fine; else echo broken >&2; exit 1; fi`,
	}))
	mgr := pluginManager(t, p)
	require.Empty(t, mgr.Failures(), "nothing has run yet")

	startWith(mgr, ReasonStartup)
	fails := mgr.Failures()
	require.Len(t, fails, 1)
	require.Equal(t, "plugin:demo/SessionStart#1", fails[0].Path)
	require.Contains(t, fails[0].Message, "exited 1: broken")
	require.Contains(t, fails[0].Message, "disable the plugin")
	require.Equal(t, fails, mgr.FailClosedOnly().Failures(), "a fail-closed-only view shares the record")

	startWith(mgr, ReasonResume)
	require.Len(t, mgr.Failures(), 1, "a run the matcher skipped is not a success")

	require.NoError(t, os.WriteFile(filepath.Join(p.Vars[EnvClaudePluginData], "ok"), nil, 0o600))
	require.Equal(t, "fine", startWith(mgr, ReasonNew).Context)
	require.Empty(t, mgr.Failures(), "a later success clears the record")
}

func TestManagerFailuresRecordAsyncPluginHooks(t *testing.T) {
	mgr := pluginManager(t, claudePlugin(t, "demo", oneHook(t, "SessionEnd", "", map[string]any{
		"command": "exit 4", "async": true,
	})))
	mgr.SessionShutdown(t.Context(), SessionEvent{SessionID: "s1", Cwd: "/tmp", Reason: ReasonQuit})
	require.Eventually(t, func() bool { return len(mgr.Failures()) == 1 }, 5*time.Second, 10*time.Millisecond)
	require.Contains(t, mgr.Failures()[0].Message, "exited 4")
}

func TestManagerFailuresIgnoreHooksThatAreNotPlugins(t *testing.T) {
	var none *Manager
	require.Nil(t, none.Failures())

	mgr := NewManager(Entry{Kind: KindSessionStart, Hook: FuncHook{
		HookName: "plugin:fake/SessionStart#1",
		Sess: func(context.Context, SessionEvent) (SessionResult, error) {
			return SessionResult{}, errors.New("boom")
		},
	}})
	startWith(mgr, ReasonStartup)
	require.Empty(t, mgr.Failures(), "only a Claude Code plugin hook records its failures")
}
