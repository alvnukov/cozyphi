package hooks

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeHookJSON(t *testing.T, dir, body string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, ManifestFileName)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestParseManifestOK(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "guard-bash")
	path := writeHookJSON(t, dir, `{
  "name": "guard-bash",
  "event": "pre_tool",
  "match": "bash",
  "run": "./run.sh",
  "timeout": "5s",
  "fail_closed": true
}`)

	m, err := ParseManifest(path)
	require.NoError(t, err)
	assert.Equal(t, "guard-bash", m.Name)
	assert.Equal(t, KindPreTool, m.Kind)
	assert.Equal(t, "bash", m.Match)
	assert.Equal(t, "./run.sh", m.Run)
	assert.Equal(t, 5*time.Second, m.Timeout)
	assert.True(t, m.FailClosed)
	assert.False(t, m.Async)
	assert.False(t, m.Disabled)
	assert.Equal(t, dir, m.Dir)
	assert.Equal(t, path, m.Path)
}

func TestParseManifestDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audit")
	path := writeHookJSON(t, dir, `{
  "event": "post_tool",
  "run": "./audit.py",
  "async": true
}`)

	m, err := ParseManifest(path)
	require.NoError(t, err)
	assert.Equal(t, "audit", m.Name, "name defaults to directory basename")
	assert.Equal(t, "*", m.Match)
	assert.Equal(t, defaultTimeout, m.Timeout)
	assert.True(t, m.Async)
	assert.Equal(t, KindPostTool, m.Kind)
}

func TestParseManifestTimeoutNumberSeconds(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "t")
	path := writeHookJSON(t, dir, `{"event":"pre_tool","run":"./r","timeout":10}`)
	m, err := ParseManifest(path)
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, m.Timeout)
}

func TestParseManifestErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing run",
			body: `{"event":"pre_tool","match":"bash"}`,
			want: "missing required field \"run\"",
		},
		{
			name: "missing event",
			body: `{"run":"./r"}`,
			want: "missing required field \"event\"",
		},
		{
			name: "bad event",
			body: `{"event":"tool.call","run":"./r"}`,
			want: "invalid event",
		},
		{
			name: "bad timeout string",
			body: `{"event":"pre_tool","run":"./r","timeout":"nope"}`,
			want: "invalid timeout",
		},
		{
			name: "timeout too large",
			body: `{"event":"pre_tool","run":"./r","timeout":"61s"}`,
			want: "max is",
		},
		{
			name: "async on pre",
			body: `{"event":"pre_tool","run":"./r","async":true}`,
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
			dir := filepath.Join(t.TempDir(), "h")
			path := writeHookJSON(t, dir, tt.body)
			_, err := ParseManifest(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestParseManifestDisabled(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "off")
	path := writeHookJSON(t, dir, `{
  "event": "pre_tool",
  "run": "./run.sh",
  "disabled": true
}`)
	m, err := ParseManifest(path)
	require.NoError(t, err)
	assert.True(t, m.Disabled)
}

func TestParseManifestFileMissing(t *testing.T) {
	_, err := ParseManifest(filepath.Join(t.TempDir(), "missing", ManifestFileName))
	require.Error(t, err)
}
