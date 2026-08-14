package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeHookTree(t *testing.T, root, name, body string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, ManifestFileName)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o755))
	return dir
}

func TestProjectHooksDir(t *testing.T) {
	assert.Equal(t, "/tmp/proj/.phi/hooks", ProjectHooksDir("/tmp/proj"))
	assert.Empty(t, ProjectHooksDir(""))
}

func TestDiscoverUserAndProjectShadow(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	userDir := filepath.Join(home, "hooks")
	projDir := ProjectHooksDir(cwd)

	writeHookTree(t, userDir, "guard-bash", `{
  "name": "guard-bash",
  "event": "pre_tool",
  "match": "bash",
  "run": "./run.sh",
  "fail_closed": false
}`)
	writeHookTree(t, userDir, "audit", `{
  "name": "audit",
  "event": "post_tool",
  "run": "./run.sh",
  "async": true
}`)
	// Project shadows guard-bash with fail_closed true.
	writeHookTree(t, projDir, "guard-bash", `{
  "name": "guard-bash",
  "event": "pre_tool",
  "match": "bash",
  "run": "./run.sh",
  "fail_closed": true
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
	assert.Equal(t, filepath.Join(projDir, "guard-bash", "run.sh"), guard.RunPath)

	audit := byName["audit"]
	assert.Equal(t, SourceUser, audit.Source)
	assert.True(t, audit.Manifest.Async)
}

func TestDiscoverDisabledSkipped(t *testing.T) {
	userDir := t.TempDir()
	writeHookTree(t, userDir, "off", `{
  "event": "pre_tool",
  "run": "./run.sh",
  "disabled": true
}`)
	writeHookTree(t, userDir, "on", `{
  "event": "pre_tool",
  "run": "./run.sh"
}`)

	found, warns, err := Discover(userDir, "")
	require.NoError(t, err)
	assert.Empty(t, warns)
	require.Len(t, found, 1)
	assert.Equal(t, "on", found[0].Manifest.Name)
}

func TestDiscoverBadJSONWarning(t *testing.T) {
	userDir := t.TempDir()
	writeHookTree(t, userDir, "good", `{"event":"pre_tool","run":"./run.sh"}`)
	badDir := filepath.Join(userDir, "bad")
	require.NoError(t, os.MkdirAll(badDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(badDir, ManifestFileName), []byte(`{`), 0o644))

	found, warns, err := Discover(userDir, "")
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0].String(), "bad")
	assert.Contains(t, warns[0].Message, "parse")
}

func TestDiscoverSkipsDirWithoutManifest(t *testing.T) {
	userDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(userDir, "empty"), 0o755))
	writeHookTree(t, userDir, "ok", `{"event":"pre_tool","run":"./run.sh"}`)

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
	writeHookTree(t, userDir, "guard", `{"event":"pre_tool","run":"./run.sh"}`)
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

	dir := filepath.Join(userDir, "abs")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	body := `{"event":"pre_tool","run":` + mustJSONString(absRun) + `}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(body), 0o644))

	found, warns, err := Discover(userDir, "")
	require.NoError(t, err)
	assert.Empty(t, warns)
	require.Len(t, found, 1)
	assert.Equal(t, filepath.Clean(absRun), found[0].RunPath)
}

func TestDiscoverForCwd(t *testing.T) {
	homeHooks := t.TempDir()
	cwd := t.TempDir()
	writeHookTree(t, homeHooks, "u", `{"event":"pre_tool","run":"./run.sh"}`)
	writeHookTree(t, ProjectHooksDir(cwd), "p", `{"event":"post_tool","run":"./run.sh"}`)

	found, warns, err := DiscoverForCwd(homeHooks, cwd)
	require.NoError(t, err)
	assert.Empty(t, warns)
	require.Len(t, found, 2)
}

func mustJSONString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
