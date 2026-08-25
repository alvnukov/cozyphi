package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestFakeLSP re-execs the test binary as a minimal framed LSP server. It is
// never part of the normal suite: without LSP_TEST_SERVER=1 it skips.
func TestFakeLSP(t *testing.T) {
	if os.Getenv("LSP_TEST_SERVER") != "1" {
		t.Skip("fake LSP server helper")
	}
	history := os.Getenv("LSP_TEST_HISTORY")
	record := func(method string) {
		if history == "" {
			return
		}
		f, err := os.OpenFile(history, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return
		}
		_, _ = f.WriteString(method + "\n")
		_ = f.Close()
	}

	reader := bufio.NewReader(os.Stdin)
	write := func(msg any) {
		raw, _ := json.Marshal(msg)
		_, _ = os.Stdout.Write(encodeFrame(raw))
	}
	for {
		raw, err := readFrame(reader)
		if err != nil {
			return
		}
		var msg message
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		record(msg.Method)
		switch msg.Method {
		case "initialize":
			write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": map[string]any{
					"capabilities": map[string]any{"textDocumentSync": 1},
				},
			})
		case "shutdown":
			write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": nil})
		case "exit":
			return
		case "textDocument/definition":
			write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result":  json.RawMessage(os.Getenv("LSP_TEST_DEF_RESULT")),
			})
		default:
			// Requests we didn't expect get an empty result.
			if msg.ID != nil && msg.Method != "" {
				write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": nil})
			}
		}
	}
}

// fakeConfig builds a Config that spawns the in-process fake server.
func fakeConfig(env ...string) Config {
	return Config{
		Enabled: true,
		Gopls: GoplsConfig{
			Command: []string{os.Args[0], "-test.run=TestFakeLSP", "--"},
			Env:     append(os.Environ(), append([]string{"LSP_TEST_SERVER=1"}, env...)...),
		},
	}
}

// defFixture returns a JSON definition payload for uri and a simple range.
func defFixture(uri string) string {
	return fmt.Sprintf(`[{"uri":%q,"range":{"start":{"line":2,"character":5},"end":{"line":2,"character":6}}}]`, uri)
}

// history reads the fake server's method history file.
func history(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}
