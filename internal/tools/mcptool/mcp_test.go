package mcptool_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/phi/internal/mcp"
	"github.com/pulseaiclub/phi/internal/tools/mcptool"
	"github.com/pulseaiclub/phi/internal/tools/tooldef"
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

func TestMCPListRequiresServer(t *testing.T) {
	pool := mcp.NewPool(map[string]mcp.ServerConfig{
		"echo": {Command: []string{"true"}},
	})
	list := findTool(t, mcptool.Tools(pool), "mcp_list")
	req := list.Definition.Params.Required
	if len(req) != 1 || req[0] != "server" {
		t.Fatalf("Required = %v, want [server]", req)
	}
	_, err := list.Run(t.Context(), []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "server is required") {
		t.Fatalf("err = %v, want server is required", err)
	}
}

func findTool(t *testing.T, tools []tooldef.Tool, name string) tooldef.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Definition.Name == name {
			return tool
		}
	}
	t.Fatalf("missing %s", name)
	return tooldef.Tool{}
}
