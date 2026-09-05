package submit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
	"github.com/alvnukov/cozyphi/internal/tui/composer"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

// BashRunner runs user "!cmd" shells locally (not via the agent).
type BashRunner struct {
	transcript *transcript.TranscriptPane
	composer   composer.Input
	toast      func(msg string, kind toast.ToastKind, d time.Duration)
	publish    func(controller.Msg)

	cwd     string
	initErr error
	running atomic.Bool
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	closed  bool
}

// NewBashRunner builds a runner that reports initialization errors on submission.
// A supplied cwd is explicit and fails closed; legacy callers capture process cwd.
func NewBashRunner(
	transcript *transcript.TranscriptPane,
	composer composer.Input,
	toast func(msg string, kind toast.ToastKind, d time.Duration),
	publish func(controller.Msg),
	dirs ...string,
) *BashRunner {
	var cwd string
	var err error
	switch len(dirs) {
	case 0:
		cwd, err = os.Getwd()
	case 1:
		cwd = dirs[0]
	default:
		err = errors.New("provide exactly one local shell working directory")
	}
	if err == nil {
		var runner *BashRunner
		runner, err = NewBashRunnerInDir(transcript, composer, toast, publish, cwd)
		if err == nil {
			return runner
		}
	}
	return &BashRunner{
		transcript: transcript,
		composer:   composer,
		toast:      toast,
		publish:    publish,
		initErr:    fmt.Errorf("initialize local shell working directory: %w", err),
	}
}

// NewBashRunnerInDir binds local shells to a canonical, existing directory.
// The directory is captured once, never inferred from process cwd at submission.
func NewBashRunnerInDir(
	transcript *transcript.TranscriptPane,
	composer composer.Input,
	toast func(msg string, kind toast.ToastKind, d time.Duration),
	publish func(controller.Msg),
	cwd string,
) (*BashRunner, error) {
	if cwd == "" {
		return nil, errors.New("local shell working directory must not be empty")
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve local shell working directory: %w", err)
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve local shell working directory: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat local shell working directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("local shell working directory %q is not a directory", abs)
	}
	return &BashRunner{
		transcript: transcript,
		composer:   composer,
		toast:      toast,
		publish:    publish,
		cwd:        abs,
	}, nil
}

// Running reports whether a local bash command is in flight.
func (b *BashRunner) Running() bool {
	return b != nil && b.running.Load()
}

// HandleSubmit runs a user "!cmd". Returns true when the input was consumed.
func (b *BashRunner) HandleSubmit(text string) bool {
	if b == nil || !strings.HasPrefix(text, "!") {
		return false
	}
	command := strings.TrimSpace(text[1:])
	if command == "" {
		return false
	}
	if b.transcript != nil && b.transcript.IsStreaming() {
		b.showToast("Unable to use shell mode while agent is active", 3*time.Second)
		return true
	}
	b.mu.Lock()
	if b.closed || b.initErr != nil {
		err := b.initErr
		b.mu.Unlock()
		msg := "Local shell runner is closed"
		if err != nil {
			msg = err.Error()
		}
		b.showToast(msg, 3*time.Second)
		return true
	}
	if b.running.Load() {
		b.mu.Unlock()
		b.showToast(
			"A bash command is already running. Press Esc to cancel it first.",
			3*time.Second,
		)
		return true
	}
	// ExecShell's shell spec reads cwd from context and sets the process Dir.
	ctx, cancel := context.WithCancel(tooldef.WithCwd(context.Background(), b.cwd))
	b.cancel = cancel
	b.done = make(chan struct{})
	b.running.Store(true)
	b.mu.Unlock()

	if b.composer != nil {
		b.composer.HideCompleters()
		b.composer.ClearInput()
		b.SyncBorder("")
	}

	id := fmt.Sprintf("bash-%d", time.Now().UnixNano())
	if b.transcript != nil {
		b.transcript.ApplySession(session.LocalBashStart{ID: id, Command: command})
		b.transcript.Sync()
		b.transcript.StickToBottom()
	}

	go b.run(ctx, id, command)
	return true
}

