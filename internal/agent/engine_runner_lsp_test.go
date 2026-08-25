package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/lsp"
)

// TestBuildChildBorrowsLSPQuery pins the shared-runtime seam: every child role
// gets the parent Manager's borrowed QueryFunc without lifecycle authority or
// agent_* tools. Explore and review stay read-only; workers keep their writable
// base set.
func TestBuildChildBorrowsLSPQuery(t *testing.T) {
	q := lsp.QueryFunc(func(context.Context, lsp.Query) (lsp.Result, error) {
		return lsp.Result{}, nil
	})
	runner := EngineRunner{
		Model: llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		LSP:   q,
	}
	cases := []struct {
		name      string
		role      job.Role
		wantWrite bool
	}{
		{name: "explore", role: job.RoleExplore, wantWrite: false},
		{name: "review", role: job.RoleReview, wantWrite: false},
		{name: "worker-a", role: job.RoleWorker, wantWrite: true},
		{name: "worker-b", role: job.RoleWorker, wantWrite: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng, _, err := runner.buildChild(job.Meta{
				Dir:      t.TempDir(),
				Prompt:   tc.name,
				WorkDir:  t.TempDir(),
				ParentID: "parent",
				Role:     tc.role,
			})
			require.NoError(t, err)
			require.True(t, eng.HasTool("lsp"), "child engine must carry the borrowed lsp tool")
			require.False(t, eng.HasTool("agent_spawn"), "children never gain agent_* tools")
			require.Equal(t, tc.wantWrite, eng.HasTool("write"), "write must follow the role boundary")
		})
	}
}
