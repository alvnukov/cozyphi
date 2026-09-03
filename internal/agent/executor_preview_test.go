package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// TestExecutorAskCarriesTheWritePreview: the request the ask handler sees
// carries the diff of the pending write, so the overlay can show the user
// the change they are approving instead of a bare path.
func TestExecutorAskCarriesTheWritePreview(t *testing.T) {
	dir := t.TempDir()
	reg := tools.Registry{
		"write": {
			Definition: llm.ToolDefinition{Name: "write"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: "ok"}, nil
			},
		},
	}
	var got permission.Request
	ask := func(_ context.Context, req permission.Request, _ string) (permission.AskResult, error) {
		got = req
		return permission.AskResult{Approved: true}, nil
	}
	ex := NewExecutor(reg, fixedGate{dec: permission.Ask, reason: "needs approval"}, ask, nil)
	ex.SetMeta("s1", dir)

	args := fmt.Sprintf(`{"path":%q,"content":"hello\n"}`, filepath.Join(dir, "n.txt"))
	_, _, _ = ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "write", Arguments: args},
	}}, func(session.ToolData) bool { return true })

	if !strings.Contains(got.Preview, "+hello") {
		t.Fatalf("the ask request must carry the write diff, got %q", got.Preview)
	}
}
