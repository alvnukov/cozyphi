package controller

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// ChildSession is retained independently of its assignment's execution slot.
// UI assembly reads snapshots on its own goroutine and never activates on arrival.
type ChildSession struct {
	JobID string
	Name  string
	// Title names the child the way every surface does — role(description) —
	// so the agent panel's row and the parent's transcript row read alike.
	Title string
	// ParentSessionID is the conversation that spawned this child. The UI
	// hands the child to that session's family; children never open a tab of
	// their own, so this is the only link back.
	ParentSessionID string
	Controller      *Controller
	Bus             *Bus
	Workspace       *Workspace
	Project         *project.Project
	// Ready acknowledges complete UI assembly before the first turn can start.
	Ready func(error)
}

// EnableInteractiveChildren opts the terminal runtime into retained child turns.
// Headless and single-controller clients keep their existing one-shot runner.
func (r *Runtime) EnableInteractiveChildren() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.interactiveChildren = true
}

// Children returns a recoverable creation snapshot, including already-finished
// children. Fast text-only jobs cannot disappear before the UI observes them.
func (r *Runtime) Children() []ChildSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ChildSession(nil), r.children...)
}

type interactiveRunner struct {
	agent.EngineRunner
	parent *Controller
}

func (r interactiveRunner) Run(ctx context.Context, env job.RunEnv) (string, error) {
	opts, prompt, err := r.PrepareChild(env.Job)
	if err != nil {
		return "", err
	}
	child, err := r.parent.runtime.newChild(r.parent, env.Job, opts)
	if err != nil {
		return "", err
	}
	if env.BindSession != nil {
		if err := env.BindSession(child.engine.SessionID()); err != nil {
			child.Close()
			return "", err
		}
	}
	env.Log(
		fmt.Sprintf("sub-agent role=%s session=%s parent=%s", env.Job.Role, child.engine.SessionID(), env.Job.ParentID),
	)
	return child.runAssignment(ctx, env.Job.ID, queuedPrompt{text: prompt}, assignmentHooks{
		Intervened: env.MarkIntervention,
		Progress:   env.OnProgress,
	})
}

// childCap is how many children one parent retains at once. It is a budget of
// its own, not a share of the user's tabs: a session opened by hand never
// costs a sub-agent a slot, and a sub-agent never costs the user one.
// job.Manager.MaxConcurrent still bounds how many of them run at the same time.
const childCap = 12

// admitChild reserves one retention slot, releasing the oldest child that has
// already stopped working when every slot is taken. Only a family of twelve
// live children is refused — a finished one is retention, not work, and gives
// way to the child the model is asking for now.
//
// A released record is dropped, never closed here: the UI owns the View built
// around that Controller and reconciles against [Runtime.Children], so it is
// the one that retires both. The loop is bounded by the cap so a concurrent
// spawn can never spin it.
func (r *Runtime) admitChild() error {
	for range childCap + 1 {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return errors.New("cannot open child: runtime is closed")
		}
		if len(r.children)+r.childBuilding < childCap {
			r.childBuilding++
			r.childBuilders++
			r.builders.Add(1)
			r.mu.Unlock()
			return nil
		}
		retained := slices.Clone(r.children)
		r.mu.Unlock()
		// Finished-ness is asked outside r.mu: a Controller answers under its
		// own stream lock, and the two must never be nested.
		released := ""
		for _, child := range retained {
			if child.Controller == nil || child.Controller.childFinished() {
				released = child.JobID
				break
			}
		}
		if released == "" {
			return fmt.Errorf(
				"cannot open child: %d sub-agents are already running; stop one before spawning another",
				childCap,
			)
		}
		r.mu.Lock()
		r.children = slices.DeleteFunc(r.children, func(c ChildSession) bool { return c.JobID == released })
		r.mu.Unlock()
	}
	return fmt.Errorf("cannot open child: the %d retained sub-agent slots keep filling up", childCap)
}

// finishChildBuild releases the reservation admitChild took.
func (r *Runtime) finishChildBuild() {
	r.mu.Lock()
	r.childBuilding--
	r.childBuilders--
	r.mu.Unlock()
	r.builders.Done()
}

// childFinished reports a child that is not working: its assignment is
// terminal, no turn is streaming and no nested work is left.
func (c *Controller) childFinished() bool {
	if c == nil {
		return true
	}
	return c.Assignment().Terminal && !c.RunActive() && c.LiveJobCount() == 0
}

