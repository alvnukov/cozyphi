package proc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultOutputLimit bounds Run's combined output when Limit.Bytes <= 0.
	DefaultOutputLimit = 8 * 1024 * 1024
	// DefaultStderrLimit bounds Start's stderr tail when stderrLimit <= 0.
	DefaultStderrLimit = 64 * 1024
	// DefaultGrace is the shutdown grace used when Close is passed a
	// non-positive duration.
	DefaultGrace = 2 * time.Second
)

// Spec describes one process to start. Argv is the exact argv without a shell.
// Env is the exact environment; nil inherits os.Environ(). Stdin feeds Run's
// standard input; Stream, when set, receives each output chunk as it arrives.
type Spec struct {
	Argv   []string
	Dir    string   // optional; must be an absolute, existing directory
	Env    []string // exact environment; nil inherits os.Environ()
	Stdin  string   // optional stdin content (Run only)
	Stream func(string)
}

// Limit bounds Run's combined output. Bytes <= 0 selects DefaultOutputLimit.
type Limit struct {
	Bytes int
}

// Result is the outcome of Run. Error is non-nil only for validation, spawn,
// or non-exit transport failures; a non-zero exit and cancellation are reported
// in ExitCode and Canceled instead.
type Result struct {
	Output    string
	Truncated bool
	ExitCode  int
	Canceled  bool
}

// Run executes a finite command with bounded combined stdout+stderr.
func Run(ctx context.Context, spec Spec, limit Limit) (Result, error) {
	if err := validateSpec(spec); err != nil {
		return Result{}, err
	}
	capBytes := limit.Bytes
	if capBytes <= 0 {
		capBytes = DefaultOutputLimit
	}

	cmd := command(ctx, spec.Argv)
	cmd.Dir = spec.Dir
	cmd.Env = envOrInherit(spec.Env)
	cmd.SysProcAttr = processGroupAttr()
	cmd.WaitDelay = waitDelay
	cmd.Cancel = func() error { return killProcessTree(cmd.Process.Pid) }
	if spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}

	tail := newTailBuffer(capBytes)
	var sink io.Writer = tail
	if spec.Stream != nil {
		sink = &outputWriter{tail: tail, stream: spec.Stream}
	}
	cmd.Stdout = sink
	cmd.Stderr = sink

	err := cmd.Run()
	res := Result{Output: tail.String(), Truncated: tail.Truncated()}
	if err == nil {
		return res, nil
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		res.Canceled = true
		return res, nil
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return res, fmt.Errorf("proc: run: %w", err)
}

// command builds a child command from explicit argv. gosec flags the variable
// argv spread; the caller owns argv, and shell semantics never apply here.
func command(ctx context.Context, argv []string) *exec.Cmd {
	return exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // G204: argv is the caller's explicit command
}

// Process is a long-lived subprocess started by Start.
type Process struct {
	spec   Spec
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *tailBuffer

	done      chan struct{}
	waitMu    sync.Mutex
	waitErr   error
	killed    bool
	closeOnce sync.Once
	killOnce  sync.Once
}

// Start launches a long-lived process. stdout and stderr stay separate: the
// caller must read Stdout; StderrTail is bounded. lifetime owns the process:
// canceling it kills the whole tree promptly, independent of Close.
func Start(lifetime context.Context, spec Spec, stderrLimit int) (*Process, error) {
	if err := validateSpec(spec); err != nil {
		return nil, err
	}
	if stderrLimit <= 0 {
		stderrLimit = DefaultStderrLimit
	}

	cmd := command(lifetime, spec.Argv)
	cmd.Dir = spec.Dir
	cmd.Env = envOrInherit(spec.Env)
	cmd.SysProcAttr = processGroupAttr()
	cmd.WaitDelay = waitDelay
	cmd.Cancel = func() error { return killProcessTree(cmd.Process.Pid) }

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("proc: start: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("proc: start: stdout pipe: %w", err)
	}
	stderr := newTailBuffer(stderrLimit)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("proc: start: %w", err)
	}

	p := &Process{
		spec:   spec,
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		done:   make(chan struct{}),
	}
	go p.reap()
	return p, nil
}

// Stdin returns the write side of the process standard input.
func (p *Process) Stdin() io.Writer { return p.stdin }

// Stdout returns the read side of the process standard output. Only one reader
// may consume it.
func (p *Process) Stdout() io.Reader { return p.stdout }

// StderrTail returns the bounded newest stderr output.
func (p *Process) StderrTail() string { return p.stderr.String() }

// StderrTruncated reports whether stderr exceeded the tail limit.
func (p *Process) StderrTruncated() bool { return p.stderr.Truncated() }

// Wait blocks until the process exits and is reaped, then returns its wait
// error (nil for a clean exit).
func (p *Process) Wait() error {
	<-p.done
	p.waitMu.Lock()
	defer p.waitMu.Unlock()
	return p.waitErr
}

// Close is idempotent. It closes stdin, waits grace for a natural exit, then
// kills the whole process tree and reaps. Close returns the wait error only
// when the process already exited on its own with a failure; an expected
// kill-on-close yields nil.
func (p *Process) Close(grace time.Duration) error {
	p.closeOnce.Do(func() {
		_ = p.stdin.Close()
		if grace <= 0 {
			grace = DefaultGrace
		}
		timer := time.NewTimer(grace)
		select {
		case <-p.done:
		case <-timer.C:
			p.killed = true
			p.killTree()
			<-p.done
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	})
	if p.killed {
		return nil
	}
	return p.Wait()
}

// reap joins the process and its stderr copy, then releases Wait.
func (p *Process) reap() {
	err := p.cmd.Wait()
	p.waitMu.Lock()
	p.waitErr = err
	p.waitMu.Unlock()
	close(p.done)
	_ = p.stdin.Close()
}

// killTree kills the process group once, unless the process is already reaped.
func (p *Process) killTree() {
	p.killOnce.Do(func() {
		select {
		case <-p.done:
			return // already reaped; never signal a possibly reused pid
		default:
		}
		if p.cmd != nil && p.cmd.Process != nil {
			_ = killProcessTree(p.cmd.Process.Pid)
		}
	})
}

// outputWriter tees each chunk to the bounded collector and the observer.
type outputWriter struct {
	tail   *tailBuffer
	stream func(string)
}

func (w *outputWriter) Write(p []byte) (int, error) {
	if _, err := w.tail.Write(p); err != nil {
		return 0, err
	}
	if w.stream != nil && len(p) > 0 {
		w.stream(string(p))
	}
	return len(p), nil
}

// tailBuffer keeps the newest limit bytes and reports whether older bytes were
// dropped.
type tailBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf.Write(p)
	if t.buf.Len() > t.limit {
		t.buf.Next(t.buf.Len() - t.limit)
		t.truncated = true
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buf.String()
}

func (t *tailBuffer) Truncated() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.truncated
}

func validateSpec(spec Spec) error {
	if len(spec.Argv) == 0 || strings.TrimSpace(spec.Argv[0]) == "" {
		return errors.New("proc: empty argv")
	}
	if spec.Dir == "" {
		return nil
	}
	if !filepath.IsAbs(spec.Dir) {
		return fmt.Errorf("proc: working directory must be absolute: %s", spec.Dir)
	}
	st, err := os.Stat(spec.Dir)
	if err != nil {
		return fmt.Errorf("proc: working directory: %w", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("proc: working directory is not a directory: %s", spec.Dir)
	}
	return nil
}

func envOrInherit(env []string) []string {
	if env == nil {
		return os.Environ()
	}
	return env
}
