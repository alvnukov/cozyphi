package mcptool_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/phi/internal/mcp"
	"github.com/pulseaiclub/phi/internal/tools/mcptool"
)

func buildEcho(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "echoserver")
	root := moduleRoot(t)
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/echoserver")
	cmd.Dir = filepath.Join(root, "internal", "mcp")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build echoserver: %v\n%s", err, out)
	}
	return bin
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func TestMCPToolsListInspectCall(t *testing.T) {
	t.Setenv("PHI_MCP_LOG_DIR", t.TempDir())
	bin := buildEcho(t)
	pool := mcp.NewPool(map[string]mcp.ServerConfig{
		"echo": {Command: []string{bin}},
	})
	defer func() { _ = pool.Close() }()

	tools := mcptool.Tools(pool)
	if len(tools) != 3 {
		t.Fatalf("tools = %d", len(tools))
	}
	byName := map[string]int{}
	for i, tool := range tools {
		byName[tool.Definition.Name] = i
	}
	for _, name := range []string{"mcp_list", "mcp_inspect", "mcp_call"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing %s", name)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	list := tools[byName["mcp_list"]]
	res, err := list.Run(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "echo") {
		t.Fatalf("list servers = %q", res.Content)
	}

	res, err = list.Run(ctx, json.RawMessage(`{"server":"echo"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "echo") || !strings.Contains(res.Content, "add") {
		t.Fatalf("list tools = %q", res.Content)
	}

	inspect := tools[byName["mcp_inspect"]]
	res, err = inspect.Run(ctx, json.RawMessage(`{"server":"echo","tool":"echo"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "message:s*") {
		t.Fatalf("inspect = %q", res.Content)
	}

	call := tools[byName["mcp_call"]]
	res, err = call.Run(ctx, json.RawMessage(`{"server":"echo","tool":"echo","args":{"message":"ok"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "ok") {
		t.Fatalf("call = %q", res.Content)
	}
}

func TestMCPToolsNilPool(t *testing.T) {
	if mcptool.Tools(nil) != nil {
		t.Fatal("expected nil")
	}
}
