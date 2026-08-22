package hooks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testScript(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("command hook fixtures are shell scripts")
	}
	path := filepath.Join("testdata", name)
	abs, err := filepath.Abs(path)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(abs, 0o755))
	return abs
}

func preHook(t *testing.T, script, match string, timeout time.Duration) *CommandHook {
	t.Helper()
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &CommandHook{
		name:    script,
		kind:    KindPreTool,
		match:   match,
		runPath: testScript(t, script),
		dir:     filepath.Dir(testScript(t, script)),
		timeout: timeout,
	}
}

func TestCommandHookAllowDenyModify(t *testing.T) {
	ctx := t.Context()
	ev := Event{Tool: "bash", Input: json.RawMessage(`{"command":"ls"}`), Cwd: "/tmp"}

	allow := preHook(t, "allow.sh", "bash", 0)
	res, err := allow.PreTool(ctx, ev)
	require.NoError(t, err)
	assert.Equal(t, ActionAllow, res.Action)

	deny := preHook(t, "deny.sh", "*", 0)
	res, err = deny.PreTool(ctx, ev)
	require.NoError(t, err)
	assert.Equal(t, ActionDeny, res.Action)
	assert.Equal(t, "blocked by script", res.Reason)

	mod := preHook(t, "modify.sh", "bash", 0)
	res, err = mod.PreTool(ctx, ev)
	require.NoError(t, err)
	assert.Equal(t, ActionModify, res.Action)
	assert.JSONEq(t, `{"command":"echo safe"}`, string(res.Input))
}

func TestCommandHookExit2(t *testing.T) {
	h := preHook(t, "exit2.sh", "*", 0)
	res, err := h.PreTool(t.Context(), Event{Tool: "bash", Input: json.RawMessage(`{}`)})
	require.NoError(t, err)
	assert.Equal(t, ActionDeny, res.Action)
	assert.Equal(t, "exit two", res.Reason)
}

