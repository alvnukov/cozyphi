package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/agent"
	"github.com/pulseaiclub/phi/internal/hooks"
	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tools"
)

// End-to-end S8 path: Discover/Load → Manager → Executor rejects bash.
func TestLoadedHooksDenyBash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell hook fixture")
	}

	userDir := t.TempDir()
	hookDir := filepath.Join(userDir, "guard-bash")
	require.NoError(t, os.MkdirAll(hookDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hookDir, hooks.ManifestFileName), []byte(`{
  "name": "guard-bash",
  "event": "pre_tool",
  "match": "bash",
  "run": "./run.sh",
  "fail_closed": true,
  "timeout": "5s"
}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(hookDir, "run.sh"), []byte(`#!/bin/sh
input=$(cat)
echo "$input" | grep -q 'rm -rf' && {
  echo '{"action":"deny","reason":"refusing rm -rf"}'
  exit 2
}
echo '{"action":"allow"}'
`), 0o755))

	mgr, warns, err := hooks.Load(userDir, "")
	require.NoError(t, err)
	assert.Empty(t, warns)

	var ran int
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran++
				return tools.Result{Content: "should not run"}, nil
			},
		},
	}
	ex := agent.NewExecutor(reg, permission.AllowAll{}, nil, mgr)
	var statuses []session.ToolStatus
	msgs := ex.Run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"rm -rf /tmp/x"}`},
	}}, func(td session.ToolData) bool {
		statuses = append(statuses, td.Run.Status)
		return true
	})

	assert.Equal(t, 0, ran)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "refusing rm -rf")
	found := false
	for _, s := range statuses {
		if s == session.ToolRejected {
			found = true
		}
	}
	assert.True(t, found)
}

func TestLoadedHooksOffAllows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell hook fixture")
	}
	userDir := t.TempDir()
	hookDir := filepath.Join(userDir, "guard-bash")
	require.NoError(t, os.MkdirAll(hookDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hookDir, hooks.ManifestFileName), []byte(`{
  "event": "pre_tool",
  "match": "bash",
  "run": "./run.sh",
  "fail_closed": true
}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(hookDir, "run.sh"), []byte(`#!/bin/sh
echo '{"action":"deny","reason":"should not load"}'
exit 2
`), 0o755))

	t.Setenv(hooks.EnvHooks, "off")
	mgr, _, err := hooks.Load(userDir, "")
	require.NoError(t, err)

	var ran int
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran++
				return tools.Result{Content: "ok"}, nil
			},
		},
	}
	ex := agent.NewExecutor(reg, permission.AllowAll{}, nil, mgr)
	msgs := ex.Run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"rm -rf /tmp/x"}`},
	}}, func(session.ToolData) bool { return true })

	assert.Equal(t, 1, ran)
	require.Len(t, msgs, 1)
	assert.Equal(t, "ok", msgs[0].Content)
}
