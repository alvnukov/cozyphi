package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alvnukov/cozyphi/internal/proc"
)

// rpcError is the JSON-RPC error object.
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	if e == nil {
		return "rpc: <nil>"
	}
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// message is the decoded common envelope for requests, responses, and
// notifications. A non-nil ID plus a method is a server->client request; a
// non-nil ID without a method is a response; a nil ID is a notification.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// client is one live JSON-RPC connection keyed by canonical Go root.
type client struct {
	proc *proc.Process

	writeMu sync.Mutex
	nextID  atomic.Int64

	pendingMu sync.Mutex
	pending   map[int64]*pendingCall

	failOnce sync.Once
	failErr  error
	done     chan struct{}

	// capabilities stores the server's initialize result for later tickets.
	capsMu sync.Mutex
	caps   map[string]any

	// settings is the frozen config.Gopls.Settings used for configuration replies.
	settings map[string]any

	// opened tracks documents already sent via didOpen in this generation.
	openedMu sync.Mutex
	opened   map[string]bool
}

type pendingCall struct {
	ch chan message
}

// startClient spawns gopls and performs the handshake. ctx is the Manager
// lifetime: canceling it kills the process tree promptly.
func startClient(ctx context.Context, root string, config Config) (*client, error) {
	argv := append([]string(nil), config.Gopls.Command...)
	if len(argv) == 0 {
		argv = []string{"gopls"}
	}
	p, err := proc.Start(ctx, proc.Spec{
		Argv: argv,
		Dir:  root,
		Env:  config.Gopls.Env,
	}, MaxStderrTailBytes)
	if err != nil {
		return nil, fmt.Errorf("lsp: start gopls: %w", err)
	}

	c := &client{
		proc:     p,
		pending:  make(map[int64]*pendingCall),
		done:     make(chan struct{}),
		opened:   make(map[string]bool),
		settings: config.Gopls.Settings,
	}
	go c.readLoop()
	return c, nil
}

