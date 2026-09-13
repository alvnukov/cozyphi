package shelltask

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/alvnukov/cozyphi/internal/proc"
)

// Manager is the single owner of its processes and output files. Close joins
// processes; stopping a turn only cancels tasks still in the foreground.
type Manager struct {
	mu              sync.Mutex
	dir             string
	runner          Runner
	entries         map[string]*entry
	order           []string
	subscribers     map[chan struct{}]struct{}
	closed          bool
	wg              sync.WaitGroup
	cleanup         chan string
	cleanupWG       sync.WaitGroup
	cleanupOnce     sync.Once
	storeOnce       sync.Once
	storeErr        error
	cleanupErr      error
	cleanupFailures int
}

type entry struct {
	receiptMu     sync.Mutex
	snapshot      Snapshot
	cancel        context.CancelFunc
	turn          context.Context
	done          chan struct{}
	promoted      chan struct{}
	result        proc.Result
	runErr        error
	captureErr    error
	persistErr    error
	stopRequested bool
	acknowledged  bool
	dir           string
	file          *os.File
	written       int
}

// New creates a private process-owned store below dir. Close removes only this
// owner's artifacts; no command or process is resumed from an old receipt.
func New(dir string, runner Runner) (*Manager, error) {
	if runner == nil {
		runner = proc.Run
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create shell task store: %w", err)
	}
	owned, err := os.MkdirTemp(dir, "shells-")
	if err != nil {
		return nil, fmt.Errorf("create private shell task store: %w", err)
	}
	m := &Manager{
		dir:         owned,
		runner:      runner,
		entries:     make(map[string]*entry),
		subscribers: make(map[chan struct{}]struct{}),
		cleanup:     make(chan string, MaxTasks),
	}
	m.cleanupWG.Add(1)
	go m.cleanArtifacts()
	return m, nil
}

// Run starts exactly one process, then waits for completion or promotion. The
// process context belongs to the manager from birth, so promotion never restarts it.
func (m *Manager) Run(ctx context.Context, req Request) (Result, error) {
	if req.ParentSessionID == "" || req.ToolUseID == "" || strings.TrimSpace(req.Command) == "" {
		return Result{}, errors.New("shell task requires a session, tool call and command")
	}
	if req.Timeout < 0 {
		return Result{}, errors.New("shell task timeout cannot be negative")
	}
	m.mu.Lock()
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		return Result{}, err
	}
	if m.closed {
		m.mu.Unlock()
		return Result{}, ErrClosed
	}
	live := 0
	for _, e := range m.entries {
		if e.snapshot.State == Running {
			live++
		}
		if e.snapshot.ParentSessionID == req.ParentSessionID && e.snapshot.ToolUseID == req.ToolUseID {
			m.mu.Unlock()
			return Result{}, errors.New("shell task already exists for this tool call")
		}
	}
	if live >= MaxLive {
		m.mu.Unlock()
		return Result{}, fmt.Errorf(
			"shell task limit reached: %d live processes; wait for completion or stop a task",
			MaxLive,
		)
	}
	if req.Background {
		if err := m.reserveBackgroundLocked(); err != nil {
			m.mu.Unlock()
			return Result{}, err
		}
	}
	id := "sh-" + rand.Text()
	dir := filepath.Join(m.dir, id)
	outputPath := filepath.Join(dir, "output.log")
	started := time.Now()
	var lifetime context.Context
	var cancel context.CancelFunc
	var deadline *time.Time
	if req.Timeout > 0 {
		at := started.Add(req.Timeout)
		deadline = &at
		lifetime, cancel = context.WithDeadline(context.Background(), at)
	} else {
		lifetime, cancel = context.WithCancel(context.Background())
	}
	e := &entry{
		snapshot: Snapshot{
			ID: id, ParentSessionID: req.ParentSessionID, ToolUseID: req.ToolUseID,
			Command: req.Command, OutputFile: outputPath, State: Running, Background: req.Background,
			Started: started, Deadline: deadline,
		},
		cancel: cancel, turn: ctx, done: make(chan struct{}), promoted: make(chan struct{}), dir: dir,
	}
	m.entries[id] = e
	m.order = append(m.order, id)
	if req.Background {
		close(e.promoted)
	}
	m.wg.Add(1)
	m.mu.Unlock()
	// Admission is reserved before touching disk, but filesystem work never holds
	// the snapshot lock used by stop/promotion and the UI.
	file, err := createOutput(dir, outputPath)
	if err != nil {
		m.mu.Lock()
		m.removeLocked(e)
		e.cancel()
		m.signalLocked()
		m.mu.Unlock()
		m.wg.Done()
		return Result{}, errors.Join(err, os.RemoveAll(dir))
	}
	e.file = file
	m.mu.Lock()
	m.signalLocked()
	m.mu.Unlock()
	req.Spec.Stream = func(chunk string) { m.capture(e, chunk) }
	req.Spec.CleanupGroup = true
	go m.execute(lifetime, e, req.Spec)
	for {
		select {
		case <-e.done:
			return m.takeResult(e)
		case <-e.promoted:
			m.mu.Lock()
			result := Result{Snapshot: e.snapshot}
			m.mu.Unlock()
			return result, nil
		case <-ctx.Done():
			m.mu.Lock()
			// Promotion and cancellation linearize under the same lock. A user
			// promotion that won owns the process independently of this turn.
			if !e.snapshot.Background && e.snapshot.State == Running {
				e.stopRequested = true
				e.cancel()
			}
			background := e.snapshot.Background
			snapshot := e.snapshot
			m.mu.Unlock()
			if background {
				return Result{Snapshot: snapshot}, nil
			}
			<-e.done
			result, err := m.takeResult(e)
			return result, errors.Join(err, ctx.Err())
		}
	}
}

