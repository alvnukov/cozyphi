package controller

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/project"
)

// ChildSession is retained independently of its assignment's execution slot.
// UI assembly reads snapshots on its own goroutine and never activates on arrival.
type ChildSession struct {
	JobID      string
	Name       string
	Controller *Controller
	Bus        *Bus
	Workspace  *Workspace
	Project    *project.Project
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
	return child.runAssignment(ctx, env.Job.ID, queuedPrompt{text: prompt}, env.MarkIntervention)
}

func (r *Runtime) newChild(parent *Controller, meta job.Meta, opts *agent.EngineOpts) (*Controller, error) {
	r.mu.Lock()
	if r.closed || len(r.sessions)+r.childBuilders >= 12 {
		r.mu.Unlock()
		return nil, errors.New("cannot open child: runtime closed or retained session limit (12) reached")
	}
	r.childBuilders++
	r.builders.Add(1)
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.childBuilders--
		r.mu.Unlock()
		r.builders.Done()
	}()
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
				JobID:      meta.ID,
				Name:       meta.Description,
				Controller: c,
				Bus:        bus,
				Workspace:  ws,
				Project:    ws.proj,
				Ready:      ready,
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
