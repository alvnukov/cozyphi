package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePlugin(t *testing.T, dir, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, PluginFileName), []byte(body), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o755))
}

func TestDiscoverUserAndProjectShadow(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	userDir := filepath.Join(home, "hooks")
	projDir := filepath.Join(cwd, ".phi", "hooks")

	writePlugin(t, userDir, `{
  "hooks": [
    {"name":"guard-bash","event":"pre_tool","match":"bash","run":"./run.sh","fail_closed":false},
    {"name":"audit","event":"post_tool","run":"./run.sh","async":true}
  ]
}`)
	writePlugin(t, projDir, `{
  "hooks": [
    {"name":"guard-bash","event":"pre_tool","match":"bash","run":"./run.sh","fail_closed":true}
  ]
}`)

	found, warns, err := Discover(userDir, projDir)
	require.NoError(t, err)
	assert.Empty(t, warns)
	require.Len(t, found, 2)

	byName := map[string]Discovered{}
	for _, d := range found {
		byName[d.Manifest.Name] = d
	}

	guard := byName["guard-bash"]
	assert.Equal(t, SourceProject, guard.Source)
	assert.True(t, guard.Manifest.FailClosed)
	assert.True(t, filepath.IsAbs(guard.RunPath))
	assert.Equal(t, filepath.Join(projDir, "run.sh"), guard.RunPath)

	audit := byName["audit"]
	assert.Equal(t, SourceUser, audit.Source)
	assert.True(t, audit.Manifest.Async)
}

func TestDiscoverPluginSubdir(t *testing.T) {
	userDir := t.TempDir()
	writePlugin(t, filepath.Join(userDir, "org"), `{
  "name": "org",
  "hooks": [
    {"name":"guard","event":"pre_tool","run":"./run.sh"},
    {"name":"audit","event":"post_tool","run":"./run.sh"}
  ]
}`)

	found, warns, err := Discover(userDir, "")
	require.NoError(t, err)
	assert.Empty(t, warns)
	require.Len(t, found, 2)
	for _, d := range found {
		assert.Equal(t, "org", d.Manifest.Plugin)
		assert.Equal(t, filepath.Join(userDir, "org", "run.sh"), d.RunPath)
	}
}

func TestDiscoverDisabledSkipped(t *testing.T) {
	userDir := t.TempDir()
	writePlugin(t, userDir, `{
  "hooks": [
    {"name":"off","event":"pre_tool","run":"./run.sh","disabled":true},
    {"name":"on","event":"pre_tool","run":"./run.sh"}
  ]
}`)

	found, warns, err := Discover(userDir, "")
	require.NoError(t, err)
	assert.Empty(t, warns)
	require.Len(t, found, 1)
	assert.Equal(t, "on", found[0].Manifest.Name)
}

func TestDiscoverBadJSONWarning(t *testing.T) {
	userDir := t.TempDir()
	writePlugin(t, userDir, `{`)

	found, warns, err := Discover(userDir, "")
	require.NoError(t, err)
	assert.Empty(t, found)
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0].Message, "parse")
}

func TestDiscoverDuplicateNameAcrossPlugins(t *testing.T) {
	userDir := t.TempDir()
	writePlugin(t, userDir, `{"hooks":[{"name":"guard","event":"pre_tool","run":"./run.sh"}]}`)
	writePlugin(t, filepath.Join(userDir, "other"), `{"hooks":[{"name":"guard","event":"pre_tool","run":"./run.sh"}]}`)

	found, warns, err := Discover(userDir, "")
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0].Message, "duplicate hook name")
	assert.Equal(t, filepath.Join(userDir, "run.sh"), found[0].RunPath)
}

func TestDiscoverSkipsDirWithoutPlugin(t *testing.T) {
	userDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(userDir, "empty"), 0o755))
	writePlugin(t, userDir, `{"hooks":[{"name":"ok","event":"pre_tool","run":"./run.sh"}]}`)

	found, warns, err := Discover(userDir, "")
	require.NoError(t, err)
	assert.Empty(t, warns)
	require.Len(t, found, 1)
}

func TestDiscoverMissingDirsOK(t *testing.T) {
	found, warns, err := Discover(filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "also-nope"))
	require.NoError(t, err)
	assert.Empty(t, found)
	assert.Empty(t, warns)
}

func TestDiscoverPHIHooksOff(t *testing.T) {
	userDir := t.TempDir()
	writePlugin(t, userDir, `{"hooks":[{"name":"guard","event":"pre_tool","run":"./run.sh"}]}`)
	t.Setenv(EnvHooks, "off")

	found, warns, err := Discover(userDir, "")
	require.NoError(t, err)
	assert.Empty(t, found)
	assert.Empty(t, warns)
	assert.True(t, HooksDisabled())
}

func TestDiscoverAbsoluteRun(t *testing.T) {
	userDir := t.TempDir()
	absRun := filepath.Join(t.TempDir(), "bin", "hook")
	require.NoError(t, os.MkdirAll(filepath.Dir(absRun), 0o755))
	require.NoError(t, os.WriteFile(absRun, []byte("#!/bin/sh\n"), 0o755))

	body := `{"hooks":[{"name":"abs","event":"pre_tool","run":` + mustJSONString(absRun) + `}]}`
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, PluginFileName), []byte(body), 0o644))

	found, warns, err := Discover(userDir, "")
	require.NoError(t, err)
	assert.Empty(t, warns)
	require.Len(t, found, 1)
	assert.Equal(t, filepath.Clean(absRun), found[0].RunPath)
}

func mustJSONString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
