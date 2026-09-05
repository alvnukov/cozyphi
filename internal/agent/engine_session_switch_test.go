package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tools/editledger"
	"github.com/alvnukov/cozyphi/internal/tools/writetool"
	"github.com/alvnukov/cozyphi/internal/util"
)

func TestReplaceSessionRetiresEditCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name    string
		toolset []tools.Tool
	}{
		{name: "implicit defaults"},
		{name: "explicit defaults", toolset: tools.DefaultTools()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			const original = "one\ntwo\nthree"
			path := filepath.Join(dir, "note.txt")
			require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

			engine := newSessionSwitchEngine(t, dir, tc.toolset)
			readEditable(t, engine, path)

			require.NoError(t, engine.ReplaceSession(SessionOpts{Cwd: dir}))
			_, err := runSessionTool(t, engine, "edit", editPayload(t, path, original, "two", "TWO"))
			require.Error(t, err)
			var refusal *writetool.EditRefusal
			require.True(t, errors.As(err, &refusal))
			require.Equal(t, editledger.NoCapability.Code(), refusal.Code)
			requireSessionFileContent(t, path, original)

			// A fresh observation belongs to the replacement session and edits
			// normally; a following write's successor grant does too.
			readEditable(t, engine, path)
			_, err = runSessionTool(t, engine, "edit", editPayload(t, path, original, "two", "TWO"))
			require.NoError(t, err)

			const written = "alpha\nbeta\ngamma"
			writeInput, marshalErr := json.Marshal(map[string]string{"path": path, "content": written})
			require.NoError(t, marshalErr)
			_, err = runSessionTool(t, engine, "write", writeInput)
			require.NoError(t, err)
			_, err = runSessionTool(t, engine, "edit", editPayload(t, path, written, "beta", "BETA"))
			require.NoError(t, err)
			requireSessionFileContent(t, path, "alpha\nBETA\ngamma")
		})
	}
}

func TestRefreshToolsKeepsCurrentSessionEditCapabilities(t *testing.T) {
	dir := t.TempDir()
	const original = "one\ntwo\nthree"
	path := filepath.Join(dir, "note.txt")
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	engine := newSessionSwitchEngine(t, dir, nil)
	readEditable(t, engine, path)
	engine.RefreshTools()

	_, err := runSessionTool(t, engine, "edit", editPayload(t, path, original, "two", "TWO"))
	require.NoError(t, err)
	requireSessionFileContent(t, path, "one\nTWO\nthree")
}

func TestReplaceSessionKeepsExplicitEmptyToolsetEmpty(t *testing.T) {
	engine := newSessionSwitchEngine(t, t.TempDir(), []tools.Tool{})
	require.NoError(t, engine.ReplaceSession(SessionOpts{Cwd: t.TempDir()}))
	engine.SetMode(ModePlan)

	for _, name := range []string{"read", "edit", "write", "grep", "bash"} {
		require.Falsef(t, engine.HasTool(name), "explicit empty toolset gained %s", name)
	}
}

func TestReplaceSessionKeepsUnmarkedCustomRead(t *testing.T) {
	runs := 0
	customRead := tools.Tool{
		Definition: llm.ToolDefinition{Name: "read"},
		Run: func(context.Context, json.RawMessage) (tools.Result, error) {
			runs++
			return tools.Result{Content: "custom read"}, nil
		},
	}
	engine := newSessionSwitchEngine(t, t.TempDir(), []tools.Tool{customRead})
	require.NoError(t, engine.ReplaceSession(SessionOpts{Cwd: t.TempDir()}))

	result, err := runSessionTool(t, engine, "read", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "custom read", result.Content)
	require.Equal(t, 1, runs)
}

func TestNewEngineRetiresSharedDefaultToolsCapabilities(t *testing.T) {
	dir := t.TempDir()
	const original = "one\ntwo\nthree"
	path := filepath.Join(dir, "note.txt")
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	shared := tools.DefaultTools()
	first := newSessionSwitchEngine(t, dir, shared)
	readEditable(t, first, path)

	second := newSessionSwitchEngine(t, dir, shared)
	_, err := runSessionTool(t, second, "edit", editPayload(t, path, original, "two", "TWO"))
	require.Error(t, err)
	var refusal *writetool.EditRefusal
	require.True(t, errors.As(err, &refusal))
	require.Equal(t, editledger.NoCapability.Code(), refusal.Code)
	requireSessionFileContent(t, path, original)
}

func newSessionSwitchEngine(t *testing.T, dir string, toolset []tools.Tool) *Engine {
	t.Helper()
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "test", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: dir},
		Tools:       toolset,
	})
	require.NoError(t, err)
	return engine
}

func readEditable(t *testing.T, engine *Engine, path string) {
	t.Helper()
	input, err := json.Marshal(map[string]string{"path": path, "mode": "edit"})
	require.NoError(t, err)
	_, err = runSessionTool(t, engine, "read", input)
	require.NoError(t, err)
}

func runSessionTool(t *testing.T, engine *Engine, name string, input json.RawMessage) (tools.Result, error) {
	t.Helper()
	tool, ok := engine.executor.registry[name]
	require.True(t, ok)
	return tool.Run(tools.WithCwd(t.Context(), engine.SessionCwd()), input)
}

func editPayload(t *testing.T, path, content, source, replacement string) json.RawMessage {
	t.Helper()
	input, err := json.Marshal(writetool.EditInput{
		Path: path,
		Hash: util.ComputeFileHash(content),
		Edits: []writetool.FlatEdit{{
			From:    "2#" + util.ComputeLineHash(source),
			To:      "2#" + util.ComputeLineHash(source),
			Content: &replacement,
		}},
	})
	require.NoError(t, err)
	return input
}

func requireSessionFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}
