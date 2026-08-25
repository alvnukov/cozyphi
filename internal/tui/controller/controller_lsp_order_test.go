package controller

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/lsp"
)

// TestLSPShutdownHelper re-execs the test binary as a minimal framed LSP server
// so the shutdown-order test can observe lsp close without a real gopls.
func TestLSPShutdownHelper(t *testing.T) {
	if os.Getenv("COZYPHI_LSP_HELPER") != "1" {
		t.Skip("lsp shutdown helper")
	}
	log := os.Getenv("LSP_ORDER_LOG")
	appendLog := func(s string) {
		f, err := os.OpenFile(log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return
		}
		_, _ = f.WriteString(s + "\n")
		_ = f.Close()
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		raw, err := readLSPFrame(reader)
		if err != nil {
			return
		}
		var msg struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		switch msg.Method {
		case "initialize":
			writeLSPFrame(os.Stdout, map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result":  map[string]any{"capabilities": map[string]any{"definitionProvider": true}},
			})
		case "shutdown":
			appendLog("lsp-shutdown")
			writeLSPFrame(os.Stdout, map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": nil})
		case "exit":
			return
		default:
			if msg.ID != nil {
				writeLSPFrame(os.Stdout, map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": nil})
			}
		}
	}
}

func readLSPFrame(r *bufio.Reader) ([]byte, error) {
	var contentLen int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if v, ok := strings.CutPrefix(line, "Content-Length:"); ok {
			contentLen, _ = strconv.Atoi(strings.TrimSpace(v))
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	body := make([]byte, contentLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func writeLSPFrame(w io.Writer, msg any) {
	raw, _ := json.Marshal(msg)
	_, _ = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(raw))
	_, _ = w.Write(raw)
}

// TestControllerCloseOrdersJobsBeforeLSP pins the shutdown order: live
// jobs/children are cancelled and reaped before the shared LSP manager shuts
// down its process, so a child never observes a dead runtime.
func TestControllerCloseOrdersJobsBeforeLSP(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n"), 0o600))
	mainFile := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(mainFile, []byte("package main\n"), 0o600))

	log := filepath.Join(t.TempDir(), "order.log")
	lspMgr, err := lsp.Open(t.Context(), dir, lsp.Config{
		Enabled: true,
		Gopls: lsp.GoplsConfig{
			Command: []string{os.Args[0], "-test.run=TestLSPShutdownHelper", "--"},
			Env: append(os.Environ(),
				"COZYPHI_LSP_HELPER=1",
				"LSP_ORDER_LOG="+log,
			),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, lspMgr)

	// Warm the live client so Close has a process to shut down.
	_, err = lspMgr.Query(t.Context(), lsp.Query{Op: lsp.OpDefinition, File: mainFile, Line: 1, Character: 1})
	require.NoError(t, err)

	jobMgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
			<-ctx.Done()
			f, ferr := os.OpenFile(log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if ferr == nil {
				_, _ = f.WriteString("job-closed\n")
				_ = f.Close()
			}
			return "", nil
		}),
	})
	require.NoError(t, err)
	_, err = jobMgr.Spawn(t.Context(), job.SpawnRequest{Prompt: "block", WorkDir: dir})
	require.NoError(t, err)

	ctrl := &Controller{bus: NewBus(nil), jobs: jobMgr, lspMgr: lspMgr}
	ctrl.Close()

	raw, err := os.ReadFile(log)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	require.Len(t, lines, 2)
	require.Equal(t, "job-closed", lines[0], "jobs must close before the lsp manager")
	require.Equal(t, "lsp-shutdown", lines[1])
}
