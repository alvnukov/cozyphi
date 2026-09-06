package job

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Ceilings a Manager applies when Options names none. They are constants
// rather than literals inside New because the harness view reports them for
// a process that has not built a manager yet, and two spellings of the same
// default would drift.
const (
	// defaultMaxConcurrent is how many jobs run at once. There is no queue
	// behind it: past the ceiling Spawn returns [ErrBusy].
	defaultMaxConcurrent = 4
	// defaultMaxDepth is how deep jobs may nest. One means a child cannot
	// spawn a child of its own.
	defaultMaxDepth = 1
)

// Options configures a [Manager].
type Options struct {
	Root          string // required: jobs directory
	Runner        Runner // required
	MaxConcurrent int    // default 4; Spawn returns [ErrBusy] when full (no queue)
	MaxDepth      int    // default 1 (children cannot spawn further)
	Recovery      RecoveryMode
	// ModelNameForRole reports the display name pinned to a role under
	// agents.models; ok=false means the role inherits the session model.
	// A pure lookup, so the spawn result can name it without racing the
	// runner, which resolves the same pin when it builds the child.
	ModelNameForRole func(Role) (string, bool)
	// OnStoreError is called when a disk write fails after the job is live.
	// Create/Spawn still return the create error directly.
	OnStoreError func(op, jobID string, err error)
	// OnOutcome hints at a persisted terminal envelope or an explicit delivery error.
	// Consumers reconcile through PendingOutcomes; this callback is not an ack.
	OnOutcome func(ownerID, parentID string)
}

// Manager owns in-process job lifecycles and a disk store.
type Manager struct {
	store            *store
	runner           Runner
	maxConcurrent    int
	maxDepth         int
	onStoreError     func(op, jobID string, err error)
	onOutcome        func(ownerID, parentID string)
	modelNameForRole func(Role) (string, bool)

	mu            sync.Mutex
	closed        bool
	closedParents map[string]bool
	closedOwners  map[string]bool
	slots         chan struct{}
	jobs          map[string]*liveJob
	pending       map[chan struct{}]admission // guarded by mu
	subs          []*subscriber
}

type admission struct {
	parentID string
	ownerID  string
}

type liveJob struct {
	meta            Meta
	parentToolUseID string
	runner          Runner
	cancel          context.CancelFunc
	done            chan struct{}
	persistErr      error // guarded by mu; retains its slot until metadata reconciliation
	exited          bool  // guarded by mu
}

// subscriber is one progress channel. closed is guarded by m.mu: emitProgress
// sends while holding m.mu, and cancel/Close mark closed and close the channel
// under the same lock, so a send can never race a channel close.
type subscriber struct {
	ch     chan Progress
	closed bool
}

