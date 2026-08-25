package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
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

	mainURI := os.Getenv("LSP_TEST_MAIN_URI")
	otherURI := os.Getenv("LSP_TEST_OTHER_URI")
	defResult := func(uri string) json.RawMessage {
		if mainURI != "" && otherURI != "" {
			if uri == mainURI {
				return json.RawMessage(defFixture(otherURI))
			}
			return json.RawMessage(defFixture(mainURI))
		}
		return json.RawMessage(os.Getenv("LSP_TEST_DEF_RESULT"))
	}

	// When set, definition responses are batched so tests can prove
	// out-of-order ID routing and per-request cancellation. The server signals
	// readiness, waits for a go file, then answers in reverse arrival order.
	batch := 0
	if v := os.Getenv("LSP_TEST_DEF_BATCH"); v != "" {
		batch, _ = strconv.Atoi(v)
	}
	readyPath := os.Getenv("LSP_TEST_DEF_READY")
	initGate := os.Getenv("LSP_TEST_INIT_GATE")
	var defIDs []int64
	var defURIs []string

	reader := bufio.NewReader(os.Stdin)
	write := func(msg any) {
		raw, _ := json.Marshal(msg)
		_, _ = os.Stdout.Write(encodeFrame(raw))
	}

	flushBatch := func() {
		for i := range slices.Backward(defIDs) {
			write(map[string]any{
				"jsonrpc": "2.0",
				"id":      defIDs[i],
				"result":  defResult(defURIs[i]),
			})
		}
		defIDs = nil
		defURIs = nil
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
			if initGate != "" {
				_ = os.WriteFile(initGate, []byte("ready"), 0o600)
				waitForFile(initGate + ".go")
			}
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
			var params struct {
				TextDocument struct {
					URI string `json:"uri"`
				} `json:"textDocument"`
			}
			_ = json.Unmarshal(msg.Params, &params)
			if batch <= 0 {
				write(map[string]any{
					"jsonrpc": "2.0",
					"id":      *msg.ID,
					"result":  defResult(params.TextDocument.URI),
				})
				continue
			}
			defIDs = append(defIDs, *msg.ID)
			defURIs = append(defURIs, params.TextDocument.URI)
			if len(defIDs) >= batch {
				if readyPath != "" {
					_ = os.WriteFile(readyPath, []byte("ready"), 0o600)
					waitForFile(readyPath + ".go")
				}
				flushBatch()
			}
		default:
			// Requests we didn't expect get an empty result; notifications are
			// consumed and discarded (diagnostics, cancelRequest, progress).
			if msg.ID != nil && msg.Method != "" {
				write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": nil})
			}
		}
	}
}

// waitForFile polls for path to appear so the fake server and its test can
// rendezvous deterministically without racing on process scheduling. The
// bounded deadline keeps a failed test from leaking a stuck helper process.
func waitForFile(path string) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(5 * time.Millisecond)
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