func (m *Manager) capture(e *entry, chunk string) {
	m.mu.Lock()
	// Copy only the retained suffix. A substring must not keep a large stream
	// chunk alive, even when an injected runner writes megabytes in one call.
	if len(chunk) >= TailBytes {
		e.snapshot.Output = strings.Clone(chunk[len(chunk)-TailBytes:])
	} else {
		prefix := e.snapshot.Output
		prefix = prefix[max(len(prefix)-(TailBytes-len(chunk)), 0):]
		e.snapshot.Output = strings.Clone(prefix + chunk)
	}
	available := max(OutputFileBytes-e.written, 0)
	retained := chunk[:min(len(chunk), available)]
	e.written += len(retained)
	if len(retained) < len(chunk) {
		e.snapshot.Truncated = true
	}
	m.signalLocked()
	m.mu.Unlock()
	// Only the runner's stream goroutine writes the file. Disk I/O never holds
	// the snapshot lock or sends to a UI/model consumer.
	if retained != "" {
		if _, err := e.file.WriteString(retained); err != nil {
			m.mu.Lock()
			e.captureErr = fmt.Errorf("write shell output: %w", err)
			e.cancel()
			m.mu.Unlock()
		}
	}
}

func (m *Manager) execute(ctx context.Context, e *entry, spec proc.Spec) {
	defer m.wg.Done()
	res, err := m.runner(ctx, spec, proc.Limit{Bytes: OutputFileBytes})
	if flushErr := e.file.Sync(); flushErr != nil {
		err = errors.Join(err, fmt.Errorf("flush shell output: %w", flushErr))
	}
	if closeErr := e.file.Close(); closeErr != nil {
		err = errors.Join(err, fmt.Errorf("close shell output: %w", closeErr))
	}
	m.mu.Lock()
	err = errors.Join(err, e.captureErr)
	if !e.snapshot.Background {
		e.result = res
	}
	e.runErr = err
	state, stateErr := terminalState(e.stopRequested, ctx, err, res)
	finished := time.Now()
	e.snapshot.State, e.snapshot.Error = state, stateErr
	e.snapshot.Finished = &finished
	// A stopped process never reached an exit of its own; reporting any exit
	// code — let alone the zero a kill leaves behind — would read as an outcome.
	if state != Stopped {
		code := res.ExitCode
		e.snapshot.ExitCode = &code
	}
	e.snapshot.Truncated = e.snapshot.Truncated || res.Truncated
	outcome := Outcome{Snapshot: e.snapshot, EventID: "shell:" + e.snapshot.ID + ":terminal"}
	m.mu.Unlock()
	persistErr := persist(filepath.Join(e.dir, "result.json"), outcome)
	m.mu.Lock()
	e.persistErr = persistErr
	e.cancel()
	close(e.done)
	m.signalLocked()
	m.mu.Unlock()
}

// terminalState names how a process ended, with the failure detail when there
// is one. The order matters: a stop request outranks a non-zero exit because
// the kill, not the command, ended the process.
func terminalState(stopRequested bool, ctx context.Context, err error, res proc.Result) (State, string) {
	switch {
	case err != nil:
		return Failed, err.Error()
	case stopRequested || errors.Is(ctx.Err(), context.Canceled):
		return Stopped, ""
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return Failed, "command timed out"
	case res.ExitCode != 0:
		return Failed, ""
	}
	return Completed, ""
}

// Background is a user-only ownership change; it never starts a process.
func (m *Manager) Background(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return ErrNotFound
	}
	if m.closed {
		return ErrClosed
	}
	if e.snapshot.State != Running {
		return ErrFinished
	}
	if e.snapshot.Background {
		return nil
	}
	if err := e.turn.Err(); err != nil {
		return err
	}
	if e.stopRequested {
		return errors.New("shell task is stopping")
	}
	if err := m.reserveBackgroundLocked(); err != nil {
		return err
	}
	e.snapshot.Background = true
	close(e.promoted)
	m.signalLocked()
	return nil
}