func (r *Runtime) newChild(parent *Controller, meta job.Meta, opts *agent.EngineOpts) (*Controller, error) {
	if err := r.admitChild(); err != nil {
		return nil, err
	}
	defer r.finishChildBuild()
	ws, err := r.Workspace(meta.WorkDir)
	if err != nil {
		return nil, err
	}
	bus := NewBus(parent.bus.onWake)
	c := &Controller{
		runtime: r, workspace: ws, bus: bus, closeDone: make(chan struct{}), jobOwnerID: rand.Text(),
		proj: ws.proj, cwd: ws.cwd, sessionDir: opts.SessionOpts.SessionDir,
		modelCfg: opts.Model, providers: r.providers, opencode: r.opencode,
		mode: agent.ModeUsePlan, planRuntime: r.planRuntime, lspMgr: ws.lspMgr, lspOpen: ws.lspOpen,
		childRole: job.NormalizeRole(string(meta.Role)), childParentID: meta.ParentID, childRounds: opts.MaxRounds,
		childAttached: newChildAttachment(),
		assignment:    newAssignment(meta.ID),
	}
	c.basePolicy = ws.proj.Config().Permissions
	c.initGate(c.basePolicy)
	// A child runs the manager it was handed rather than loading one of its
	// own, so the record of the load that built it comes from the parent it
	// was handed from — not from the child's workspace, which may be another
	// directory whose hooks this child never got.
	c.storeHooks(opts.Hooks, parent.hookLoadFacts())
	c.engine, err = c.newEngine(opts.Model, opts.SessionOpts, opts.Hooks)
	if err != nil {
		return nil, err
	}
	id := c.engine.SessionID()
	c.progressSession.Store(&id)
	c.engineRef.Store(c.engine)
	r.mu.Lock()
	r.sessions[c] = struct{}{}
	ready := c.childAttached.finish
	closed := r.closed
	if !closed {
		r.children = append(
			r.children,
			ChildSession{
				JobID:           meta.ID,
				Name:            meta.Description,
				Title:           tools.SpawnTitle(string(meta.Role), meta.Description, meta.Prompt),
				ParentSessionID: parentSessionID(parent, meta),
				Controller:      c,
				Bus:             bus,
				Workspace:       ws,
				Project:         ws.proj,
				Ready:           ready,
			},
		)
	}
	r.mu.Unlock()
	if closed {
		c.stopSession()
		return nil, errors.New("cannot open child: runtime closed during construction")
	}
	if bus.onWake != nil {
		bus.onWake()
	}
	return c, nil
}

// parentSessionID names the conversation the child belongs to. The job's own
// ParentID is the authority; a spawn that never recorded one falls back to the
// live parent, because a child with no parent has nowhere to be shown.
func parentSessionID(parent *Controller, meta job.Meta) string {
	if meta.ParentID != "" {
		return meta.ParentID
	}
	return parent.SessionID()
}

// ChildJobs is this session's whole sub-agent history, straight from the job
// manager: children this process still retains and children it released alike,
// newest first. The manager's own list spans every session the jobs directory
// remembers, so the parent id is what makes it this conversation's own.
func (c *Controller) ChildJobs(ctx context.Context) ([]job.Info, error) {
	if c == nil || c.jobs == nil {
		return nil, errors.New("sub-agents are not available in this session")
	}
	all, err := c.jobs.List(ctx)
	if err != nil {
		return nil, err
	}
	return childrenOf(all, c.SessionID()), nil
}

// childrenOf keeps the jobs one conversation spawned. A session with no id of
// its own adopts nothing: an empty parent id would otherwise claim every job
// that was recorded without one.
func childrenOf(all []job.Info, parentID string) []job.Info {
	if parentID == "" {
		return nil
	}
	out := make([]job.Info, 0, len(all))
	for _, info := range all {
		if info.ParentID == parentID {
			out = append(out, info)
		}
	}
	return out
}

// ChildJobDir is where one child's transcript and result were written; "" in a
// session with no job manager. It is the answer a surface gives when the
// sub-agent's own session is gone but its files are not.
func (c *Controller) ChildJobDir(jobID string) string {
	if c == nil || c.jobs == nil {
		return ""
	}
	return c.jobs.JobDir(jobID)
}

// CancelChild stops one of this session's children through the manager path
// agent_cancel uses, ownership check included: the panel's x and the model's
// tool must not be able to diverge.
func (c *Controller) CancelChild(ctx context.Context, jobID string) error {
	if c == nil || c.jobs == nil {
		return errors.New("sub-agents are not available in this session")
	}
	return c.jobs.CancelForOwner(ctx, jobID, c.jobOwnerID)
}
