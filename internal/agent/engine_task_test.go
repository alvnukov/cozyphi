package agent

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tasks"
)

// TestTaskToolFollowsTheRegistry pins who works the task ledger: the session
// whose repository has one, at the level the user set. Without a registry,
// or with permissions.tasks off, there is no tool and no prompt line about
// it, so a model is never told to reach for what it cannot call.
func TestTaskToolFollowsTheRegistry(t *testing.T) {
	root := t.TempDir()
	reg := tasks.Open(root, filepath.Join(root, tasks.DefaultDir))

	for _, tc := range []struct {
		name     string
		reg      *tasks.Registry
		access   tasks.Access
		declared bool
	}{
		{"with a registry", reg, "", true},
		{"read-only", reg, tasks.AccessRead, true},
		{"switched off", reg, tasks.AccessOff, false},
		{"without one", nil, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, bodies := recordingServer(t, func(int, http.ResponseWriter) {})
			engine, err := NewEngine(EngineOpts{
				Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
				SessionOpts: SessionOpts{Cwd: t.TempDir()},
				Gate:        permission.AllowAll{},
				Tasks:       tc.reg,
				TasksAccess: tc.access,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.declared, engine.HasTool("task"))
			drain(t, engine, "what should I work on")

			require.NotEmpty(t, bodies())
			if tc.declared {
				assert.Contains(t, bodies()[0], `"name":"task"`)
				assert.Contains(t, bodies()[0], "task registry")
			} else {
				assert.NotContains(t, bodies()[0], `"name":"task"`)
				assert.NotContains(t, bodies()[0], "task registry")
			}
		})
	}
}

// TestSetTasksAccessRebindsLive pins the settings row's promise: changing
// the level mid-session changes what the next round carries — the tool
// leaves at off and returns at read, with a schema that offers reads only.
func TestSetTasksAccessRebindsLive(t *testing.T) {
	root := t.TempDir()
	reg := tasks.Open(root, filepath.Join(root, tasks.DefaultDir))
	server, bodies := recordingServer(t, func(int, http.ResponseWriter) {})
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
		Tasks:       reg,
	})
	require.NoError(t, err)
	require.True(t, engine.HasTool("task"))

	engine.SetTasksAccess(tasks.AccessOff)
	assert.False(t, engine.HasTool("task"))

	engine.SetTasksAccess(tasks.AccessRead)
	require.True(t, engine.HasTool("task"))
	drain(t, engine, "what should I work on")
	require.NotEmpty(t, bodies())
	assert.Contains(t, bodies()[0], "read but not change")
	assert.Contains(t, bodies()[0], "permissions.tasks: read")
	assert.NotContains(t, bodies()[0], "take a task: in_progress")
}
