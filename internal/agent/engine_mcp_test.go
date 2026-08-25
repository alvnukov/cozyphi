package agent_test

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/mcp"
)

func TestEngineRegistersMCPMetaTools(t *testing.T) {
	pool := mcp.NewPool(map[string]mcp.ServerConfig{
		"echo": {Command: []string{"true"}},
	})

	eng, err := agent.NewEngine(agent.EngineOpts{
		Model: llm.ModelConfig{Name: "test", APIKey: "x", BaseURL: "http://127.0.0.1:9"},
		SessionOpts: agent.SessionOpts{
			Cwd:     t.TempDir(),
			Persist: false,
		},
		MCP: pool,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mcp_list", "mcp_inspect", "mcp_call", "bash"} {
		if !eng.HasTool(name) {
			t.Fatalf("missing tool %s", name)
		}
	}

	// Explicit child tool list without MCP → no meta tools (sub-agent path).
	eng2, err := agent.NewEngine(agent.EngineOpts{
		Model: llm.ModelConfig{Name: "test", APIKey: "x", BaseURL: "http://127.0.0.1:9"},
		SessionOpts: agent.SessionOpts{
			Cwd:     t.TempDir(),
			Persist: false,
		},
		Tools: agent.ChildTools(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if eng2.HasTool("mcp_list") {
		t.Fatal("child tools should not include mcp_list")
	}
}
