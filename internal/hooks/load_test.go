package hooks

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadBuildsManager(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell run.sh")
	}
	userDir := t.TempDir()
	dir := writeHookTree(t, userDir, "guard-bash", `{
  "name": "guard-bash",
  "event": "pre_tool",
  "match": "bash",
  "run": "./run.sh",
  "fail_closed": true
}`)
	// Replace run.sh with deny-on-rm script.
	script := `#!/bin/sh
input=$(cat)
echo "$input" | grep -q 'rm -rf' && {
  echo '{"action":"deny","reason":"refusing rm -rf"}'
  exit 2
}
echo '{"action":"allow"}'
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.sh"), []byte(script), 0o755))

	mgr, warns, err := Load(userDir, "")
	require.NoError(t, err)
	assert.Empty(t, warns)
	require.NotNil(t, mgr)
}

func TestLoadPHIHooksOff(t *testing.T) {
	userDir := t.TempDir()
	writeHookTree(t, userDir, "x", `{"event":"pre_tool","run":"./run.sh"}`)
	t.Setenv(EnvHooks, "off")
	mgr, warns, err := Load(userDir, "")
	require.NoError(t, err)
	assert.Empty(t, warns)
	require.NotNil(t, mgr)
}

func TestFormatWarningsSummary(t *testing.T) {
	assert.Empty(t, FormatWarningsSummary(nil))
	assert.Contains(t, FormatWarningsSummary([]Warning{{Path: "a", Message: "b"}}), "1 warning")
}
