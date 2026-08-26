package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wireParamsFor unmarshals the recorded params for the first occurrence of
// method in the fake server's params log.
func wireParamsFor(t *testing.T, path, method string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	for line := range strings.SplitSeq(string(raw), "\n") {
		name, params, ok := strings.Cut(line, "\t")
		if !ok || name != method {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(params), &m))
		return m
	}
	t.Fatalf("method %s not found in %s", method, path)
	return nil
}

// TestSettingsAndInitOptionsReachOnlyDefinedFields proves the frozen wiring:
// initialization options appear only inside initialize, settings appear only
// in didChangeConfiguration and configuration replies, and env additions
// never reach the wire.
func TestSettingsAndInitOptionsReachOnlyDefinedFields(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	params := filepath.Join(t.TempDir(), "params")
	configOut := filepath.Join(t.TempDir(), "config-out")
	cfg := fakeConfig(
		"LSP_TEST_PARAMS="+params,
		"LSP_TEST_DEF_RESULT="+defFixture(uriFromPath(otherFile)),
		"LSP_TEST_ASK_CONFIG=gopls.hints,missing.section",
		"LSP_TEST_CONFIG_OUT="+configOut,
	)
	cfg.Gopls.InitializationOptions = map[string]any{"buildFlags": []any{"-tags=e2e"}}
	cfg.Gopls.Settings = map[string]any{
		"gopls": map[string]any{"hints": map[string]any{"assignVariableTypes": true}},
	}
	cfg.Gopls.Env = append(cfg.Gopls.Env, "TEST_SECRET=DONTLEAK")

	mgr, err := Open(t.Context(), dir, cfg)
	require.NoError(t, err)
	_, err = mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
	require.NoError(t, err)
	require.NoError(t, mgr.Close(t.Context()))

	init := wireParamsFor(t, params, "initialize")
	assert.Equal(t, map[string]any{"buildFlags": []any{"-tags=e2e"}}, init["initializationOptions"])
	_, hasSettings := init["settings"]
	assert.False(t, hasSettings, "settings must not ride initialize")

	dcc := wireParamsFor(t, params, "workspace/didChangeConfiguration")
	assert.Equal(t, cfg.Gopls.Settings, dcc["settings"])

	whole, err := os.ReadFile(params)
	require.NoError(t, err)
	assert.NotContains(t, string(whole), "DONTLEAK", "env additions must never reach the wire")

	// The server's own configuration pull is answered from the frozen
	// settings: the requested dotted section resolves, the missing one is null.
	out, err := os.ReadFile(configOut)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	require.Len(t, lines, 2)
	have, missing, _ := strings.Cut(lines[0], "\t")
	assert.Equal(t, "gopls.hints", have)
	assert.JSONEq(t, `{"assignVariableTypes":true}`, missing)
	name2, val2, _ := strings.Cut(lines[1], "\t")
	assert.Equal(t, "missing.section", name2)
	assert.Equal(t, "null", val2)
}