func TestCommandHookBadJSON(t *testing.T) {
	h := preHook(t, "badjson.sh", "*", 0)
	_, err := h.PreTool(t.Context(), Event{Tool: "bash", Input: json.RawMessage(`{}`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid json")
}

func TestCommandHookExit1Error(t *testing.T) {
	h := preHook(t, "exit1.sh", "*", 0)
	_, err := h.PreTool(t.Context(), Event{Tool: "bash", Input: json.RawMessage(`{}`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exited 1")
}

func TestCommandHookTimeout(t *testing.T) {
	h := preHook(t, "slow.sh", "*", 200*time.Millisecond)
	_, err := h.PreTool(t.Context(), Event{Tool: "bash", Input: json.RawMessage(`{}`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestCommandHookPost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("command hook fixtures are shell scripts")
	}
	h := &CommandHook{
		name:    "post",
		kind:    KindPostTool,
		match:   "*",
		runPath: testScript(t, "post.sh"),
		dir:     filepath.Dir(testScript(t, "post.sh")),
		timeout: 5 * time.Second,
	}
	res, err := h.PostTool(t.Context(), Event{Tool: "bash", Output: "ok"})
	require.NoError(t, err)
	assert.Equal(t, "post note", res.Context)
}

func TestCommandHookSanitizedEnv(t *testing.T) {
	t.Setenv("PHI_API_KEY", "sk-secret")
	h := preHook(t, "checkenv.sh", "*", 0)
	res, err := h.PreTool(t.Context(), Event{
		Tool:      "bash",
		SessionID: "s1",
		Cwd:       "/proj",
		Input:     json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	assert.Equal(t, ActionAllow, res.Action)
	assert.Equal(t, "env ok", res.Context)
}

func TestCommandHookMatch(t *testing.T) {
	h := &CommandHook{match: "bash"}
	assert.True(t, h.Match("bash"))
	assert.False(t, h.Match("write"))
	h2 := &CommandHook{match: "*"}
	assert.True(t, h2.Match("write"))
}

func TestEntryFromDiscoveredFailClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("command hook fixtures are shell scripts")
	}
	script := testScript(t, "exit1.sh")
	d := Discovered{
		Manifest: Manifest{
			Name:       "strict",
			Kind:       KindPreTool,
			Match:      "*",
			Timeout:    5 * time.Second,
			FailClosed: true,
			Dir:        filepath.Dir(script),
		},
		RunPath: script,
		Source:  SourceUser,
	}
	mgr := NewManager(EntryFromDiscovered(d))
	out := mgr.PreTool(t.Context(), Event{Tool: "bash", Input: json.RawMessage(`{}`)})
	assert.True(t, out.Denied)
	assert.Contains(t, out.Reason, "fail_closed")
}

func TestEntriesFromDiscovered(t *testing.T) {
	ds := []Discovered{{
		Manifest: Manifest{Name: "a", Kind: KindPreTool, Match: "bash"},
		RunPath:  "/bin/true",
	}}
	entries := EntriesFromDiscovered(ds)
	require.Len(t, entries, 1)
	assert.Equal(t, KindPreTool, entries[0].Kind)
	assert.Equal(t, "a", entries[0].Hook.Name())
}

func cmdHook(t *testing.T, script string) *CommandHook {
	t.Helper()
	return &CommandHook{
		name:    "review",
		kind:    KindCommand,
		runPath: testScript(t, script),
		dir:     filepath.Dir(testScript(t, script)),
		timeout: 5 * time.Second,
	}
}

func TestCommandHookCommandSubmit(t *testing.T) {
	h := cmdHook(t, "command.sh")
	res, err := h.Command(t.Context(), CommandEvent{Cwd: "/tmp", Args: []string{"a"}})
	require.NoError(t, err)
	assert.Equal(t, "from command hook", res.Submit)
	assert.Empty(t, res.Toast)
}

func TestCommandHookCommandToast(t *testing.T) {
	h := cmdHook(t, "command_toast.sh")
	res, err := h.Command(t.Context(), CommandEvent{})
	require.NoError(t, err)
	assert.Equal(t, "done", res.Toast)
}

func TestCommandHookCommandUIIntents(t *testing.T) {
	h := cmdHook(t, "command_ui.sh")
	res, err := h.Command(t.Context(), CommandEvent{})
	require.NoError(t, err)
	require.True(t, res.StatusSet)
	assert.Equal(t, "3 findings", res.Status)
	require.NotNil(t, res.List)
	assert.Equal(t, "Findings", res.List.Title)
	require.Len(t, res.List.Items, 1)
	assert.Equal(t, "auth.go:12", res.List.Items[0].Label)
	assert.Equal(t, "fix auth.go:12", res.List.Items[0].Submit)
}

func TestCommandHookSession(t *testing.T) {
	start := &CommandHook{
		name:    "on-start",
		kind:    KindSessionStart,
		runPath: testScript(t, "session_start.sh"),
		dir:     filepath.Dir(testScript(t, "session_start.sh")),
		timeout: 5 * time.Second,
	}
	res, err := start.Session(t.Context(), SessionEvent{Kind: KindSessionStart, Reason: "startup"})
	require.NoError(t, err)
	assert.Equal(t, "session ready", res.Toast)
	require.True(t, res.StatusSet)
	assert.Equal(t, "hooks on", res.Status)

	deny := &CommandHook{
		name:    "guard",
		kind:    KindSessionBeforeSwitch,
		runPath: testScript(t, "session_deny.sh"),
		dir:     filepath.Dir(testScript(t, "session_deny.sh")),
		timeout: 5 * time.Second,
	}
	res, err = deny.Session(t.Context(), SessionEvent{Kind: KindSessionBeforeSwitch, Reason: "new"})
	require.NoError(t, err)
	assert.Equal(t, ActionDeny, res.Action)
	assert.Equal(t, "dirty repo", res.Reason)
}

func TestCommandHookCommandWrongKind(t *testing.T) {
	h := preHook(t, "allow.sh", "*", 0)
	res, err := h.Command(t.Context(), CommandEvent{})
	require.NoError(t, err)
	assert.Equal(t, CommandResult{}, res)
}

func TestManagerRunCommand(t *testing.T) {
	m := NewManager(
		Entry{Hook: FuncHook{HookName: "skip-me"}, Kind: KindPreTool},
		Entry{Hook: FuncHook{
			HookName: "Review",
			Cmd: func(_ context.Context, ev CommandEvent) (CommandResult, error) {
				return CommandResult{Submit: strings.Join(ev.Args, " ")}, nil
			},
		}, Kind: KindCommand},
	)
	require.Len(t, m.CommandEntries(), 1)

	res, err := m.RunCommand(t.Context(), "review", CommandEvent{Args: []string{"one", "two"}})
	require.NoError(t, err)
	assert.Equal(t, "one two", res.Submit)

	_, err = m.RunCommand(t.Context(), "missing", CommandEvent{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestManagerRunCommandNil(t *testing.T) {
	var m *Manager
	assert.Nil(t, m.CommandEntries())
	_, err := m.RunCommand(t.Context(), "x", CommandEvent{})
	require.Error(t, err)
}
