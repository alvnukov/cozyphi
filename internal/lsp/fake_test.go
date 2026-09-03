package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"maps"
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
	// paramsLog records "method<TAB>params" lines so tests can assert exactly
	// what the harness put on the wire (includeDeclaration, opaque data).
	paramsLog := os.Getenv("LSP_TEST_PARAMS")
	recordParams := func(method string, params json.RawMessage) {
		if paramsLog == "" {
			return
		}
		f, err := os.OpenFile(paramsLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return
		}
		_, _ = f.WriteString(method + "\t" + string(params) + "\n")
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

	// Publication specs let tests drive publishDiagnostics from wire events:
	// {"on":"textDocument/didOpen","uri":...,"version":2|"matchDocVersion":
	// true,"docVersion":N,"echo":true,"diagnostics":[...]}. A literal version
	// pins stale publications; matchDocVersion echoes the document version;
	// neither sends an unversioned publication. docVersion gates a spec to
	// the didChange carrying that document version so tests sequence
	// publications one query at a time. echo rewrites each message to
	// "len:<text bytes>" so restart tests can distinguish generations.
	type pubSpec struct {
		On          string           `json:"on"`
		URI         string           `json:"uri"`
		Version     *int             `json:"version"`
		MatchDoc    bool             `json:"matchDocVersion"`
		DocVersion  int              `json:"docVersion"`
		Echo        bool             `json:"echo"`
		Diagnostics []wireDiagnostic `json:"diagnostics"`
	}
	var pubs []pubSpec
	if v := os.Getenv("LSP_TEST_PUBLISH"); v != "" {
		if err := json.Unmarshal([]byte(v), &pubs); err != nil {
			return
		}
	}
	// dieOn kills the fake server on the first matching method so tests can
	// crash a client generation deterministically.
	dieOn := os.Getenv("LSP_TEST_DIE_ON")

	// ASK_CONFIG makes the fake pull workspace/configuration right after
	// initialized; CONFIG_OUT records each requested section with the reply
	// item the client produced, so tests can prove settings reach exactly the
	// configuration channel.
	askConfig := os.Getenv("LSP_TEST_ASK_CONFIG")
	configOut := os.Getenv("LSP_TEST_CONFIG_OUT")
	askSections := map[int64][]string{}
	nextAskID := int64(9001)

	// docInfo extracts the document uri, version, and full text carried by
	// didOpen/didChange params.
	docInfo := func(params json.RawMessage) (uri string, version int, text string) {
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
				Text    string `json:"text"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if json.Unmarshal(params, &p) != nil {
			return "", 0, ""
		}
		uri, version, text = p.TextDocument.URI, p.TextDocument.Version, p.TextDocument.Text
		if len(p.ContentChanges) > 0 {
			text = p.ContentChanges[0].Text
		}
		return uri, version, text
	}

	firePubs := func(method string, params json.RawMessage) {
		uri, version, text := docInfo(params)
		for i := range pubs {
			spec := &pubs[i]
			if spec.On != method {
				continue
			}
			if spec.DocVersion != 0 && spec.DocVersion != version {
				continue
			}
			if spec.URI != "" && spec.URI != uri {
				continue
			}
			outURI := spec.URI
			if outURI == "" {
				outURI = uri
			}
			if outURI == "" {
				continue
			}
			payload := map[string]any{"uri": outURI}
			switch {
			case spec.Version != nil:
				payload["version"] = *spec.Version
			case spec.MatchDoc:
				payload["version"] = version
			}
			diags := make([]map[string]any, 0, len(spec.Diagnostics))
			for _, d := range spec.Diagnostics {
				m := map[string]any{
					"message":  d.Message,
					"severity": d.Severity,
					"range":    d.Range,
				}
				if d.Code != nil {
					m["code"] = d.Code
				}
				if d.Source != "" {
					m["source"] = d.Source
				}
				if spec.Echo {
					m["message"] = fmt.Sprintf("len:%d", len(text))
				}
				diags = append(diags, m)
			}
			payload["diagnostics"] = diags
			write(map[string]any{
				"jsonrpc": "2.0",
				"method":  "textDocument/publishDiagnostics",
				"params":  payload,
			})
		}
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
		// Responses to our own requests carry no method; they feed the
		// configuration-reply log instead of the method history.
		if msg.Method == "" && msg.ID != nil {
			if sections, ok := askSections[*msg.ID]; ok && configOut != "" {
				delete(askSections, *msg.ID)
				var items []json.RawMessage
				if json.Unmarshal(msg.Result, &items) == nil {
					f, ferr := os.OpenFile(configOut, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
					if ferr == nil {
						for i, section := range sections {
							item := "null"
							if i < len(items) {
								item = string(items[i])
							}
							_, _ = f.WriteString(section + "\t" + item + "\n")
						}
						_ = f.Close()
					}
				}
			}
			continue
		}
		record(msg.Method)
		recordParams(msg.Method, msg.Params)
		if dieOn != "" && msg.Method == dieOn {
			return // crash the generation: the client read loop sees EOF
		}
		firePubs(msg.Method, msg.Params)
		switch msg.Method {
		case "initialize":
			if initGate != "" {
				_ = os.WriteFile(initGate, []byte("ready"), 0o600)
				waitForFile(initGate + ".go")
			}
			caps := map[string]any{
				"textDocumentSync":        1,
				"definitionProvider":      true,
				"referencesProvider":      true,
				"implementationProvider":  true,
				"typeDefinitionProvider":  true,
				"hoverProvider":           true,
				"documentSymbolProvider":  true,
				"workspaceSymbolProvider": true,
				"callHierarchyProvider":   true,
			}
			// LSP_TEST_CAPS=minimal advertises nothing so tests can pin the
			// fail-closed unsupported-capability behavior.
			if os.Getenv("LSP_TEST_CAPS") == "minimal" {
				caps = map[string]any{"textDocumentSync": 1}
			}
			// SYNC_KIND overrides the negotiated textDocumentSync (number or
			// options object); EXTRA_CAPS merges extra capabilities.
			if v := os.Getenv("LSP_TEST_SYNC_KIND"); v != "" {
				var raw any
				if json.Unmarshal([]byte(v), &raw) == nil {
					caps["textDocumentSync"] = raw
				}
			}
			if v := os.Getenv("LSP_TEST_EXTRA_CAPS"); v != "" {
				var extra map[string]any
				if json.Unmarshal([]byte(v), &extra) == nil {
					maps.Copy(caps, extra)
				}
			}
			write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result":  map[string]any{"capabilities": caps},
			})
		case "initialized":
			if askConfig != "" {
				sections := strings.Split(askConfig, ",")
				items := make([]map[string]any, 0, len(sections))
				for _, section := range sections {
					items = append(items, map[string]any{"section": section})
				}
				id := nextAskID
				nextAskID++
				askSections[id] = sections
				write(map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"method":  "workspace/configuration",
					"params":  map[string]any{"items": items},
				})
			}
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
				if readyPath == "" {
					flushBatch()
					continue
				}
				// With a release gate the batch answers out of band: the read
				// loop keeps consuming stdin so tests can wait for methods
				// (cancels, didClose) that arrive while the batch is held. The
				// queued requests are handed to the flusher so the loop stays
				// free to batch the next round.
				pendingIDs := defIDs
				pendingURIs := defURIs
				defIDs = nil
				defURIs = nil
				_ = os.WriteFile(readyPath, []byte("ready"), 0o600)
				go func() {
					waitForFile(readyPath + ".go")
					for i := range slices.Backward(pendingIDs) {
						write(map[string]any{
							"jsonrpc": "2.0",
							"id":      pendingIDs[i],
							"result":  defResult(pendingURIs[i]),
						})
					}
				}()
			}
		case "textDocument/references":
			write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": envPayload("LSP_TEST_REF_RESULT")})
		case "textDocument/implementation":
			write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": envPayload("LSP_TEST_IMPL_RESULT")})
		case "textDocument/typeDefinition":
			write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": envPayload("LSP_TEST_TYPEDEF_RESULT")})
		case "textDocument/hover":
			write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": envPayload("LSP_TEST_HOVER_RESULT")})
		case "textDocument/documentSymbol":
			write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": envPayload("LSP_TEST_DOC_SYM_RESULT")})
		case "workspace/symbol":
			write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": envPayload("LSP_TEST_WS_SYM_RESULT")})
		case "textDocument/prepareCallHierarchy":
			write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": envPayload("LSP_TEST_CALL_PREPARE_RESULT")})
		case "callHierarchy/incomingCalls", "callHierarchy/outgoingCalls":
			write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": envPayload("LSP_TEST_CALL_RESULT")})
		case "textDocument/diagnostic":
			// DIAG_UNCHANGED answers with an unchanged report once a
			// previousResultId was sent; otherwise the fixture reply.
			var p struct {
				PreviousResultID any `json:"previousResultId"`
			}
			_ = json.Unmarshal(msg.Params, &p)
			if os.Getenv("LSP_TEST_DIAG_UNCHANGED") == "1" && p.PreviousResultID != nil {
				write(
					map[string]any{
						"jsonrpc": "2.0",
						"id":      *msg.ID,
						"result":  map[string]any{"kind": "unchanged", "resultId": "r2"},
					},
				)
				continue
			}
			write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": envPayload("LSP_TEST_DIAG_RESULT")})
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

// envPayload returns the recorded fixture for name, or JSON null when unset.
func envPayload(name string) json.RawMessage {
	if v := os.Getenv(name); v != "" {
		return json.RawMessage(v)
	}
	return json.RawMessage("null")
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
