package agent_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pulseaiclub/phi/internal/agent"
	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/mcp"
)

func TestEngineRegistersMCPMetaTools(t *testing.T) {
	t.Setenv("PHI_MCP_LOG_DIR", t.TempDir())
	bin := buildEcho(t)
	pool := mcp.NewPool(map[string]mcp.ServerConfig{
		"echo": {Command: []string{bin}},
	})
	defer func() { _ = pool.Close() }()

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

func buildEcho(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "echoserver")
	root := moduleRoot(t)
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/echoserver")
	cmd.Dir = filepath.Join(root, "internal", "mcp")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
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