// New creates a Manager. Root and Runner are required.
//
// By default ([RecoverMarkFailed]), leftover starting/running jobs on disk are
// marked failed so a process restart does not leave Wait/Cancel zombies.
func New(opts Options) (*Manager, error) {
	if opts.Runner == nil {
		return nil, fmt.Errorf("%w: Runner is required", ErrInvalid)
	}
	st, err := newStore(opts.Root)
	if err != nil {
		return nil, err
	}
	maxC := defaultedConcurrency(opts.MaxConcurrent)
	maxD := defaultedDepth(opts.MaxDepth)
	m := &Manager{
		store:            st,
		runner:           opts.Runner,
		maxConcurrent:    maxC,
		maxDepth:         maxD,
		onStoreError:     opts.OnStoreError,
		onOutcome:        opts.OnOutcome,
		modelNameForRole: opts.ModelNameForRole,
		slots:            make(chan struct{}, maxC),
		jobs:             make(map[string]*liveJob),
		closedParents:    make(map[string]bool),
		closedOwners:     make(map[string]bool),
		pending:          make(map[chan struct{}]admission),
	}
	if opts.Recovery != RecoverIgnore {
		if err := m.recoverStale(); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *Manager) recoverStale() error {
	ids, err := m.store.listIDs()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, id := range ids {
		meta, err := m.store.readMeta(id)
		if err != nil {
			m.reportStore("readMeta", id, err)
			continue
		}
		if meta.Status.Terminal() {
			continue
		}
		meta.Status = StatusFailed
		meta.FinishedAt = now
		meta.Error = "interrupted: process restarted before job finished"
		if err := m.store.writeMeta(meta); err != nil {
			return fmt.Errorf("job: recover %s: %w", id, err)
		}
		if err := m.store.appendEvent(meta, "recovered: marked failed after restart"); err != nil {
			m.reportStore("appendEvent", id, err)
		}
	}
	return nil
}

func (m *Manager) reportStore(op, jobID string, err error) {
	if err == nil || m.onStoreError == nil {
		return
	}
	m.onStoreError(op, jobID, err)
}

func (m *Manager) persistMeta(meta Meta) {
	if err := m.store.writeMeta(meta); err != nil {
		m.reportStore("writeMeta", meta.ID, err)
	}
}

func (m *Manager) persistEvent(meta Meta, msg string) {
	if err := m.store.appendEvent(meta, msg); err != nil {
		m.reportStore("appendEvent", meta.ID, err)
	}
}

// ModelNameForRole reports the per-role model display name from the same
// agents.models pin the runner resolves. A manager built without the seam —
// or a role without a pin — reports inherit (ok=false).
func (m *Manager) ModelNameForRole(role Role) (string, bool) {
	if m == nil || m.modelNameForRole == nil {
		return "", false
	}
	return m.modelNameForRole(role)
}

// Spawn starts a job asynchronously. The returned Info reflects starting state.
// Concurrency: if MaxConcurrent slots are full, Spawn returns [ErrBusy]
// (jobs are not queued). Depth: if req.Depth >= MaxDepth, returns [ErrDepth].
func (m *Manager) Spawn(ctx context.Context, req SpawnRequest) (Info, error) {
	return m.SpawnWithRunner(ctx, req, m.runner)
}

// SpawnWithRunner starts a job with an execution adapter selected for this spawn.
// The runner is required, is never persisted, and shares Spawn's admission limits.
func (m *Manager) SpawnWithRunner(ctx context.Context, req SpawnRequest, runner Runner) (Info, error) {
	if runner == nil {
		return Info{}, fmt.Errorf("%w: Runner is required", ErrInvalid)
	}
	if err := req.validate(); err != nil {
		return Info{}, err
	}
	if req.Depth >= m.maxDepth {
		return Info{}, fmt.Errorf("%w: depth %d >= max %d", ErrDepth, req.Depth, m.maxDepth)
	}

	m.mu.Lock()
	if m.closed || m.closedParents[req.ParentID] || m.closedOwners[req.OwnerID] {
		m.mu.Unlock()
		return Info{}, ErrClosed
	}
	select {
	case m.slots <- struct{}{}:
	default:
		m.mu.Unlock()
		return Info{}, ErrBusy
	}
	// Store creation happens without the manager lock. Shutdown must still
	// reap this admission, including cancellation writes if registration loses.
	admitted := make(chan struct{})
	m.pending[admitted] = admission{parentID: req.ParentID, ownerID: req.OwnerID}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		if _, pending := m.pending[admitted]; pending {
			delete(m.pending, admitted)
			close(admitted)
		}
		m.mu.Unlock()
	}()

	id, err := newJobID()
	if err != nil {
		<-m.slots
		return Info{}, err
	}
	now := time.Now().UTC()
	meta := Meta{
		ID:              id,
		ParentID:        req.ParentID,
		ParentToolUseID: req.ParentToolUseID,
		PreviousJobID:   req.PreviousJobID,
		OwnerID:         req.OwnerID,
		ParentDepth:     req.Depth,
		Role:            NormalizeRole(string(req.Role)),
		Prompt:          req.Prompt,
		Description:     req.Description,
		WorkDir:         req.WorkDir,
		ParentWorkspace: req.ParentWorkspace,
		Skills:          req.Skills,
		Effort:          req.Effort,
		Status:          StatusStarting,
		CreatedAt:       now,
	}
	meta, err = m.store.create(meta)
	if err != nil {
		<-m.slots
		return Info{}, err
	}

	var runCtx context.Context
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(context.Background(), req.Timeout)
	} else {
		runCtx, cancel = context.WithCancel(context.Background())
	}
	// Parent ctx cancel does not kill the job (jobs outlive a single tool call);
	// callers use Cancel. Still respect if Spawn itself is aborted before start.
	if err := ctx.Err(); err != nil {
		cancel()
		meta.Status = StatusCancelled
		meta.FinishedAt = time.Now().UTC()
		meta.Error = err.Error()
		m.persistMeta(meta)
		<-m.slots
		return Info{}, err
	}

	lj := &liveJob{
		meta:            meta,
		parentToolUseID: req.ParentToolUseID,
		runner:          runner,
		cancel:          cancel,
		done:            make(chan struct{}),
	}
	m.mu.Lock()
	// Shutdown may have barred admission during store creation;
	// registering now would start a runner outside its reaping snapshot.
	if m.closed || m.closedParents[req.ParentID] || m.closedOwners[req.OwnerID] {
		m.mu.Unlock()
		cancel()
		meta.Status = StatusCancelled
		meta.FinishedAt = time.Now().UTC()
		meta.Error = ErrClosed.Error()
		m.persistMeta(meta)
		<-m.slots
		return Info{}, ErrClosed
	}
	m.jobs[id] = lj
	// Transfer ownership atomically so counts never include both admission and job.
	delete(m.pending, admitted)
	close(admitted)
	m.mu.Unlock()

	go m.run(runCtx, lj)
	return Info{Meta: meta}, nil
}