func (b *BashRunner) run(ctx context.Context, id, command string) {
	defer func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.cancel()
		b.cancel = nil
		b.running.Store(false)
		close(b.done)
		b.done = nil
	}()

	const bashPublishInterval = 100 * time.Millisecond

	liveOutput := newBashLiveOutput(bashPublishInterval, func(cur string) {
		b.publishSession(session.ToolData{Run: session.ToolRun{
			ToolUseID: id,
			Name:      "bash",
			Status:    session.ToolInProgress,
			Detail:    command,
			Output:    cur,
			Local:     true,
		}})
	})

	result, err := tools.ExecShell(ctx, command, tools.ShellExecOptions{
		OnChunk: liveOutput.Append,
	})
	liveOutput.Close()
	if err != nil {
		b.publishSession(session.ToolData{Run: session.ToolRun{
			ToolUseID: id,
			Name:      "bash",
			Status:    session.ToolError,
			Detail:    command,
			Output:    result.Output,
			Error:     err.Error(),
			Local:     true,
		}})
		return
	}
	status := session.ToolDone
	if result.Canceled {
		status = session.ToolCancelled
	} else if result.ExitCode != 0 {
		status = session.ToolError
	}
	outText := result.Output
	if strings.TrimSpace(outText) == "" && !result.Canceled {
		outText = "(no output)"
	}
	b.publishSession(session.ToolData{Run: session.ToolRun{
		ToolUseID: id,
		Name:      "bash",
		Status:    status,
		Detail:    command,
		Output:    outText,
		ExitCode:  result.ExitCode,
		Local:     true,
	}})
}

func (b *BashRunner) publishSession(ev session.Event) {
	if b == nil || b.publish == nil {
		return
	}
	b.publish(controller.SessionEventMsg{Event: ev})
}

func (b *BashRunner) showToast(msg string, d time.Duration) {
	if b != nil && b.toast != nil {
		b.toast(msg, toast.ToastWarning, d)
	}
}

// Cancel aborts a running user "!cmd". Returns true if one was cancelled.
func (b *BashRunner) Cancel() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancel == nil {
		return false
	}
	b.cancel()
	return true
}

// Close permanently denies admission and cancels accepted work. A deadline
// does not relinquish ownership: callers may Close again to await completion.
// Completion includes shell exit and the final session publication.
func (b *BashRunner) Close(ctx context.Context) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	b.closed = true
	if b.cancel != nil {
		b.cancel()
	}
	done := b.done
	b.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for local shell exit: %w", ctx.Err())
	}
}

// SyncBorder paints the composer border for bash mode when text starts with "!".
func (b *BashRunner) SyncBorder(text string) {
	if b == nil || b.composer == nil {
		return
	}
	bash := strings.HasPrefix(strings.TrimLeft(text, " \t"), "!")
	b.composer.SetBashBorderActive(bash)
}

// bashLiveOutput publishes a bounded live tail immediately, then at most once
// per interval. A skipped update always schedules one trailing publication.
type bashLiveOutput struct {
	mu          sync.Mutex
	tail        *tools.BashOutputTail
	interval    time.Duration
	lastPublish time.Time
	timer       *time.Timer
	stopped     bool
	publish     func(output string)
}

func newBashLiveOutput(interval time.Duration, publish func(output string)) *bashLiveOutput {
	return &bashLiveOutput{
		tail:     tools.NewBashOutputTail(tools.BashMaxOutputLines, tools.BashMaxOutputBytes),
		interval: interval,
		publish:  publish,
	}
}

func (o *bashLiveOutput) Append(chunk string) {
	_, _ = o.tail.WriteString(chunk)

	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stopped || o.timer != nil {
		return
	}
	now := time.Now()
	if o.lastPublish.IsZero() || now.Sub(o.lastPublish) >= o.interval {
		o.publishLocked(now)
		return
	}
	delay := o.interval - now.Sub(o.lastPublish)
	o.timer = time.AfterFunc(delay, o.publishTrailing)
}

func (o *bashLiveOutput) publishTrailing() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.timer = nil
	if o.stopped {
		return
	}
	o.publishLocked(time.Now())
}

func (o *bashLiveOutput) publishLocked(now time.Time) {
	cur, truncated := o.tail.Snapshot()
	if truncated {
		cur = "[live output truncated; showing latest output]\n" + cur
	}
	o.lastPublish = now
	if o.publish != nil {
		o.publish(cur)
	}
}

// Close synchronizes with any active timer callback and prevents future
// in-progress publications.
func (o *bashLiveOutput) Close() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stopped = true
	if o.timer != nil {
		o.timer.Stop()
		o.timer = nil
	}
}
