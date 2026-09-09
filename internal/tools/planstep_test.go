package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tools/lsptool"
	"github.com/alvnukov/cozyphi/internal/tools/webtool"
)

// gatedToolSet assembles every model-facing tool the plan gate injects
// plan_step into, built the way the engine builds them but with the minimal
// dependencies a decode-only call needs. Exempt tools never see the argument
// and are left out; a tool added to the engine without being listed here is
// caught by nothing, so the list follows engine.buildToolListFor.
func gatedToolSet(t *testing.T) []tools.Tool {
	t.Helper()
	set := tools.DefaultTools()
	set = append(set, tools.MCPTools(mcp.NewPool(nil))...)
	set = append(set, webtool.Tool(webtool.Deps{})...)
	set = append(set, lsptool.Tool(nil))
	var gated []tools.Tool
	for _, tool := range set {
		if !plangate.IsExempt(tool.Definition.Name) {
			gated = append(gated, tool)
		}
	}
	return gated
}

// The plan gate lists plan_step in the schema of every gated tool and marks
// it required, then leaves it in the arguments the tool decodes. A strict
// decoder without a slot for it turns the call the model was told to make
// into an unknown-field error, so every gated tool must reserve the name.
func TestGatedToolsAcceptPlanStep(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	seen := map[string]bool{}
	for _, tool := range gatedToolSet(t) {
		seen[tool.Definition.Name] = true
		t.Run(tool.Definition.Name, func(t *testing.T) {
			_, err := tool.Run(ctx, json.RawMessage(`{"plan_step":"probe"}`))
			if err != nil && strings.Contains(err.Error(), `unknown field "plan_step"`) {
				t.Fatalf("%s rejects the plan_step the gate requires: %v", tool.Definition.Name, err)
			}
		})
	}
	for _, name := range []string{"bash", "read", "write", "edit", "grep", "ls", "find", "web", "lsp", "mcp_call"} {
		if !seen[name] {
			t.Errorf("gated tool %q is missing from the test set; keep gatedToolSet in step with the engine", name)
		}
	}
}
