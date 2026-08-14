package mcptool_test

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/mcp"
	"github.com/pulseaiclub/phi/internal/tools/mcptool"
)

func TestMCPToolsRegister(t *testing.T) {
	pool := mcp.NewPool(map[string]mcp.ServerConfig{
		"echo": {Command: []string{"true"}},
	})
	tools := mcptool.Tools(pool)
	if len(tools) != 3 {
		t.Fatalf("tools = %d", len(tools))
	}
	byName := map[string]bool{}
	for _, tool := range tools {
		byName[tool.Definition.Name] = true
	}
	for _, name := range []string{"mcp_list", "mcp_inspect", "mcp_call"} {
		if !byName[name] {
			t.Fatalf("missing %s", name)
		}
	}
}

func TestMCPToolsNilPool(t *testing.T) {
	if mcptool.Tools(nil) != nil {
		t.Fatal("expected nil")
	}
}
