package mcp

import (
	"bufio"
	"context"
	"encoding/json"
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
		if _, werr := t.proc.Stdin().Write(payload); werr != nil {
			ch <- outcome{err: fmt.Errorf("write %s: %w", method, werr)}
			return
		}
		rpc, rerr := t.readResponse()
		if rerr != nil {
			ch <- outcome{err: fmt.Errorf("read %s: %w", method, rerr)}
			return
		}
		raw, err := resultOrError(method, rpc)
		ch <- outcome{raw: raw, err: err}
	}()

	timer := time.NewTimer(deadline)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("mcp %s: timeout after %s", method, deadline)
	case out := <-ch:
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

func (t *stdioTransport) readResponse() (jsonRPCResponse, error) {
	for {
		line, err := t.stdout.ReadBytes('\n')
		if err != nil {
			return jsonRPCResponse{}, err
		}
		var rpc jsonRPCResponse
		if err := json.Unmarshal(line, &rpc); err != nil {
			return jsonRPCResponse{}, fmt.Errorf("parse response: %w; raw=%q", err, truncate(string(line), 200))
		}
		// Skip server notifications (method set, no id).
		if rpc.Method != "" && rpc.ID == nil {
			continue
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