func (m *Manager) run(ctx context.Context, lj *liveJob) {
	defer func() {
		lj.cancel() // Release timeout resources even on normal completion.
		m.mu.Lock()
		// Shutdown must not miss a job between removal and resource release.
		lj.exited = true
		if lj.persistErr == nil {
			delete(m.jobs, lj.meta.ID)
			<-m.slots
		}
		close(lj.done)
		m.mu.Unlock()
	}()

	meta := lj.meta
	meta.Status = StatusRunning
	meta.StartedAt = time.Now().UTC()
	m.persistMeta(meta)
	m.setLiveMeta(meta)
	m.persistEvent(meta, "running")

	var intervened atomic.Bool
	env := RunEnv{
		Job:              meta,
		MarkIntervention: func() { intervened.Store(true) },
		BindSession: func(id string) error {
			if id == "" || (meta.ChildSessionID != "" && meta.ChildSessionID != id) {
				return errors.New("job: child session identity is empty or already bound")
			}
			next := meta
			next.ChildSessionID = id
			if err := m.store.writeMeta(next); err != nil {
				m.reportStore("bindSession", meta.ID, err)
				return fmt.Errorf("job: persist child session identity: %w", err)
			}
			meta = next
			m.setLiveMeta(meta)
			return nil
		},
		Log: func(message string) {
			m.persistEvent(meta, message)
		},
		WriteResult: func(summary string) error {
			err := m.store.writeResult(meta, summary)
			if err != nil {
				m.reportStore("writeResult", meta.ID, err)
			}
			return err
		},
		OnProgress: func(p Progress) {
			p.JobID = meta.ID
			p.ParentID = meta.ParentID
			p.OwnerID = meta.OwnerID
			if p.ParentToolUseID == "" {
				p.ParentToolUseID = lj.parentToolUseID
			}
			if p.Time.IsZero() {
				p.Time = time.Now().UTC()
			}
			m.emitProgress(p)
		},
	}

	summary, err := lj.runner.Run(ctx, env)
	meta.UserIntervened = intervened.Load()
	var stopped *StoppedError

	meta.FinishedAt = time.Now().UTC()

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		meta.Status = StatusTimedOut
		if err != nil {
			meta.Error = err.Error()
		} else {
			meta.Error = ctx.Err().Error()
		}
		m.persistEvent(meta, "timed_out: "+meta.Error)
	case errors.Is(ctx.Err(), context.Canceled):
		meta.Status = StatusCancelled
		if err != nil {
			meta.Error = err.Error()
		} else {
			meta.Error = ctx.Err().Error()
		}
		m.persistEvent(meta, "cancelled: "+meta.Error)
	case errors.As(err, &stopped):
		meta.Status = StatusCancelled
		meta.StopReason = stopped.Reason
		meta.Error = stopped.Error()
		if summary != "" {
			if writeErr := m.store.writeResult(meta, summary); writeErr != nil {
				m.reportStore("writeResult", meta.ID, writeErr)
				meta.Error += "; failed to persist partial result: " + writeErr.Error()
			}
		}
		m.persistEvent(meta, "cancelled: "+meta.Error)
	case err != nil:
		meta.Status = StatusFailed
		meta.Error = err.Error()
		m.persistEvent(meta, "failed: "+meta.Error)
	default:
		meta.Status = StatusCompleted
		if summary != "" {
			if werr := m.store.writeResult(meta, summary); werr != nil {
				m.reportStore("writeResult", meta.ID, werr)
				meta.Status = StatusFailed
				meta.Error = "failed to write result.md: " + werr.Error()
				m.persistEvent(meta, "failed: "+meta.Error)
			} else {
				m.persistEvent(meta, "completed")
			}
		} else {
			m.persistEvent(meta, "completed")
		}
	}

	// The terminal envelope and status commit together before Wait observes
	// completion. Progress subscribers are deliberately not a delivery channel.
	meta.OutcomeID = meta.ID + ":terminal"
	meta.OutcomeSummary = boundedOutcomeText(summary)
	if err := m.store.writeMeta(meta); err != nil {
		m.mu.Lock()
		lj.meta = meta
		lj.persistErr = err
		m.mu.Unlock()
		m.reportStore("writeOutcome", meta.ID, err)
	}
	m.setLiveMeta(meta)
	if m.onOutcome != nil {
		m.onOutcome(meta.OwnerID, meta.ParentID)
	}
}