// Stop requests cancellation without blocking the UI. Terminal means reaped.
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return ErrNotFound
	}
	if e.snapshot.State == Running {
		e.stopRequested = true
		e.cancel()
	}
	return nil
}

// List returns immutable snapshots belonging only to the requested conversation.
func (m *Manager) List(parentID string) []Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Snapshot, 0, len(m.order))
	for _, id := range m.order {
		if e := m.entries[id]; parentID == "" || e.snapshot.ParentSessionID == parentID {
			out = append(out, e.snapshot)
		}
	}
	return out
}

// Output returns at most limit bytes of the live tail, without polling a process.
func (m *Manager) Output(id string, limit int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return "", ErrNotFound
	}
	if limit <= 0 || limit > TailBytes {
		limit = TailBytes
	}
	output := e.snapshot.Output
	return output[max(len(output)-limit, 0):], nil
}

// Pending returns background results only after their durable terminal record.
func (m *Manager) Pending(parentID string, limit int) ([]Outcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = MaxLive
	}
	out := make([]Outcome, 0, min(limit, len(m.order)))
	for _, id := range m.order {
		e := m.entries[id]
		if e.snapshot.ParentSessionID != parentID || !e.snapshot.Background || e.acknowledged {
			continue
		}
		select {
		case <-e.done:
		default:
			continue
		}
		if e.persistErr != nil {
			return nil, e.persistErr
		}
		out = append(out, Outcome{Snapshot: e.snapshot, EventID: "shell:" + id + ":terminal"})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

// Acknowledge is called only after the session persisted the matching receipt.
func (m *Manager) Acknowledge(parentID, eventID string) error {
	m.mu.Lock()
	id := strings.TrimSuffix(strings.TrimPrefix(eventID, "shell:"), ":terminal")
	e, ok := m.entries[id]
	if !ok || eventID != "shell:"+id+":terminal" || e.snapshot.ParentSessionID != parentID {
		m.mu.Unlock()
		return errors.New("shell receipt does not identify this conversation")
	}
	m.mu.Unlock()
	// Never take a receipt lock while holding mu. This serializes durable acks
	// without blocking process cancellation or UI snapshots on disk latency.
	e.receiptMu.Lock()
	defer e.receiptMu.Unlock()
	m.mu.Lock()
	select {
	case <-e.done:
	default:
		m.mu.Unlock()
		return errors.New("shell task has no terminal receipt")
	}
	if e.persistErr != nil {
		err := e.persistErr
		m.mu.Unlock()
		return err
	}
	if e.acknowledged {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	if err := persist(filepath.Join(e.dir, "ack.json"), eventID); err != nil {
		return err
	}
	m.mu.Lock()
	e.acknowledged = true
	m.mu.Unlock()
	return nil
}

// Subscribe returns bounded, coalesced invalidations. The payload stays in List/Pending.
func (m *Manager) Subscribe() (<-chan struct{}, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan struct{}, 1)
	if m.closed {
		close(ch)
		return ch, func() {}
	}
	m.subscribers[ch] = struct{}{}
	ch <- struct{}{}
	return ch, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if _, ok := m.subscribers[ch]; ok {
			delete(m.subscribers, ch)
			close(ch)
		}
	}
}

func (m *Manager) removeLocked(e *entry) {
	delete(m.entries, e.snapshot.ID)
	m.order = slices.DeleteFunc(m.order, func(id string) bool { return id == e.snapshot.ID })
}

func (m *Manager) signalLocked() {
	for ch := range m.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Close cancels and joins every owned process and removes this owner's private
// artifacts. Delivered session receipts remain the transcript's responsibility.
func (m *Manager) Close() error {
	m.mu.Lock()
	m.closed = true
	for _, e := range m.entries {
		if e.snapshot.State == Running {
			e.stopRequested = true
			e.cancel()
		}
	}
	m.mu.Unlock()
	m.wg.Wait()
	m.cleanupOnce.Do(func() { close(m.cleanup) })
	m.cleanupWG.Wait()
	m.storeOnce.Do(func() {
		if err := os.RemoveAll(m.dir); err != nil {
			m.storeErr = fmt.Errorf("remove private shell task store: %w", err)
		}
	})
	m.mu.Lock()
	defer m.mu.Unlock()
	var errs []error
	if m.storeErr != nil {
		errs = append(errs, m.storeErr)
	}
	if m.cleanupErr != nil {
		errs = append(errs, fmt.Errorf("remove shell artifacts (%d failures): %w", m.cleanupFailures, m.cleanupErr))
	}
	for _, e := range m.entries {
		if e.persistErr != nil {
			errs = append(errs, e.persistErr)
		}
	}
	for ch := range m.subscribers {
		delete(m.subscribers, ch)
		close(ch)
	}
	return errors.Join(errs...)
}
