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
// whose repository has one. Without a registry there is no tool and no
// prompt line about it, so a model is never told to reach for what it
// cannot call.
func TestTaskToolFollowsTheRegistry(t *testing.T) {
	root := t.TempDir()
	reg := tasks.Open(root, filepath.Join(root, tasks.DefaultDir))

	for _, tc := range []struct {
		name     string
		reg      *tasks.Registry
		declared bool
	}{
		{"with a registry", reg, true},
		{"without one", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, bodies := recordingServer(t, func(int, http.ResponseWriter) {})
			engine, err := NewEngine(EngineOpts{
				Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
				SessionOpts: SessionOpts{Cwd: t.TempDir()},
				Gate:        permission.AllowAll{},
				Tasks:       tc.reg,
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
