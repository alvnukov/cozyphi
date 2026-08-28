package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/alvnukov/cozyphi/internal/proc"
)

const (
	defaultTimeout = 60 * time.Second
	mcpCloseGrace  = 2 * time.Second
)

// stdioTransport speaks newline-delimited JSON-RPC over a subprocess. Process
// ownership, tree termination, and bounded stderr live in the proc module.
type stdioTransport struct {
	name    string
	cfg     ServerConfig
	timeout time.Duration
	id      atomic.Int64

	proc   *proc.Process
	stdout *bufio.Reader
}

func newStdioTransport(name string, cfg ServerConfig) (*stdioTransport, error) {
	if _, err := cfg.CmdLine(); err != nil {
		return nil, fmt.Errorf("server %q: %w", name, err)
	}
	timeout, err := cfg.TimeoutDuration()
	if err != nil {
		return nil, fmt.Errorf("server %q: %w", name, err)
	}
	return &stdioTransport{
		name:    name,
		cfg:     cfg,
		timeout: timeout,
	}, nil
}

func (t *stdioTransport) call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	if err := t.ensureStarted(); err != nil {
		return nil, err
	}
	// Pin this process generation for the reader goroutine: after a timeout,
	// call closes the transport, and the abandoned goroutine must keep
	// touching only these locals — never t.proc/t.stdout, which the next
	// call replaces with a fresh pair.
	proc, stdout := t.proc, t.stdout
	id := nextID(&t.id)
	payload, err := marshalRequest(id, method, params)
	if err != nil {
		return nil, err
	}
	payload = append(payload, '\n')

	deadline := t.requestDeadline(ctx)
	type outcome struct {
		raw json.RawMessage
		err error
	}
	ch := make(chan outcome, 1)
	go func() {
		if _, werr := proc.Stdin().Write(payload); werr != nil {
			ch <- outcome{err: fmt.Errorf("write %s: %w (%w)", method, werr, errTransportDead)}
			return
		}
		rpc, rerr := readResponse(stdout, id)
		if rerr != nil {
			ch <- outcome{err: fmt.Errorf("read %s: %w (%w)", method, rerr, errTransportDead)}
			return
		}
		raw, err := resultOrError(method, rpc)
		ch <- outcome{raw: raw, err: err}
	}()

	timer := time.NewTimer(deadline)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		// Abandoning the request leaves the wire unsynchronized: the late
		// answer would sit in the pipe. Close the generation so the parked
		// reader unwinds on pipe errors and the next call starts clean.
		_ = t.close()
		return nil, fmt.Errorf("mcp %s: %w (%w)", method, ctx.Err(), errTransportDead)
	case <-timer.C:
		_ = t.close()
		return nil, fmt.Errorf("mcp %s: timeout after %s (%w)", method, deadline, errTransportDead)
	case out := <-ch:
		if errors.Is(out.err, errTransportDead) {
			_ = t.close()
		}
		return out.raw, out.err
	}
}

func (t *stdioTransport) notify(_ context.Context, method string, params map[string]any) error {
	if err := t.ensureStarted(); err != nil {
		return err
	}
	payload, err := marshalNotification(method, params)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	_, err = t.proc.Stdin().Write(payload)
	return err
}

func (t *stdioTransport) close() error {
	if t.proc == nil {
		return nil
	}
	err := t.proc.Close(mcpCloseGrace)
	t.appendStderrLog()
	t.proc = nil
	t.stdout = nil
	return err
}

func (t *stdioTransport) ensureStarted() error {
	if t.proc != nil {
		return nil
	}
	argv, err := t.cfg.CmdLine()
	if err != nil {
		return fmt.Errorf("server %q: %w", t.name, err)
	}
	env := os.Environ()
	for k, v := range t.cfg.Env {
		env = append(env, k+"="+v)
	}
	p, err := proc.Start(context.Background(), proc.Spec{Argv: argv, Env: env}, proc.DefaultStderrLimit)
	if err != nil {
		return fmt.Errorf("spawn %q: %w", t.name, err)
	}
	t.proc = p
	t.stdout = bufio.NewReader(p.Stdout())
	return nil
}

// appendStderrLog persists the bounded stderr tail for post-mortem debugging.
// The live stream is bounded in memory by the proc module instead of an
// unbounded append-only log.
func (t *stdioTransport) appendStderrLog() {
	tail := t.proc.StderrTail()
	if tail == "" {
		return
	}
	logDir, err := LogDir()
	if err != nil {
		return
	}
	f, err := os.OpenFile(
		filepath.Join(logDir, sanitizeName(t.name)+".log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0o644,
	)
	if err != nil {
		return
	}
	_, _ = f.WriteString(tail)
	_ = f.Close()
}

// readResponse scans stdout lines until the response for id arrives.
// Messages carrying a method (notifications and server-to-client requests,
// whose id counter is independent of ours) and responses carrying some other
// id are skipped: only the id pair on a method-less message pins the answer
// to this call. An unparseable line fails closed as a dead transport: framing
// can no longer be trusted.
func readResponse(r *bufio.Reader, id int64) (jsonRPCResponse, error) {
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return jsonRPCResponse{}, err
		}
		var rpc jsonRPCResponse
		if err := json.Unmarshal(line, &rpc); err != nil {
			return jsonRPCResponse{}, fmt.Errorf(
				"parse response: %w (%w); raw=%q", err, errTransportDead, truncate(string(line), 200),
			)
		}
		if rpc.Method != "" {
			continue // notification or server-to-client request, never our response
		}
		if !responseIDMatches(rpc.ID, id) {
			continue // late answer to an abandoned call or another client's id
		}
		return rpc, nil
	}
}

func (t *stdioTransport) requestDeadline(ctx context.Context) time.Duration {
	deadline := t.timeout
	if deadline <= 0 {
		deadline = defaultTimeout
	}
	if d, ok := ctx.Deadline(); ok {
		if left := time.Until(d); left > 0 && left < deadline {
			return left
		}
	}
	return deadline
}
