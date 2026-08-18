package hooks

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePluginJSON(t *testing.T, dir, body string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, PluginFileName)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestParsePluginOK(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "org-policy")
	path := writePluginJSON(t, dir, `{
  "name": "org-policy",
  "hooks": [
    {
      "name": "guard-bash",
      "event": "pre_tool",
      "match": "bash",
      "run": "./guard.sh",
      "timeout": "5s",
      "fail_closed": true
    },
    {
      "name": "audit",
      "event": "post_tool",
      "run": "./audit.py",
      "async": true
    }
  ]
}`)

	ms, err := ParsePlugin(path)
	require.NoError(t, err)
	require.Len(t, ms, 2)

	assert.Equal(t, "guard-bash", ms[0].Name)
	assert.Equal(t, KindPreTool, ms[0].Kind)
	assert.Equal(t, "bash", ms[0].Match)
	assert.Equal(t, "./guard.sh", ms[0].Run)
	assert.Equal(t, 5*time.Second, ms[0].Timeout)
	assert.True(t, ms[0].FailClosed)
	assert.Equal(t, "org-policy", ms[0].Plugin)
	assert.Equal(t, dir, ms[0].Dir)
	assert.Equal(t, path, ms[0].Path)

	assert.Equal(t, "audit", ms[1].Name)
	assert.Equal(t, KindPostTool, ms[1].Kind)
	assert.Equal(t, "*", ms[1].Match)
	assert.True(t, ms[1].Async)
	assert.Equal(t, defaultTimeout, ms[1].Timeout)
}

func TestParsePluginTopLevelArray(t *testing.T) {
	dir := t.TempDir()
	path := writePluginJSON(t, dir, `[
  {"name":"a","event":"pre_tool","run":"./a.sh"},
  {"name":"b","event":"post_tool","run":"./b.sh"}
]`)
	ms, err := ParsePlugin(path)
	require.NoError(t, err)
	require.Len(t, ms, 2)
	assert.Equal(t, "a", ms[0].Name)
	assert.Equal(t, "b", ms[1].Name)
	assert.Equal(t, filepath.Base(dir), ms[0].Plugin)
}

func TestParsePluginSingleHookNameDefaultsToPlugin(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "guard")
	path := writePluginJSON(t, dir, `{
  "name": "guard",
  "hooks": [{"event":"pre_tool","run":"./run.sh"}]
}`)
	ms, err := ParsePlugin(path)
	require.NoError(t, err)
	require.Len(t, ms, 1)
	assert.Equal(t, "guard", ms[0].Name)
}

func TestParsePluginTimeoutNumberSeconds(t *testing.T) {
	dir := t.TempDir()
	path := writePluginJSON(t, dir, `{"hooks":[{"name":"t","event":"pre_tool","run":"./r","timeout":10}]}`)
	ms, err := ParsePlugin(path)
	require.NoError(t, err)
	require.Len(t, ms, 1)
	assert.Equal(t, 10*time.Second, ms[0].Timeout)
}

func TestParsePluginErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing hooks",
			body: `{"name":"x"}`,
			want: "missing hooks",
		},
		{
			name: "empty hooks",
			body: `{"hooks":[]}`,
			want: "missing hooks",
		},
		{
			name: "missing run",
			body: `{"hooks":[{"name":"h","event":"pre_tool"}]}`,
			want: "missing required field \"run\"",
		},
		{
			name: "missing event",
			body: `{"hooks":[{"name":"h","run":"./r"}]}`,
			want: "missing required field \"event\"",
		},
		{
			name: "missing name among many",
			body: `{"hooks":[{"event":"pre_tool","run":"./a"},{"name":"b","event":"pre_tool","run":"./b"}]}`,
			want: "missing required field \"name\"",
		},
		{
			name: "duplicate name",
			body: `{"hooks":[{"name":"h","event":"pre_tool","run":"./a"},{"name":"h","event":"post_tool","run":"./b"}]}`,
			want: "duplicate hook name",
		},
		{
			name: "bad event",
			body: `{"hooks":[{"name":"h","event":"tool.call","run":"./r"}]}`,
			want: "invalid event",
		},
		{
			name: "bad timeout string",
			body: `{"hooks":[{"name":"h","event":"pre_tool","run":"./r","timeout":"nope"}]}`,
			want: "invalid timeout",
		},
		{
			name: "timeout too large",
			body: `{"hooks":[{"name":"h","event":"pre_tool","run":"./r","timeout":"61s"}]}`,
			want: "max is",
		},
		{
			name: "async on pre",
			body: `{"hooks":[{"name":"h","event":"pre_tool","run":"./r","async":true}]}`,
			want: "async is only valid",
		},
		{
			name: "invalid json",
			body: `{`,
			want: "parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writePluginJSON(t, dir, tt.body)
			_, err := ParsePlugin(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestParsePluginDisabled(t *testing.T) {
	dir := t.TempDir()
	path := writePluginJSON(t, dir, `{
  "hooks": [{"name":"off","event":"pre_tool","run":"./run.sh","disabled":true}]
}`)
	ms, err := ParsePlugin(path)
	require.NoError(t, err)
	require.Len(t, ms, 1)
	assert.True(t, ms[0].Disabled)
}

func TestParsePluginFileMissing(t *testing.T) {
	_, err := ParsePlugin(filepath.Join(t.TempDir(), "missing", PluginFileName))
	require.Error(t, err)
}
