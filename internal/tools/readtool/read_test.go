package readtool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunRead_DefaultsToViewOutput(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0o644))
	t.Chdir(root)

	raw, err := json.Marshal(readInput{Path: "src/main.go"})
	require.NoError(t, err)
	out, err := runRead(t.Context(), raw)
	require.NoError(t, err)
	assert.Equal(t, "1|package main\n2|\n", out.Content)
	assert.NotContains(t, out.Content, "@file")
	assert.NotContains(t, out.Content, "#")
	assert.Equal(t, "src/main.go", out.Detail)
}

func TestRunRead_EditModeReturnsHashlineOutput(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0o644))
	t.Chdir(root)

	raw, err := json.Marshal(readInput{Path: "src/main.go", Mode: "edit"})
	require.NoError(t, err)
	out, err := runRead(t.Context(), raw)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out.Content, "@file src/main.go#"))
	assert.Regexp(t, `(?m)^1#[a-z]{3}\|package main$`, out.Content)
	assert.Equal(t, "src/main.go", out.Detail)
}

func TestReadToolSchemaDescribesViewAndEditModes(t *testing.T) {
	tool := ReadTool()
	mode, ok := tool.Definition.Params.Properties["mode"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []string{"view", "edit"}, mode["enum"])
	require.Contains(t, tool.Definition.Description, `mode:"edit"`)
	require.Contains(t, tool.Definition.Description, "N|content")
}