// initialize runs the LSP handshake and returns the server capabilities.
func (c *client) initialize(ctx context.Context, root string, config Config) error {
	params := map[string]any{
		"processId":             int64(pid()),
		"rootUri":               uriFromPath(root),
		"capabilities":          clientCapabilities(),
		"initializationOptions": config.Gopls.InitializationOptions,
	}
	raw, err := c.request(ctx, "initialize", params)
	if err != nil {
		return err
	}
	var result struct {
		Capabilities map[string]any `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("lsp: initialize: %w", err)
	}
	c.capsMu.Lock()
	c.caps = result.Capabilities
	c.capsMu.Unlock()
	// The initialized notification follows a successful initialize response.
	return c.notify(ctx, "initialized", map[string]any{})
}

func clientCapabilities() map[string]any {
	return map[string]any{
		"workspace": map[string]any{
			// We answer configuration and workspace-folder server requests.
			"configuration":    true,
			"workspaceFolders": true,
			// Never invite edits: the LSP tool is read-only.
			"applyEdit": false,
			// The workspace-wide symbol search this harness consumes.
			"symbol": true,
		},
		"textDocument": map[string]any{
			"synchronization": map[string]any{"didSave": true},
			"definition":      true,
			"references":      true,
			"hover":           true,
			"documentSymbol":  true,
			"callHierarchy":   true,
		},
	}
}

// request sends one request and awaits its response or ctx cancellation.
// A canceled ctx sends $/cancelRequest and never touches the shared process.
func (c *client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	call, id, err := c.beginCall()
	if err != nil {
		return nil, err
	}
	if err := c.write(id, method, params); err != nil {
		c.removeCall(id)
		return nil, err
	}
	select {
	case msg := <-call.ch:
		if msg.Error != nil {
			return nil, msg.Error
		}
		return msg.Result, nil
	case <-ctx.Done():
		c.removeCall(id)
		// Best effort: never block or kill the shared process on query cancel.
		_ = c.writeCancel(id)
		return nil, ctx.Err()
	case <-c.done:
		c.removeCall(id)
		return nil, c.failure()
	}
}

// notify sends a notification without awaiting a response.
func (c *client) notify(ctx context.Context, method string, params any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.write(0, method, params)
}

// beginCall reserves an ID and registers the pending slot before any write.
func (c *client) beginCall() (*pendingCall, int64, error) {
	select {
	case <-c.done:
		return nil, 0, c.failure()
	default:
	}
	id := c.nextID.Add(1)
	call := &pendingCall{ch: make(chan message, 1)}
	c.pendingMu.Lock()
	c.pending[id] = call
	c.pendingMu.Unlock()
	return call, id, nil
}

func (c *client) removeCall(id int64) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

// write serializes one frame. id==0 writes a notification envelope.
func (c *client) write(id int64, method string, params any) error {
	body := map[string]any{"jsonrpc": "2.0", "method": method}
	if id != 0 {
		body["id"] = id
		body["params"] = params
	} else if params != nil {
		body["params"] = params
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("lsp: marshal %s: %w", method, err)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := c.proc.Stdin().Write(encodeFrame(raw)); err != nil {
		c.fail(fmt.Errorf("lsp: write %s: %w", method, err))
		return c.failure()
	}
	return nil
}

func (c *client) writeCancel(id int64) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "$/cancelRequest",
		"params":  map[string]any{"id": id},
	})
	if _, err := c.proc.Stdin().Write(encodeFrame(raw)); err != nil {
		c.fail(fmt.Errorf("lsp: cancel write: %w", err))
		return c.failure()
	}
	return nil
}

// failure returns the terminal transport error after the reader has failed.
func (c *client) failure() error {
	if c.failErr != nil {
		return c.failErr
	}
	return errors.New("lsp: connection closed")
}

// fail marks the client dead exactly once and unblocks every pending call.
func (c *client) fail(err error) {
	c.failOnce.Do(func() {
		c.failErr = err
		close(c.done)
		c.pendingMu.Lock()
		pending := c.pending
		c.pending = make(map[int64]*pendingCall)
		c.pendingMu.Unlock()
		for _, call := range pending {
			call.fail()
		}
	})
}

func (call *pendingCall) fail() {
	select {
	case call.ch <- message{}:
	default:
	}
}

// readLoop consumes stdout until EOF or a framing error, then fails the client.
func (c *client) readLoop() {
	reader := bufio.NewReader(c.proc.Stdout())
	for {
		raw, err := readFrame(reader)
		if err != nil {
			c.fail(err)
			return
		}
		var msg message
		if err := json.Unmarshal(raw, &msg); err != nil {
			c.fail(fmt.Errorf("lsp: bad frame: %w", err))
			return
		}
		c.handle(msg)
	}
}

func (c *client) handle(msg message) {
	switch {
	case msg.ID != nil && msg.Method != "":
		c.handleServerRequest(msg)
	case msg.ID != nil:
		c.handleResponse(msg)
	case msg.Method != "":
		// Notifications (diagnostics, logTrace, progress) are consumed and
		// discarded here; ticket 5 wires publishDiagnostics into its cache.
	default:
		c.fail(errors.New("lsp: frame has no id and no method"))
	}
}

func (c *client) handleResponse(msg message) {
	c.pendingMu.Lock()
	call := c.pending[*msg.ID]
	delete(c.pending, *msg.ID)
	c.pendingMu.Unlock()
	if call == nil {
		return // cancelled or already completed; drop the late response
	}
	select {
	case call.ch <- msg:
	default:
	}
}

// handleServerRequest answers the server without the model. Mutating requests
// are always declined; unsupported methods return a method-not-found error.
func (c *client) handleServerRequest(msg message) {
	switch msg.Method {
	case "workspace/workspaceFolders":
		c.reply(*msg.ID, nil, nil)
	case "workspace/configuration":
		c.replyConfiguration(msg)
	case "workspace/applyEdit":
		c.reply(*msg.ID, map[string]any{"applied": false}, nil)
	case "window/workDoneProgress/create", "client/registerCapability", "client/unregisterCapability":
		c.reply(*msg.ID, nil, nil)
	default:
		c.reply(*msg.ID, nil, &rpcError{Code: -32601, Message: "method not found"})
	}
}

// replyConfiguration answers workspace/configuration items from frozen settings.
func (c *client) replyConfiguration(msg message) {
	var params struct {
		Items []struct {
			Section string `json:"section"`
		} `json:"items"`
	}
	_ = json.Unmarshal(msg.Params, &params)
	items := make([]any, 0, len(params.Items))
	for _, item := range params.Items {
		items = append(items, c.configItem(item.Section))
	}
	c.reply(*msg.ID, items, nil)
}

func (c *client) configItem(section string) any {
	return dottedLookup(c.settings, section)
}

// dottedLookup resolves a "gopls.hints"-style section against settings.
func dottedLookup(settings map[string]any, section string) any {
	if section == "" {
		return nil
	}
	var cur any = settings
	for part := range strings.SplitSeq(section, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[part]
		if !ok {
			return nil
		}
	}
	return cur
}

func (c *client) reply(id int64, result any, rpcErr *rpcError) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	body := map[string]any{"jsonrpc": "2.0", "id": id}
	if rpcErr != nil {
		body["error"] = rpcErr
	} else {
		body["result"] = result
	}
	raw, _ := json.Marshal(body)
	if _, err := c.proc.Stdin().Write(encodeFrame(raw)); err != nil {
		c.fail(fmt.Errorf("lsp: reply write: %w", err))
	}
}

// shutdown sends the graceful LSP shutdown/exit sequence, then reaps the
// process. It is safe to call at most once per client.
func (c *client) shutdown(grace time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	// A nil response to shutdown is a valid acknowledgment.
	_, _ = c.request(ctx, "shutdown", nil)
	_ = c.notify(ctx, "exit", nil)
	_ = c.proc.Close(grace)
}

// encodeFrame wraps raw in a Content-Length header.
func encodeFrame(raw []byte) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Content-Length: %d\r\n\r\n", len(raw))
	buf.Write(raw)
	return buf.Bytes()
}

// readFrame reads one Content-Length framed body, enforcing header and body
// caps before allocating.
func readFrame(r *bufio.Reader) ([]byte, error) {
	header, err := readHeader(r)
	if err != nil {
		return nil, err
	}
	length, err := contentLength(header)
	if err != nil {
		return nil, err
	}
	if length < 0 || length > MaxFrameBytes {
		return nil, fmt.Errorf("lsp: frame length %d exceeds limit %d", length, MaxFrameBytes)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("lsp: read frame body: %w", err)
	}
	return body, nil
}

func readHeader(r *bufio.Reader) (string, error) {
	var header strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("lsp: read header: %w", err)
		}
		if header.Len()+len(line) > MaxHeaderBytes {
			return "", fmt.Errorf("lsp: header exceeds %d bytes", MaxHeaderBytes)
		}
		header.WriteString(line)
		if line == "\r\n" || line == "\n" {
			return header.String(), nil
		}
	}
}

func contentLength(header string) (int, error) {
	for line := range strings.SplitSeq(header, "\n") {
		line = strings.TrimRight(line, "\r")
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return 0, fmt.Errorf("lsp: bad Content-Length %q: %w", strings.TrimSpace(value), err)
			}
			return n, nil
		}
	}
	return 0, errors.New("lsp: missing Content-Length")
}
