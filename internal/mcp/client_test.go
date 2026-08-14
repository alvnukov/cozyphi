package mcp_test

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
)

func echoServerCmd(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "echoserver")
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/echoserver")
	cmd.Dir = filepath.Join(findModuleRoot(t), "internal", "mcp")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build echoserver: %v\n%s", err, out)
	}
	return []string{bin}
}

func findModuleRoot(t *testing.T) string {
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

func TestConfigLoadSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := mcp.ServerConfig{
		Transport: "stdio",
		Command:   []string{"npx"},
		Args:      []string{"-y", "pkg"},
	}
	if err := mcp.AddServer("demo", cfg); err != nil {
		t.Fatal(err)
	}

	servers, err := mcp.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := servers["demo"]; !ok {
		t.Fatal("expected demo server")
	}
	if got := servers["demo"].Command; len(got) != 1 || got[0] != "npx" {
		t.Fatalf("command = %v", got)
	}

	ok, err := mcp.RemoveServer("demo")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected remove ok")
	}
}

func TestCompactAndSlim(t *testing.T) {
	if got := mcp.CompactServerList([]string{"a", "b"}); got != "a b" {
		t.Fatalf("got %q", got)
	}
	tools := []mcp.ToolDef{
		{Name: "echo", Description: "Echo back", InputSchema: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`)},
	}
	if got := mcp.CompactToolNames(tools); got != "echo" {
		t.Fatalf("got %q", got)
	}
	slim := mcp.SlimTool(tools[0])
	if !strings.Contains(slim, "echo") || !strings.Contains(slim, "message:s*") {
		t.Fatalf("slim = %q", slim)
	}
}

func TestClientEchoServer(t *testing.T) {
	t.Setenv("PHI_MCP_LOG_DIR", t.TempDir())
	argv := echoServerCmd(t)
	c, err := mcp.NewClient("echo", mcp.ServerConfig{Command: argv})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 {
		t.Fatalf("tools = %d", len(tools))
	}

	out, err := c.CallTool(ctx, "echo", map[string]any{"message": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hi") {
		t.Fatalf("out = %q", out)
	}

	sum, err := c.CallTool(ctx, "add", map[string]any{"a": 2.0, "b": 3.0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sum, "5") {
		t.Fatalf("sum = %q", sum)
	}
}

func TestPoolMetaFlow(t *testing.T) {
	t.Setenv("PHI_MCP_LOG_DIR", t.TempDir())
	argv := echoServerCmd(t)
	pool := mcp.NewPool(map[string]mcp.ServerConfig{
		"echo": {Command: argv},
	})
	defer func() { _ = pool.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if names := pool.ServerNames(); len(names) != 1 || names[0] != "echo" {
		t.Fatalf("names = %v", names)
	}
	tools, err := pool.ListTools(ctx, "echo")
	if err != nil {
		t.Fatal(err)
	}
	if got := mcp.CompactToolNames(tools); got != "echo add" {
		t.Fatalf("compact = %q", got)
	}

	def, err := pool.Inspect(ctx, "echo", "echo")
	if err != nil {
		t.Fatal(err)
	}
	if def.Name != "echo" {
		t.Fatalf("name = %q", def.Name)
	}

	out, err := pool.Call(ctx, "echo", "echo", map[string]any{"message": "phi"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "phi") {
		t.Fatalf("out = %q", out)
	}

	results := pool.Doctor(ctx)
	if len(results) != 1 || !results[0].OK {
		t.Fatalf("doctor = %+v", results)
	}
}

func TestDisabled(t *testing.T) {
	t.Setenv("PHI_MCP", "off")
	if !mcp.Disabled() {
		t.Fatal("expected disabled")
	}
	pool, err := mcp.LoadPool(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if pool != nil {
		t.Fatal("expected nil pool")
	}
}