func (m *Manager) setLiveMeta(meta Meta) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if lj, ok := m.jobs[meta.ID]; ok {
		lj.meta = meta
	}
}

// List returns all jobs known on disk, newest CreatedAt first.
func (m *Manager) List(ctx context.Context) ([]Info, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ids, err := m.store.listIDs()
	if err != nil {
		return nil, err
	}
	out := make([]Info, 0, len(ids))
	for _, id := range ids {
		info, err := m.loadInfo(id)
		if err != nil {
			continue
		}
		out = append(out, info)
	}
	slices.SortFunc(out, func(a, b Info) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	return out, nil
}

// Get returns one job by id.
func (m *Manager) Get(ctx context.Context, id string) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	return m.loadInfo(id)
}

func (m *Manager) loadInfo(id string) (Info, error) {
	m.mu.Lock()
	if lj, ok := m.jobs[id]; ok {
		err := m.reconcileOutcomeLocked(lj)
		meta := lj.meta
		m.mu.Unlock()
		return Info{Meta: meta}, err
	}
	m.mu.Unlock()
	meta, err := m.store.readMeta(id)
	if err != nil {
		return Info{}, err
	}
	return Info{Meta: meta}, nil
}

// Wait blocks until the job reaches a terminal status or ctx is done.
//
// A Wait timeout (ctx deadline) does not cancel the job; use [Manager.Cancel].
func (m *Manager) Wait(ctx context.Context, id string) (WaitResult, error) {
	for {
		info, err := m.loadInfo(id)
		if err != nil {
			return WaitResult{}, err
		}
		if info.Status.Terminal() {
			return m.waitResult(info), nil
		}

		m.mu.Lock()
		lj, ok := m.jobs[id]
		var done <-chan struct{}
		if ok {
			done = lj.done
		}
		m.mu.Unlock()

		if !ok {
			info, err = m.loadInfo(id)
			if err != nil {
				return WaitResult{}, err
			}
			if info.Status.Terminal() {
				return m.waitResult(info), nil
			}
			return WaitResult{}, fmt.Errorf("%w: %s disappeared while non-terminal", ErrNotFound, id)
		}

		select {
		case <-ctx.Done():
			return WaitResult{}, fmt.Errorf("%w: %w", ErrWaitTimeout, ctx.Err())
		case <-done:
		}
	}
}

// Log returns the last limit events (0 = all).
func (m *Manager) Log(ctx context.Context, id string, limit int) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := m.loadInfo(id)
	if err != nil {
		return nil, err
	}
	return m.store.readEvents(info.Meta, limit)
}

// Cancel requests cancellation of a running/starting job.
func (m *Manager) Cancel(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	lj, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		info, err := m.loadInfo(id)
		if err != nil {
			return err
		}
		if info.Status.Terminal() {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrNotRunning, id)
	}
	lj.cancel()
	select {
	case <-lj.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CloseParent permanently closes admission for parentID and cancels its live jobs.
// It waits for in-flight admissions, runners and final writes, or returns ctx.Err(). A timed-out
// wait does not release their slots; repeated calls can finish reaping them.
// Other parents and progress subscriptions remain usable.
func (m *Manager) CloseParent(ctx context.Context, parentID string) error {
	m.mu.Lock()
	m.closedParents[parentID] = true
	var done []<-chan struct{}
	for _, lj := range m.jobs {
		if lj.meta.ParentID == parentID {
			done = append(done, lj.done)
			lj.cancel()
		}
	}
	for admitted, pending := range m.pending {
		if pending.parentID == parentID {
			done = append(done, admitted)
		}
	}
	m.mu.Unlock()

	for _, ch := range done {
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return m.reconcileOutcomes(ctx, func(meta Meta) bool { return meta.ParentID == parentID })
}

// CloseOwner permanently closes admission for an assignment owner, independently
// of ParentID. It cancels and reaps all of that owner's jobs and pending admissions.
// A deadline only bounds the wait: slots remain held through exit and final writes.
// An empty owner identifies legacy unowned jobs; it is not a wildcard.
func (m *Manager) CloseOwner(ctx context.Context, ownerID string) error {
	m.mu.Lock()
	m.closedOwners[ownerID] = true
	var done []<-chan struct{}
	for _, lj := range m.jobs {
		if lj.meta.OwnerID == ownerID {
			done = append(done, lj.done)
			lj.cancel()
		}
	}
	for admitted, pending := range m.pending {
		if pending.ownerID == ownerID {
			done = append(done, admitted)
		}
	}
	m.mu.Unlock()
	for _, ch := range done {
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return m.reconcileOutcomes(ctx, func(meta Meta) bool { return meta.OwnerID == ownerID })
}

// Close cancels all live jobs and waits for them to exit. The wait is
// unconditional by contract: t.Context() is cancelled before test cleanups
// run, so a caller-side deadline here would abandon a runner mid-write into
// directories the caller is already removing. Callers with a shutdown budget
// bound their own wait instead (Controller.Close does) and abandon the
// reaping, not the runner's final writes.
func (m *Manager) Close() error {
	m.mu.Lock()
	m.closed = true
	done := make([]<-chan struct{}, 0, len(m.jobs)+len(m.pending))
	for _, lj := range m.jobs {
		done = append(done, lj.done)
		lj.cancel()
	}
	for admitted := range m.pending {
		done = append(done, admitted)
	}
	// Close subscriber channels under the lock so they cannot race a send in
	// emitProgress (which also holds m.mu). The closed flag makes Close safe
	// even if a subscriber's cancel func runs later.
	for _, s := range m.subs {
		if !s.closed {
			s.closed = true
			close(s.ch)
		}
	}
	m.subs = nil
	m.mu.Unlock()

	// Every job above is cancelled and runners must honor cancellation, so
	// wait unconditionally for the last store write instead of racing a
	// caller that is already removing the job directories.
	for _, ch := range done {
		<-ch
	}
	return m.reconcileOutcomes(context.Background(), func(Meta) bool { return true })
}

// Subscribe receives live [Progress] events. The channel is buffered; slow
// consumers may miss events. Cancel closes the channel and unregisters.
func (m *Manager) Subscribe() (<-chan Progress, func()) {
	ch := make(chan Progress, 64)
	sub := &subscriber{ch: ch}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	m.subs = append(m.subs, sub)
	m.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			for i, s := range m.subs {
				if s == sub {
					m.subs = slices.Delete(m.subs, i, i+1)
					break
				}
			}
			// Idempotent vs Manager.Close: never close an already closed channel.
			if !sub.closed {
				sub.closed = true
				close(ch)
			}
		})
	}
	return ch, cancel
}

func (m *Manager) emitProgress(p Progress) {
	// Sends are non-blocking, so holding the lock is safe; it makes send and
	// channel close mutually exclusive (cancel/Close close under m.mu).
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.subs {
		select {
		case s.ch <- p:
		default:
			// drop if subscriber is slow
		}
	}
}

// ParentToolUseID returns the UI correlation id for a live job, if any.
func (m *Manager) ParentToolUseID(jobID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if lj, ok := m.jobs[jobID]; ok {
		return lj.parentToolUseID
	}
	return ""
}

// MaxDepth exposes the configured nesting ceiling (for tool adapters).
func (m *Manager) MaxDepth() int { return m.maxDepth }

// MaxConcurrent exposes the concurrency ceiling.
func (m *Manager) MaxConcurrent() int { return m.maxConcurrent }

// LiveCount returns how many jobs are currently starting/running in-process.
func (m *Manager) LiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.jobs)
}

// LiveCountForOwner counts an owner's admissions and runners, including teardown.
// Empty selects legacy unowned jobs, not all owners.
func (m *Manager) LiveCountForOwner(ownerID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, lj := range m.jobs {
		if lj.meta.OwnerID == ownerID {
			n++
		}
	}
	for _, pending := range m.pending {
		if pending.ownerID == ownerID {
			n++
		}
	}
	return n
}
