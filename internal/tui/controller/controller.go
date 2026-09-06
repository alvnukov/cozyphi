package controller

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/debuglog"
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/memory"
	"github.com/alvnukov/cozyphi/internal/opencode"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/runerror"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/session/compaction"
	"github.com/alvnukov/cozyphi/internal/tasks"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
	"github.com/alvnukov/cozyphi/internal/usage"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// Controller owns agent.Engine lifecycle and stream cancellation.
// It talks to the UI only by publishing Msg values onto the Bus.
//
// Construction: NewController(bus, proj, cwd, resumePath). Callers (cmd)
// assemble collaborators; Controller does not call project.GetDefaultProject.
type Controller struct {
	runtime       *Runtime
	workspace     *Workspace
	ownsRuntime   bool
	closeOnce     sync.Once
	closeDone     chan struct{}
	jobOwnerID    string // immutable live controller identity, not a persisted conversation ID
	childRole     job.Role
	childParentID string
	childRounds   int
	childAttached *childAttachment
	engine        *agent.Engine
	proj          *project.Project

	streamMu      sync.Mutex
	streamCancel  context.CancelFunc
	streamGen     int
	streamRunning bool
	streamStopped bool
	promptQueue   []queuedPrompt
	assignment    *assignment // guarded by streamMu; execution uses the ordinary queue
	streamWG      sync.WaitGroup
	closing       bool
	lifetimes     []completionBarrier // registration and disposal share streamMu
	lastUsage     hooks.SessionUsage  // usage of the last completed turn (streamMu)
	usageWork     usageWork           // subscription reads and confirmed resets (streamMu)
	// switchDone reserves admission; cleanup joins it before touching the engine (streamMu).
	switchDone chan struct{}
	// planGateBlocked records a tool denied by the approval gate (streamMu).
	planGateBlocked bool
	// planApprovalResumePending records an approved active plan waiting for an
	// idle stream. Real tool progress clears it (streamMu).
	planApprovalResumePending bool

	bus *Bus

	sessionDir string
	cwd        string
	modelCfg   llm.ModelConfig
	// modelEffort is the reasoning effort selected for modelCfg; empty runs
	// the model at its configured (or the provider's) default.
	modelEffort llm.ReasoningEffort
	providers   *provider.Manager
	opencode    *opencode.Source
	jobs        *job.Manager
	unsubJobs   func()
	// closeBudget bounds each wait in Close; zero means the default 3s.
	closeBudget time.Duration

	// gate is replaced whenever the policy is rebuilt (SetModel, SetMode), so
	// it is published atomically: a reader must never observe a half-written
	// boundary.
	gate atomic.Pointer[permission.Gate]
	// gateFailure records why the last assembly fell back to a denying gate;
	// empty when the boundary is real. The UI reports it once at startup.
	gateFailure string
	// workspaceRootFn resolves the root every gate is built against. nil means
	// the process workspace; tests substitute a root that cannot resolve to
	// exercise the fail-closed assembly.
	workspaceRootFn func() string
	allowAll        atomic.Bool // session-wide allow-all for this process
	agentsEnabled   atomic.Bool // when false, agent_* tools are not registered
	// agentContextLimit is the persisted agents.context_limit default
	// ceiling; agentContextOverride replaces it for this session only.
	// Both are token counts read at spawn; 0 means unlimited/none.
	agentContextLimit    atomic.Int64
	agentContextOverride atomic.Int64
	// reminderSetting is the persisted General reminder threshold;
	// reminderOverride replaces it for this session only. Both are token
	// counts; 0 means the window-derived default.
	reminderSetting  atomic.Int64
	reminderOverride atomic.Int64
	hooksManager     atomic.Pointer[hooks.Manager]
	mcpPool          *mcp.Pool
	mcpLoad          mcp.LoadFacts
	memory           *memory.Store
	watches          *watch.Manager
	tasks            *tasks.Registry
	unsubWatches     func()
	lspMgr           *lsp.Manager
	lspOpen          lsp.OpenFacts

	// diagnostics is this session's read-only harness view, non-nil only in a
	// process started with --developer-mode and only for a session the user
	// drives. A child controller leaves it nil: the capability is the user's,
	// and a sub-agent is not the user.
	diagnostics *diag.Registry

	// mode is the build/plan/useplan posture; plan overlays ModeReadonly on basePolicy.
	mode              agent.Mode
	planDisabled      bool // mirror of the UIState switch; zero value keeps the plan on
	basePolicy        permission.Policy
	planRuntime       *plangate.Runtime
	planAutoApproveFn func() bool

	// lastJobProgress dedupes identical Progress publishes (key → signature).
	lastJobProgress sync.Map
	progressSession atomic.Pointer[string] // immutable routing identity, independent of View activation
	// engineRef publishes the engine to readers outside the UI goroutine —
	// today the diagnostics collectors, which run on a tool goroutine inside
	// the very turn the engine is driving. It is stored beside
	// progressSession at every replacement, so an observer that reads both
	// can never see one session's engine under another's identity. Taking
	// streamMu here instead would deadlock against the turn the reader is
	// running inside, which is why this is a pointer and not a lock.
	engineRef atomic.Pointer[agent.Engine]

	// watchQueue holds watch events waiting for the model, watchWake is the
	// timer that coalesces a burst of them into one turn, and wakeStreak
	// counts the turns watches have started in a row with no user input
	// between them (all streamMu).
	watchQueue     []watch.Event
	watchWake      *time.Timer
	wakeStreak     int
	wakeSuppressed bool
	outcomeParent  string // durable outcome hint scoped to the original conversation

	// startupModelFallback records that the startup model came from the
	// runtime catalog rather than config, environment, or the user's last
	// pick. It powers the startup notice and keeps persistLastModel from
	// recording a choice the user never made.
	startupModelFallback bool
}

const (
	// watchQueueLimit bounds events waiting for the model. Past it the oldest
	// go: a burst nobody has read yet is best described by its newest events.
	watchQueueLimit = 32
	// watchWakeDelay coalesces a burst into one turn. Events that land within
	// a moment of each other are almost always one thing happening.
	watchWakeDelay = 750 * time.Millisecond
	// maxWakeStreak bounds how many turns watches may start in a row without
	// the user saying anything. It is the brake on the obvious failure — a
	// watch whose events are caused by the turn it woke. Past the streak
	// events still arrive and still show in the transcript; they simply wait
	// for the next thing the user sends instead of starting a turn.
	maxWakeStreak = 5
)

// NewController is the single-session convenience constructor. It owns a private
// Runtime and closes it with the Controller. Multisession assembly uses
// NewRuntime, Runtime.Workspace and Runtime.NewSession instead.
func NewController(
	bus *Bus,
	proj *project.Project,
	cwd, resumePath string,
	histories ...*usage.Store,
) (*Controller, error) {
	if bus == nil {
		return nil, errors.New("tui: nil bus")
	}
	rt, err := NewRuntime(proj, histories...)
	if err != nil {
		return nil, err
	}
	ws, err := rt.Workspace(cwd)
	if err != nil {
		return nil, errors.Join(err, rt.Close())
	}
	c, err := rt.NewSession(bus, ws, resumePath, nil)
	if err != nil {
		return nil, errors.Join(err, rt.Close())
	}
	c.ownsRuntime = true
	return c, nil
}

// newController borrows workspace services; only turn state, watches, permission
// decisions and the engine/session are owned here.
func newController(
	bus *Bus,
	rt *Runtime,
	ws *Workspace,
	resumePath string,
	acquired *session.Manager,
) (*Controller, error) {
	if acquired != nil {
		resumePath = acquired.File()
	}
	config := ws.proj.Config()
	c := &Controller{
		bus: bus, runtime: rt, workspace: ws, closeDone: make(chan struct{}), jobOwnerID: rand.Text(),
		proj: ws.proj, cwd: ws.cwd, sessionDir: ws.proj.SessionDir(),
		modelCfg: config.Model(), providers: rt.providers, opencode: rt.opencode,
		mode: agent.ModeUsePlan, planRuntime: rt.planRuntime,
		memory: ws.memory, tasks: ws.tasks, lspMgr: ws.lspMgr, lspOpen: ws.lspOpen,
		mcpPool: ws.mcpPool, mcpLoad: ws.mcpLoad, jobs: rt.jobs,
	}
	c.applyLastModel(config, resumePath)
	c.applyStartupFallbackModel(resumeSessionModel(resumePath))
	c.watches = watch.New(watch.Options{Cwd: func() string { return c.cwd }})
	c.basePolicy = config.Permissions
	c.initGate(config.Permissions)
	c.agentsEnabled.Store(config.Agents.Enabled)
	hooksManager := ws.hooks
	c.hooksManager.Store(hooksManager)
	// Before the first engine: every engine this controller builds later —
	// clear, resume, model rebind — reads the same capability.
	c.diagnostics = rt.newDiagnostics(c)

	eng, err := c.newEngine(c.runtimeModel(), agent.SessionOpts{
		Cwd: ws.cwd, SessionDir: c.sessionDir, Persist: true, ResumePath: resumePath, Acquired: acquired,
	}, hooksManager)
	if err != nil {
		c.watches.Close()
		return nil, err
	}
	c.engine = eng
	id := eng.SessionID()
	c.progressSession.Store(&id)
	c.engineRef.Store(eng)
	// The engine normalizes what it was handed; keep the normalization but
	// not the applied effort — modelCfg stays the base, and modelEffort is
	// the only source of the applied level.
	base := eng.ModelConfig()
	base.ReasoningEffort = c.modelCfg.ReasoningEffort
	c.modelCfg = base
	if resumePath == "" && !c.startupModelFallback {
		// A resumed session runs the model it was recorded with; that is not
		// the user's latest pick, so it must not become the next fresh
		// session's starting model. A catalog fallback is not a user pick
		// either — the notice names it, but the choice stays unset.
		c.persistLastModel()
	}
	c.startJobProgress()
	c.startWatchEvents()
	c.emitSessionStart("startup", eng.SessionID(), "")
	return c, nil
}

// newEngine is the single assembly point for controller-owned engines. Session
// identity and hooks vary across startup/resume/clear; every other collaborator
// and live policy must follow each replacement engine automatically.
func (c *Controller) newEngine(
	cfg llm.ModelConfig,
	sessionOpts agent.SessionOpts,
	hooksManager *hooks.Manager,
) (*agent.Engine, error) {
	var childTools []tools.Tool
	if c.childRole != "" {
		childTools = agent.SpecForRole(c.childRole).Tools
		sessionOpts.ParentID = c.childParentID
	}
	return agent.NewEngine(agent.EngineOpts{
		Model:       cfg,
		Tools:       childTools,
		MaxRounds:   c.childRounds,
		SessionOpts: sessionOpts,
		Gate:        c.currentGate(),
		Ask:         c.askPermission,
		ContinueAsk: c.askContinue,
		Jobs:        c.engineJobs(),
		JobOwnerID:  c.jobOwnerID,
		JobRunner:   c.bindJobRunner,
		Hooks:       hooksManager,
		MCP:         c.mcpPool,
		Memory:      c.memory,
		Watches:     c.watches,
		Tasks:       c.tasks,
		TasksAccess: c.basePolicy.Tasks,
		LSP:         c.lspQuery(),
		QuestionAsk: c.askQuestion,
		PlanUpdated: c.publishPlan,
		// Plan action runs ride the bus here: the engine emits them from
		// transition doors that may run outside a streaming round.
		SessionEvents: func(ev session.Event) { c.publish(SessionEventMsg{Event: ev}) },
		AutoApprove:   c.planAutoApproveFn,
		PlanRuntime:   c.planRuntime,
		ResolveModel:  c.findModel,
		ModelNames:    c.ModelNames,
		Diagnostics:   c.diagnostics,
	})
}

func (c *Controller) bindJobRunner(
	model llm.ModelConfig, hooksManager *hooks.Manager, query tools.LSPQueryFunc,
) job.Runner {
	models := c.agentModels()
	resolved := make(map[job.Role]llm.ModelConfig)
	for _, role := range job.Roles() {
		if cfg, ok := models.For(role); ok {
			resolved[role] = cfg
		}
	}
	runner := agent.EngineRunner{
		Model: model, Hooks: hooksManager, LSP: query,
		ContextLimit: c.AgentWindowLimit,
		ModelForRole: func(role job.Role) (llm.ModelConfig, bool) {
			cfg, ok := resolved[role]
			return cfg, ok
		},
	}
	if c.runtime != nil {
		c.runtime.mu.Lock()
		interactive := c.runtime.interactiveChildren
		c.runtime.mu.Unlock()
		if interactive {
			return interactiveRunner{EngineRunner: runner, parent: c}
		}
	}
	return runner
}

func (c *Controller) startJobProgress() {
	if c.jobs == nil || c.bus == nil {
		return
	}
	ch, cancel := c.jobs.Subscribe()
	c.unsubJobs = cancel
	go func() {
		for p := range ch {
			id := c.progressSession.Load()
			if id == nil || p.ParentID != *id || p.OwnerID != c.jobOwnerID {
				continue
			}
			if c.shouldPublishJobProgress(p) {
				c.publish(JobProgressMsg{Progress: p})
			}
		}
	}()
}

// terminalToolStatuses are the tool-run statuses after which a child slot
// never publishes again. The vocabulary is session's; Progress carries the
// strings.
var terminalToolStatuses = map[string]bool{
	session.ToolDone.String():      true,
	session.ToolError.String():     true,
	session.ToolCancelled.String(): true,
	session.ToolRejected.String():  true,
}

// shouldPublishJobProgress drops duplicate progress for the same child tool
// slot (same status/detail/name). Status transitions and new children still
// publish. A terminal status publishes and drops the key: the slot is closed,
// so the map tracks live slots instead of the session's whole history.
func (c *Controller) shouldPublishJobProgress(p job.Progress) bool {
	key := p.JobID + "\x00" + p.ToolUseID
	if p.ToolUseID == "" {
		key = p.JobID + "\x00" + p.Name + "\x00" + p.Detail
	}
	sig := p.Status + "\x00" + p.Name + "\x00" + p.Detail
	if prev, ok := c.lastJobProgress.Load(key); ok && prev.(string) == sig {
		return false
	}
	if terminalToolStatuses[p.Status] {
		c.lastJobProgress.Delete(key)
		return true
	}
	c.lastJobProgress.Store(key, sig)
	return true
}

// startWatchEvents fans background watch events into the session: every one
// shows in the transcript, and every one is queued for the model.
func (c *Controller) startWatchEvents() {
	if c.watches == nil || c.bus == nil {
		return
	}
	ch, cancel := c.watches.Subscribe()
	c.unsubWatches = cancel
	go func() {
		for ev := range ch {
			c.observeWatchEvent(ev)
		}
	}()
}

// observeWatchEvent puts one event in front of the user at once, and in front
// of the model when there is somewhere to put it: a running turn picks it up
// at its next tool-round boundary, and an idle session wakes for it.
func (c *Controller) observeWatchEvent(ev watch.Event) {
	c.publish(SessionEventMsg{Event: session.WatchFired{
		ID:    fmt.Sprintf("%s-%d", ev.ID, ev.Time.UnixNano()),
		Label: ev.Label,
		Text:  ev.Text,
	}})

	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.closing {
		return
	}
	c.watchQueue = append(c.watchQueue, ev)
	if over := len(c.watchQueue) - watchQueueLimit; over > 0 {
		c.watchQueue = slices.Delete(c.watchQueue, 0, over)
	}
	if c.switchDone != nil || c.streamRunning || c.wakeSuppressed || c.watchWake != nil ||
		c.wakeStreak >= maxWakeStreak {
		return
	}
	c.watchWake = time.AfterFunc(watchWakeDelay, c.wakeForWatches)
}

// wakeForWatches starts a turn for events that arrived with nothing running.
// Everything it checks may have changed during the coalescing delay, so it
// checks all of it again.
func (c *Controller) wakeForWatches() {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	c.watchWake = nil
	if c.closing || c.switchDone != nil || c.streamRunning || c.wakeSuppressed ||
		(len(c.watchQueue) == 0 && !c.hasPendingOutcomesLocked()) || c.wakeStreak >= maxWakeStreak {
		return
	}
	if len(c.watchQueue) == 0 && !c.reconcileOutcomeHintLocked() {
		return
	}
	c.wakeStreak++
	c.startPromptLocked("", nil, nil)
}

// drainWatchLocked takes the queued events and disarms any pending wake for
// them. The caller holds streamMu.
func (c *Controller) drainWatchLocked() []watch.Event {
	if c.watchWake != nil {
		c.watchWake.Stop()
		c.watchWake = nil
	}
	events := c.watchQueue
	c.watchQueue = nil
	return events
}

func (c *Controller) initGate(policy permission.Policy) {
	if c.childRole != "" {
		// Role ceilings narrow configured rules; never replace user/org denials
		// with defaults when a child changes mode or rebuilds its engine.
		policy.DangerouslyAllowAll = false
		policy.WorkspaceOnlyWrites = true
		policy.MemoryDir = ""
		roleMode := agent.SpecForRole(c.childRole).Mode
		if policy.Mode == "" {
			policy.Mode = roleMode
		}
		if roleMode == permission.ModeReadonly || c.mode == agent.ModePlan {
			policy.Mode = permission.ModeReadonly
		}
		gate, err := permission.NewGate(policy, c.workspaceRoot())
		if err != nil {
			c.setGate(permission.UnavailableGate{Reason: err.Error()})
			return
		}
		c.setGate(gate)
		return
	}
	if policy.Mode == "" {
		policy.Mode = permission.ModeInteractive
	}
	// Plan overlays readonly on whatever the config says; SetModel re-inits
	// the gate from disk, so the overlay lives here to survive that rebuild.
	if c.mode == agent.ModePlan {
		policy.Mode = permission.ModeReadonly
	}
	if policy.DangerouslyAllowAll {
		c.allowAll.Store(true)
	}
	policy.MemoryDir = c.memory.Dir()
	// Only an explicit dangerously_allow_all opts into bypass. Never clear
	// allowAll here: the runtime palette toggle must survive SetModel / re-init.
	root := c.workspaceRoot()
	inner, err := permission.NewGate(policy, root)
	if err != nil {
		configured := err
		inner, err = permission.NewGate(permission.DefaultPolicy(), root)
		if err != nil {
			// Both the configured policy and the built-in default failed to
			// compile: the session has no rules at all. A gate that allowed
			// everything here would turn a broken workspace root into an
			// unguarded session, so the boundary denies and says why.
			reason := fmt.Sprintf("%v (default policy: %v)", configured, err)
			debuglog.Logf("permission: gate assembly failed: %s", reason)
			c.gateFailure = reason
			c.setGate(&permission.BypassGate{
				Inner:   permission.UnavailableGate{Reason: reason},
				Enabled: &c.allowAll,
			})
			return
		}
		debuglog.Logf("permission: configured policy rejected, using the default: %v", configured)
	}
	c.gateFailure = ""
	c.setGate(&permission.BypassGate{Inner: inner, Enabled: &c.allowAll})
}

// AllowAll reports whether permission prompts are bypassed for this session.
func (c *Controller) AllowAll() bool {
	if c == nil {
		return true
	}
	return c.allowAll.Load()
}

// Mode returns the current posture (useplan by default).
func (c *Controller) Mode() agent.Mode {
	if c == nil || c.mode == "" {
		return agent.ModeUsePlan
	}
	return c.mode
}

// SetMode switches the build/plan/useplan posture: the gate is rebuilt with
// the readonly overlay for plan, and the engine swaps its system prompt and
// read-only tool set. Takes effect from the next tool round. With the plan
// feature off, plan mode falls back to useplan.
func (c *Controller) SetMode(m agent.Mode) {
	if c == nil {
		return
	}
	switch m {
	case agent.ModeBuild, agent.ModePlan, agent.ModeUsePlan:
	default:
		m = agent.ModeUsePlan
	}
	if m == agent.ModePlan && c.planDisabled {
		m = agent.ModeUsePlan
	}
	c.mode = m
	c.initGate(c.basePolicy)
	if c.engine != nil {
		c.engine.SetPermission(c.currentGate(), c.askPermission)
		c.engine.SetMode(c.mode)
	}
}

// ToggleMode cycles build → plan → useplan → build and returns the new mode.
// An empty/unknown mode is treated as the useplan default, so the first
// toggle from a zero-value controller lands on build. With the plan feature
// off, the cycle skips the plan hop entirely.
func (c *Controller) ToggleMode() agent.Mode {
	if c == nil {
		return agent.ModeUsePlan
	}
	switch c.mode {
	case agent.ModeBuild:
		c.SetMode(agent.ModePlan)
	case agent.ModePlan:
		c.SetMode(agent.ModeUsePlan)
	case agent.ModeUsePlan:
		c.SetMode(agent.ModeBuild)
	default:
		c.SetMode(agent.ModeBuild)
	}
	return c.mode
}

// SetAllowAll enables or disables session-wide permission bypass.
func (c *Controller) SetAllowAll(v bool) {
	if c == nil {
		return
	}
	c.allowAll.Store(v)
}

// AgentsEnabled reports whether sub-agent tools are registered on the main engine.
func (c *Controller) AgentsEnabled() bool {
	if c == nil {
		return false
	}
	return c.agentsEnabled.Load()
}

// SetAgentsEnabled registers or removes agent_* tools for this session.
func (c *Controller) SetAgentsEnabled(v bool) {
	if c == nil {
		return
	}
	c.agentsEnabled.Store(v)
	if c.engine != nil {
		c.engine.SetJobs(c.engineJobs())
	}
}

// StopOnLimit reports whether the engine stops the turn at the tool-round cap.
func (c *Controller) StopOnLimit() bool {
	if c == nil || c.engine == nil {
		return true
	}
	return c.engine.StopOnLimit()
}

// SetStopOnLimit toggles whether the engine stops the turn at the tool-round cap.
func (c *Controller) SetStopOnLimit(enabled bool) {
	if c == nil || c.engine == nil {
		return
	}
	c.engine.SetStopOnLimit(enabled)
}

// SetPlanEnabled toggles the plan feature live on the engine: the plan tool,
// the plan-gate prompt blocks and plan_step injection. Non-destructive — the
// durable plan survives a disable. The controller mirrors the switch so mode
// cycling can skip the plan posture while the feature is off.
func (c *Controller) SetPlanEnabled(enabled bool) {
	if c == nil {
		return
	}
	c.planDisabled = !enabled
	if c.engine != nil {
		c.engine.SetPlanEnabled(enabled)
	}
}

// engineJobs returns the job manager only when sub-agents are enabled.
func (c *Controller) engineJobs() *job.Manager {
	if c == nil || !c.agentsEnabled.Load() {
		return nil
	}
	return c.jobs
}

// lspQuery borrows the shared manager's query function, never its lifecycle.
func (c *Controller) lspQuery() lsp.QueryFunc {
	if c == nil || c.lspMgr == nil {
		return nil
	}
	return c.lspMgr.Query
}

// Hooks returns the currently loaded hooks manager (may be nil).
func (c *Controller) Hooks() *hooks.Manager {
	if c == nil {
		return nil
	}
	return c.hooksManager.Load()
}

// ReloadHooks re-discovers hooks from disk and swaps the manager on the engine
// (and on future sub-agents via Hooks()).
func (c *Controller) ReloadHooks() (loaded int, warns []hooks.Warning, err error) {
	if c == nil {
		return 0, nil, errors.New("controller not initialized")
	}
	proj := c.proj
	if proj == nil {
		return 0, nil, errors.New("project not available")
	}
	found, warns, err := hooks.Discover(proj.Global().HooksDir(), proj.HooksDir())
	if err != nil {
		return 0, warns, err
	}
	mgr := hooks.NewManager(hooks.EntriesFromDiscovered(found)...)
	hooks.LogWarnings(warns)
	c.hooksManager.Store(mgr)
	if c.engine != nil {
		c.engine.SetHooks(mgr)
	}
	return len(found), warns, nil
}

// ListHooks returns the current on-disk discovery (does not swap the manager).
func (c *Controller) ListHooks() ([]hooks.Discovered, []hooks.Warning, error) {
	if c == nil {
		return nil, nil, errors.New("controller not initialized")
	}
	proj := c.proj
	if proj == nil {
		return nil, nil, errors.New("project not available")
	}
	return hooks.Discover(proj.Global().HooksDir(), proj.HooksDir())
}

// MCPServers returns the sorted configured MCP server names (nil when the
// pool is disabled) — the status sidebar data source.
func (c *Controller) MCPServers() []string {
	if c == nil {
		return nil
	}
	return c.mcpPool.ServerNames()
}

// MCPStatuses returns the latest sorted connection states for the status panel.
func (c *Controller) MCPStatuses() []mcp.ServerStatus {
	if c == nil {
		return nil
	}
	if c.mcpLoad.Failed {
		return []mcp.ServerStatus{{Name: "configuration", State: mcp.StateFailed}}
	}
	return c.mcpPool.ServerStatuses()
}

// ToggleMCPServer switches a configured MCP server on or off for the whole
// stack at once: the pool hides it from the mcp_* tools and the prompt
// catalog immediately, the engine rebinds so the next round carries the
// refreshed prompt, and the user config persists the choice across
// sessions. A failing persist leaves the session toggled and returns the
// error — the next toggle rewrites the file.
func (c *Controller) ToggleMCPServer(name string, enabled bool) error {
	if c == nil || c.mcpPool == nil {
		return errors.New("mcp is not available")
	}
	if err := c.mcpPool.SetEnabled(name, enabled); err != nil {
		return err
	}
	if err := mcp.SetDisabled(c.mcpPool.DisabledNames()); err != nil {
		return err
	}
	if c.engine != nil {
		c.engine.RefreshTools()
	}
	return nil
}

// LSPStatuses returns the bounded language-server inventory for the status
// panel. The languages operation never spawns a process: it reports the frozen
// V1 profile plus the current live-client count.
func (c *Controller) LSPStatuses() []lsp.Language {
	if c == nil || c.lspMgr == nil {
		return nil
	}
	res, err := c.lspMgr.Query(context.Background(), lsp.Query{Op: lsp.OpLanguages})
	if err != nil {
		return nil
	}
	return res.Languages
}

// Plan returns the latest durable plan for the active session.
func (c *Controller) Plan() session.Plan {
	if c == nil || c.engine == nil {
		return session.Plan{}
	}
	return c.engine.Plan()
}

// PlanRuntime returns the live policy source shared by every replacement engine.
func (c *Controller) PlanRuntime() *plangate.Runtime {
	if c == nil {
		return nil
	}
	return c.planRuntime
}

// RenamePlanStepTypes migrates current-plan references for a settings commit.
func (c *Controller) RenamePlanStepTypes(
	ctx context.Context,
	renames map[session.StepType]session.StepType,
) (session.Plan, error) {
	if c == nil || c.engine == nil {
		return session.Plan{}, errors.New("tui: engine unavailable")
	}
	return c.engine.RenamePlanStepTypes(ctx, renames)
}

// ToolNames reports tools available in the engine's useplan projection —
// the gateable catalog regardless of the current posture.
func (c *Controller) ToolNames() []string {
	if c == nil || c.engine == nil {
		return nil
	}
	return c.engine.ToolNames()
}

// PlanUsesType reports whether the current plan references a step type; the
// settings modal uses it to block deleting a type the plan still carries.
func (c *Controller) PlanUsesType(typ session.StepType) bool {
	if c == nil {
		return false
	}
	for _, item := range c.engine.Plan().Items {
		if item.Type == typ {
			return true
		}
	}
	return false
}

// SetPlanAutoApprove binds the auto-approve policy the engine consults when the
// model revises the plan, so a plan edit comes back approved in the same turn.
func (c *Controller) SetPlanAutoApprove(fn func() bool) {
	if c == nil {
		return
	}
	c.planAutoApproveFn = fn
	if c.engine != nil {
		c.engine.SetAutoApprove(fn)
	}
}

// approvalResumePrompt nudges the model to continue the turn it could not
// start while the plan was unapproved.
const approvalResumePrompt = "The plan is now approved. Continue the task."

// SetPlanApproved flips the durable plan approval flag and republishes the
// plan so the sidebar checkbox follows the authoritative state. Approving
// hands control to the model: an in-flight run re-checks the gate on its next
// tool call, and an idle turn with active work resumes even when the model
// stopped immediately after drafting its plan. Unapproving stops the model
// and drops queued prompts, because a revoked plan must not drive tool calls.
func (c *Controller) SetPlanApproved(approved bool) error {
	if c == nil || c.engine == nil {
		return errors.New("controller: no engine")
	}
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.closing {
		return errors.New("controller: shutting down")
	}
	plan, err := c.engine.SetPlanApproved(approved)
	if err != nil {
		return err
	}
	// Approval is a control handoff. Arm a resume even when no tool reached
	// the gate: models commonly stop after drafting a plan and wait for the
	// checkbox. A successful gateable tool clears the pending resume.
	if approved && hasActivePlanStep(plan) {
		c.planApprovalResumePending = true
	}
	if !approved {
		c.planGateBlocked = false
		c.planApprovalResumePending = false
		c.streamStopped = true
		c.dropQueuedPromptsLocked()
		if c.streamCancel != nil {
			c.streamCancel()
		}
	}
	c.publishPlan(plan)
	if approved {
		c.maybeResumeApprovedWorkLocked()
	}
	return nil
}

// ClearPlan drops the durable plan, resets its revision counter, and republishes
// the empty snapshot so the sidebar shows the reset immediately. An emptied
// plan has nothing left to gate, so approval/resume state is dropped too.
func (c *Controller) ClearPlan() error {
	if c == nil || c.engine == nil {
		return errors.New("controller: no engine")
	}
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.closing {
		return errors.New("controller: shutting down")
	}
	plan, err := c.engine.ClearPlan()
	if err != nil {
		return err
	}
	c.planGateBlocked = false
	c.planApprovalResumePending = false
	c.publishPlan(plan)
	return nil
}

// PatchPlan applies a UI-authored batch of plan edits against the expected
// revision. The durable path is the same one the model tool uses, so the
// projection rebinds and the bus carries the fresh snapshot; a stale
// revision fails closed and names the actual revision.
func (c *Controller) PatchPlan(
	ctx context.Context,
	expectedRevision uint64,
	ops []session.PlanPatchOp,
) (session.Plan, error) {
	if c == nil || c.engine == nil {
		return session.Plan{}, errors.New("controller: no engine")
	}
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.closing {
		return session.Plan{}, errors.New("controller: shutting down")
	}
	plan, _, err := c.engine.PatchPlanFromUser(ctx, expectedRevision, ops)
	return plan, err
}

// CreatePlan stores a UI-authored contract as the session's first plan
// through the same durable path the model tool uses; patching cannot grow a
// plan from nothing, so the editor's empty-session save lands here.
func (c *Controller) CreatePlan(
	ctx context.Context,
	contract session.PlanV2,
) (session.Plan, error) {
	if c == nil || c.engine == nil {
		return session.Plan{}, errors.New("controller: no engine")
	}
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.closing {
		return session.Plan{}, errors.New("controller: shutting down")
	}
	// The editor pane has no advisory surface, so the create's soft-limit
	// warnings are dropped here rather than threaded into the editor's error
	// banner — the durable write already succeeded.
	plan, _, _, err := c.engine.CreatePlan(ctx, contract)
	return plan, err
}

// SetStepModel pins or clears one step's model override through the same
// durable patch path the plan tool uses; the revision is read under the
// stream lock so a stale pick fails closed instead of guessing. The fresh
// snapshot rides the bus back to the sidebar.
func (c *Controller) SetStepModel(stepID, model string) error {
	if c == nil || c.engine == nil {
		return errors.New("controller: no engine")
	}
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.closing {
		return errors.New("controller: shutting down")
	}
	ops := []session.PlanPatchOp{{
		Op:    session.PlanPatchUpdateStep,
		ID:    stepID,
		Model: session.PatchValue[string]{Set: true, Value: model},
		// An explicit human choice replaces the authored reasoning override too.
		Effort: session.PatchValue[string]{Set: true},
	}}
	_, _, err := c.engine.PatchPlan(context.Background(), c.engine.Plan().Revision, ops)
	return err
}

// SetStepSkill flips one step-skill's off mark through the durable in-place
// path that keeps run history. The toggle is material, so a live plan loses
// approval and waits for a re-approve; the fresh snapshot rides the bus back
// to the sidebar.
func (c *Controller) SetStepSkill(stepID string, actionIndex int, skill string, disabled bool) error {
	if c == nil || c.engine == nil {
		return errors.New("controller: no engine")
	}
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.closing {
		return errors.New("controller: shutting down")
	}
	_, err := c.engine.SetPlanSkillDisabled(stepID, actionIndex, skill, disabled)
	return err
}

// maybeResumeApprovedWorkLocked resumes approved work once the stream is idle.
// A real gate denial preserves the legacy resume path; direct approval resumes
// only while the current plan still has active work. The caller holds streamMu.
func (c *Controller) maybeResumeApprovedWorkLocked() {
	if c.closing || c.switchDone != nil || c.streamRunning || c.engine == nil {
		return
	}
	plan := c.engine.Plan()
	directApprovalReady := c.planApprovalResumePending && hasActivePlanStep(plan)
	if c.planApprovalResumePending && !directApprovalReady {
		c.planApprovalResumePending = false
	}
	if !plan.Approved || (!c.planGateBlocked && !directApprovalReady) {
		return
	}
	c.startPromptLocked(approvalResumePrompt, nil, nil)
}

func hasActivePlanStep(plan session.Plan) bool {
	for _, item := range plan.Items {
		// Pending counts as active work: plan create defaults every step to
		// pending, so a freshly drafted plan must resume on approval too.
		// Blocked steps still wait on their resume condition. Completed,
		// cancelled and superseded steps carry no work left to run.
		if item.Status == session.PlanPending || item.Status == session.PlanInProgress ||
			item.Status == session.PlanBlocked {
			return true
		}
	}
	return false
}

// ProviderOptions returns safe catalog metadata for the connection UI.
func (c *Controller) ProviderOptions() []provider.Info {
	if c == nil || c.providers == nil {
		return nil
	}
	return c.providers.Providers()
}

// RefreshProviders refreshes the validated last-known-good catalog.
func (c *Controller) RefreshProviders(ctx context.Context) error {
	if c == nil || c.providers == nil {
		return errors.New("provider manager not available")
	}
	catalogErr := c.providers.Refresh(ctx)
	modelsErr := c.providers.RefreshSubscriptionModels(ctx)
	return errors.Join(catalogErr, modelsErr)
}

// RefreshSubscriptionModels refreshes account-specific provider models while
// leaving the last-known-good catalog live on failure.
func (c *Controller) RefreshSubscriptionModels(ctx context.Context) error {
	if c == nil || c.providers == nil {
		return errors.New("provider manager not available")
	}
	return c.providers.RefreshSubscriptionModels(ctx)
}

// BeginProviderAuthorization starts a browser subscription flow.
func (c *Controller) BeginProviderAuthorization(
	ctx context.Context,
	providerID string,
) (provider.BrowserAuthorization, error) {
	if c == nil || c.providers == nil {
		return provider.BrowserAuthorization{}, errors.New("provider manager not available")
	}
	return c.providers.BeginBrowserAuthorization(ctx, providerID)
}

// CompleteProviderAuthorization waits for a browser subscription flow.
func (c *Controller) CompleteProviderAuthorization(
	ctx context.Context,
	flow provider.BrowserAuthorization,
) error {
	if c == nil || c.providers == nil {
		return errors.New("provider manager not available")
	}
	return c.providers.CompleteBrowserAuthorization(ctx, flow)
}

// BeginProviderDeviceAuthorization starts a headless subscription flow for a
// machine with no browser to hand off to.
func (c *Controller) BeginProviderDeviceAuthorization(
	ctx context.Context,
	providerID string,
) (provider.DeviceAuthorization, error) {
	if c == nil || c.providers == nil {
		return provider.DeviceAuthorization{}, errors.New("provider manager not available")
	}
	return c.providers.BeginDeviceAuthorization(ctx, providerID)
}

// CompleteProviderDeviceAuthorization polls a headless subscription flow.
func (c *Controller) CompleteProviderDeviceAuthorization(
	ctx context.Context,
	flow provider.DeviceAuthorization,
) error {
	if c == nil || c.providers == nil {
		return errors.New("provider manager not available")
	}
	return c.providers.CompleteDeviceAuthorization(ctx, flow)
}

// ConnectProvider stores a credential for the exact endpoint approved by the user.
func (c *Controller) ConnectProvider(req provider.ConnectRequest) error {
	if c == nil || c.providers == nil {
		return errors.New("provider manager not available")
	}
	return c.providers.Connect(req)
}

// ModelNames returns configured, connected-provider, and opencode models without duplicates.
func (c *Controller) ModelNames() []string {
	if c == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var names []string
	for _, cfg := range c.modelCatalog() {
		if _, ok := seen[cfg.Name]; ok || cfg.Name == "" {
			continue
		}
		seen[cfg.Name] = struct{}{}
		names = append(names, cfg.Name)
	}
	return names
}

func (c *Controller) findModel(name string) (llm.ModelConfig, bool) {
	for _, cfg := range c.modelCatalog() {
		if cfg.Name == name {
			return c.skillPathOrDefault(cfg), true
		}
	}
	// Legacy "name:effort" selectors predate effort being a separate choice;
	// they survive in remembered picks, resumed sessions, agent pins and
	// typed /model arguments. The base name resolves and the suffix names a
	// level the model accepts, the resolved config carries the effort.
	base, suffix, ok := splitLegacyEffortName(name)
	if !ok {
		return llm.ModelConfig{}, false
	}
	for _, cfg := range c.modelCatalog() {
		if cfg.Name != base {
			continue
		}
		effort, valid := llm.ParseReasoningEffort(suffix)
		if !valid || !slices.Contains(cfg.ReasoningEfforts, effort) {
			return llm.ModelConfig{}, false
		}
		cfg = c.skillPathOrDefault(cfg)
		cfg.ReasoningEffort = effort
		return cfg, true
	}
	return llm.ModelConfig{}, false
}

// splitLegacyEffortName splits a "name:effort" selector after its last
// colon. ok is false for names without one.
func splitLegacyEffortName(name string) (base, suffix string, ok bool) {
	at := strings.LastIndex(name, ":")
	if at <= 0 || at == len(name)-1 {
		return "", "", false
	}
	return name[:at], name[at+1:], true
}

// effortSupported reports whether cfg accepts effort as a runtime choice.
func effortSupported(cfg llm.ModelConfig, effort llm.ReasoningEffort) bool {
	return effort != "" && slices.Contains(cfg.ReasoningEfforts, effort)
}

// skillPathOrDefault fills a catalog model's empty skill path from the project
// config, so a provider or opencode pick behaves like a configured one at
// every place it is resolved.
func (c *Controller) skillPathOrDefault(cfg llm.ModelConfig) llm.ModelConfig {
	if cfg.SkillPath == "" && c.proj != nil && c.proj.Config() != nil {
		cfg.SkillPath = c.proj.Config().SkillPath
	}
	return cfg
}

func (c *Controller) modelCatalog() []llm.ModelConfig {
	if c == nil {
		return nil
	}
	var models []llm.ModelConfig
	if c.proj != nil && c.proj.Config() != nil {
		models = append(models, c.proj.Config().AllModels()...)
	}
	if c.providers != nil {
		models = append(models, c.providers.Models()...)
	}
	models = append(models, c.opencode.Models()...)
	return models
}

// agentModels resolves agents.models pins the way this session can: against
// the configured models and the connected provider catalog, which is what the
// settings picker offers.
func (c *Controller) agentModels() project.AgentModels {
	if c == nil || c.proj == nil {
		return project.AgentModels{}
	}
	return c.proj.Config().AgentModels(c.findModel)
}

// SetAgentContextLimit applies the persisted agents.context_limit live: it
// is the session's default ceiling, and the next spawn narrows every child's
// context window to the effective ceiling (a session override may widen it).
func (c *Controller) SetAgentContextLimit(tokens int) {
	if c == nil {
		return
	}
	c.agentContextLimit.Store(int64(max(tokens, 0)))
}

// SetSessionAgentContext sets or clears this session's sub-agent ceiling —
// nothing is persisted, and a fresh session starts from the General value
// (unlimited when that is 0) again. The override replaces the General value
// for this session; it does not clamp to it.
func (c *Controller) SetSessionAgentContext(tokens int) {
	if c == nil {
		return
	}
	c.agentContextOverride.Store(int64(max(tokens, 0)))
}

// AgentWindowLimit is the ceiling every sub-agent window is narrowed to at
// spawn: this session's override, else the persisted General limit. Zero
// means unlimited.
func (c *Controller) AgentWindowLimit() int {
	if c == nil {
		return 0
	}
	if override := c.agentContextOverride.Load(); override > 0 {
		return int(override)
	}
	return int(c.agentContextLimit.Load())
}

// SetReminderThreshold applies the persisted General reminder threshold and
// remembers it as this session's fallback: clearing a session override later
// returns here, not to the window-derived default.
func (c *Controller) SetReminderThreshold(tokens int) {
	if c == nil {
		return
	}
	c.reminderSetting.Store(int64(max(tokens, 0)))
	if c.reminderOverride.Load() <= 0 {
		c.applyReminder(int64(max(tokens, 0)))
	}
}

// SetSessionReminderThreshold narrows (0 restores the General value) the
// compact reminder threshold for this session only. Nothing is persisted.
func (c *Controller) SetSessionReminderThreshold(tokens int) {
	if c == nil {
		return
	}
	stored := int64(max(tokens, 0))
	c.reminderOverride.Store(stored)
	if stored > 0 {
		c.applyReminder(stored)
	} else {
		c.applyReminder(c.reminderSetting.Load())
	}
}

// ReminderThreshold is the effective compact reminder threshold: the session
// override, else the General value, else the window-derived default
// (the model's window minus the compaction headroom). Zero means the window
// is unknown and no threshold can be derived.
func (c *Controller) ReminderThreshold() int {
	if c == nil {
		return 0
	}
	if override := c.reminderOverride.Load(); override > 0 {
		return int(override)
	}
	if setting := c.reminderSetting.Load(); setting > 0 {
		return int(setting)
	}
	window := c.modelCfg.ContextWindow
	if c.engine != nil {
		window = c.engine.ModelConfig().ContextWindow
	}
	return compaction.ConfiguredSettings(0).ReminderThreshold(window)
}

// applyReminder pushes a reminder threshold into the live engine's
// compaction policy; 0 keeps the window-derived default.
func (c *Controller) applyReminder(tokens int64) {
	if c.engine != nil {
		c.engine.SetCompactionSettings(compaction.ConfiguredSettings(int(tokens)))
	}
}

// agentModelFor resolves the pin for a role. A role without a pin — or a name
// that no longer resolves (unconnected provider, stale catalog) — reports
// false so the spawn inherits the session model instead of failing.
func (c *Controller) agentModelFor(role job.Role) (llm.ModelConfig, bool) {
	return c.agentModels().For(role)
}

// applyLastModel pins a fresh session to the last model the user picked, unless
// a resume path or an explicit COZYPHI_MODEL override outranks it. A remembered
// name that no longer resolves silently keeps the configured default, and a
// remembered effort the model does not accept is dropped the same way.
func (c *Controller) applyLastModel(config *project.Config, resumePath string) {
	if resumePath != "" || config.ModelEnvOverride() {
		return
	}
	state, err := project.LoadUIState(c.proj.Global())
	if err != nil || state.LastModel == "" {
		return
	}
	if last, ok := c.findModel(state.LastModel); ok {
		c.modelCfg = last
		c.modelEffort = c.switchEffort(last)
		// A separately remembered effort outranks the one a legacy
		// "name:effort" pick carries, because it was recorded later.
		if remembered, valid := llm.ParseReasoningEffort(state.LastEffort); valid &&
			effortSupported(last, remembered) {
			c.modelEffort = remembered
		}
		// A runtime level the remembered pick names belongs to the
		// selection, not to the stored base: clearing the selection later
		// must return to the provider default, not to that level.
		if effortSupported(c.modelCfg, c.modelCfg.ReasoningEffort) {
			c.modelCfg.ReasoningEffort = ""
		}
	}
}

// applyStartupFallbackModel picks the first runtime-catalog model when
// startup found no model anywhere: no config entry, no COZYPHI_* override,
// no remembered pick. The TUI must start — a connected provider or an
// opencode install is a usable model, and the startup notice names what was
// picked so the user can switch with /model. resumeModel is the model the
// resumed session runs on: when it still resolves, the session supplies the
// model, and neither the fallback nor its notice may claim that pick.
func (c *Controller) applyStartupFallbackModel(resumeModel string) {
	if c.modelCfg.Name != "" {
		return
	}
	if resumeModel != "" {
		if _, ok := c.findModel(resumeModel); ok {
			return
		}
	}
	for _, cfg := range c.modelCatalog() {
		if cfg.Name == "" {
			continue
		}
		c.modelCfg = c.skillPathOrDefault(cfg)
		c.startupModelFallback = true
		return
	}
}

// resumeSessionModel reads the model recorded by the session being resumed,
// so startup can tell a session that supplies its own model from one that
// needs the catalog fallback. Read failures degrade to "": the engine's own
// open then decides what the session runs on.
func resumeSessionModel(resumePath string) string {
	if resumePath == "" {
		return ""
	}
	model, err := session.ReadSessionModel(resumePath)
	if err != nil {
		return ""
	}
	return model
}

// persistLastModel remembers the active model name and reasoning effort in
// global UI state so the next fresh session starts where this one left off.
// Only a model the user chose is recorded — resuming a session adopts its
// recorded model, which applyLastModel deliberately ignores, so recording it
// would move the default behind the user's back. Persistence is best-effort:
// a write failure must not block the session.
func (c *Controller) persistLastModel() {
	if c != nil && c.childRole != "" {
		return
	}
	if c == nil || c.proj == nil || c.modelCfg.Name == "" {
		return
	}
	if err := project.MutateUIState(c.proj.Global(), func(s *project.UIState) {
		s.LastModel = c.modelCfg.Name
		s.LastEffort = string(c.modelEffort)
	}); err != nil {
		debuglog.Logf("ui: persist last model: %v", err)
	}
}

// ModelName returns the active model label.
func (c *Controller) ModelName() string {
	if c == nil {
		return ""
	}
	return c.modelCfg.Name
}

// Effort returns the reasoning effort selected for the active model, "" when
// it runs at the configured or provider default.
func (c *Controller) Effort() string {
	if c == nil {
		return ""
	}
	return string(c.modelEffort)
}

// ModelEfforts returns the reasoning effort levels a catalog name accepts as
// a runtime choice. Empty means the model has none and an effort cannot be
// selected for it.
func (c *Controller) ModelEfforts(name string) []string {
	if c == nil {
		return nil
	}
	cfg, ok := c.findModel(strings.TrimSpace(name))
	if !ok || len(cfg.ReasoningEfforts) == 0 {
		return nil
	}
	levels := make([]string, 0, len(cfg.ReasoningEfforts))
	for _, effort := range cfg.ReasoningEfforts {
		levels = append(levels, string(effort))
	}
	return levels
}

// noModelLabel is what every model display shows when the session has no
// model at all: a placeholder beats an empty name, which reads as a
// rendering bug rather than a missing configuration.
const noModelLabel = "no model"

// ModelSetupNotice returns the first-run model guidance shown once at
// startup: how to get a model when none is configured, or a note naming the
// automatically picked fallback. Empty when the model came from config, the
// environment, or the user's remembered pick — those need no introduction.
func (c *Controller) ModelSetupNotice() string {
	if c == nil || c.proj == nil {
		return ""
	}
	if name := c.configuredModelName(); name != "" {
		if !c.startupModelFallback {
			return ""
		}
		return "Using " + name + " from the model catalog. /connect manages sign-ins, /model switches."
	}
	return "No model configured. /connect signs in to a provider, /model picks a model, " +
		"or edit " + c.proj.Global().ConfigFile() + " (cozyphi config)."
}

// EffectiveModelName returns the model the engine is actually running right
// now — a live turn may resolve a different model than the session default.
// The sidebar status shows this one; ModelName stays the session default.

// SetTasksAccess applies a committed permissions.tasks level live: the gate
// decides task writes by it from the next call, and the engine carries the
// task tool at that level (or drops it) from the next round. The base
// policy keeps it so a later mode switch rebuilds the same gate.
func (c *Controller) SetTasksAccess(level tasks.Access) {
	if c == nil {
		return
	}
	c.basePolicy.Tasks = level.Normalized()
	c.initGate(c.basePolicy)
	if c.engine != nil {
		c.engine.SetPermission(c.currentGate(), c.askPermission)
		c.engine.SetTasksAccess(c.basePolicy.Tasks)
	}
}

// TasksAccess reports the permissions.tasks level the session runs under.
func (c *Controller) TasksAccess() tasks.Access {
	if c == nil {
		return tasks.AccessOff
	}
	return c.basePolicy.Tasks.Normalized()
}

// RefreshProjectConfig reloads the project config from disk so edits made
// outside the session — the settings modal's agents.models pins — reach
// live consumers such as the sub-agent spawn seam without a restart.
func (c *Controller) RefreshProjectConfig() error {
	if c == nil {
		return errors.New("controller unavailable")
	}
	if c.proj == nil {
		return errors.New("project not available")
	}
	if err := c.proj.LoadConfig(); err != nil {
		return err
	}
	if c.engine != nil {
		// Rebind the next request; in-flight tools retain the old resolved pins.
		c.engine.SetJobs(c.engineJobs())
	}
	return nil
}

// AgentModelWarnings lists agents.models pins whose name no longer resolves
// under the freshly loaded config and the connected providers, as "role=name"
// strings. Empty when every pin is live or agents.models is unset.
func (c *Controller) AgentModelWarnings() []string {
	return c.agentModels().Stale()
}

func (c *Controller) EffectiveModelName() string {
	if c == nil {
		return ""
	}
	if name := c.configuredModelName(); name != "" {
		return name
	}
	return noModelLabel
}

// ModelLabel is the display label for the model a turn runs on: the name,
// plus " · <effort>" when one is set on the wire. EffectiveModelName keeps
// returning the bare name, so callers that compare names are unaffected.
func (c *Controller) ModelLabel() string {
	if c == nil {
		return session.NoModelLabel
	}
	// session.ModelLabel is the single renderer for every model info line.
	return session.ModelLabel(c.configuredModelName(), string(c.configuredEffort()))
}

// ModelRef is the model a turn runs on packed as the shared "name:effort"
// reference — the form plan pins and the pickers round-trip. Unpinned-plan
// surfaces (the sidebar step badge) resolve through it, so a default model
// with an effort renders the same `name · effort` label as a pinned one.
// ModelName keeps the bare name for callers that compare names.
func (c *Controller) ModelRef() string {
	if c == nil {
		return ""
	}
	return session.FormatModelRef(c.configuredModelName(), string(c.configuredEffort()))
}

// configuredEffort is the reasoning effort a turn runs at: the live engine's,
// else the session default's. Empty means the provider default.
func (c *Controller) configuredEffort() llm.ReasoningEffort {
	if c == nil {
		return ""
	}
	if c.engine != nil {
		return c.engine.ModelConfig().ReasoningEffort
	}
	return c.appliedEffort(c.modelCfg)
}

// configuredModelName is the model a turn would run on right now: the live
// engine's model — a resumed session adopts its recorded model — else the
// session default. Empty means no model exists to send anything to.
func (c *Controller) configuredModelName() string {
	if c == nil {
		return ""
	}
	if c.engine != nil {
		if name := c.engine.ModelConfig().Name; name != "" {
			return name
		}
	}
	return c.modelCfg.Name
}

// ModelConfig returns the engine's active model configuration.
func (c *Controller) ModelConfig() llm.ModelConfig {
	if c == nil {
		return llm.ModelConfig{}
	}
	return c.modelCfg
}

// SidebarPreferences is the resolved global presentation state for the panel.
type SidebarPreferences struct {
	Width       int
	Visible     bool
	StopOnLimit bool
	PlanEnabled bool
	ExpandEdits bool
}

// SidebarPreferences loads the global panel width and default-on visibility.
func (c *Controller) SidebarPreferences() (SidebarPreferences, error) {
	if c == nil || c.proj == nil {
		return SidebarPreferences{}, errors.New("controller not initialized")
	}
	state, err := project.LoadUIState(c.proj.Global())
	if err != nil {
		return SidebarPreferences{}, err
	}
	return SidebarPreferences{
		Width:       state.SidebarWidth,
		Visible:     state.SidebarVisible(),
		StopOnLimit: state.StopLimitEnabled(),
		PlanEnabled: state.PlanEnabled(),
		ExpandEdits: state.ExpandEdits(),
	}, nil
}

// SaveSidebarWidth atomically persists the global preferred panel width.
func (c *Controller) SaveSidebarWidth(width int) error {
	if c == nil || c.proj == nil {
		return errors.New("controller not initialized")
	}
	return project.MutateUIState(c.proj.Global(), func(s *project.UIState) {
		s.SidebarWidth = width
	})
}

// SaveSidebarVisibility atomically persists the global panel visibility.
func (c *Controller) SaveSidebarVisibility(visible bool) error {
	if c == nil || c.proj == nil {
		return errors.New("controller not initialized")
	}
	return project.MutateUIState(c.proj.Global(), func(s *project.UIState) {
		s.SidebarHidden = !visible
	})
}

// SaveStopLimit atomically persists whether the tool-round stop is enabled.
func (c *Controller) SaveStopLimit(enabled bool) error {
	if c == nil || c.proj == nil {
		return errors.New("controller not initialized")
	}
	return project.MutateUIState(c.proj.Global(), func(s *project.UIState) {
		s.StopLimitDisabled = !enabled
	})
}

// SavePlanFeature atomically persists whether the plan feature is enabled.
func (c *Controller) SavePlanFeature(enabled bool) error {
	if c == nil || c.proj == nil {
		return errors.New("controller not initialized")
	}
	return project.MutateUIState(c.proj.Global(), func(s *project.UIState) {
		s.PlanDisabled = !enabled
	})
}

// SaveExpandEdits atomically persists whether edit cards render expanded.
func (c *Controller) SaveExpandEdits(enabled bool) error {
	if c == nil || c.proj == nil {
		return errors.New("controller not initialized")
	}
	return project.MutateUIState(c.proj.Global(), func(s *project.UIState) {
		s.ExpandEditsDisabled = !enabled
	})
}

// observeToolData folds a tool run event into the blocked-on-approval signal.
func (c *Controller) observeToolData(td session.ToolData) {
	if td.Run.Status == session.ToolRejected && td.Run.Error == plangate.ReasonPlanNotApproved {
		c.markPlanGateBlocked()
		return
	}
	// A terminal run of a gateable tool means the model moved past the plan
	// gate; exempt tools never clear the pending resume.
	if td.Run.Status >= session.ToolDone && !plangate.IsExempt(td.Run.Name) {
		c.clearPlanResumePending()
	}
}

// markPlanGateBlocked records a deny that the user can resolve by approving.
func (c *Controller) markPlanGateBlocked() {
	c.streamMu.Lock()
	c.planGateBlocked = true
	c.streamMu.Unlock()
}

// clearPlanResumePending records that the model made real progress, so a
// finished turn must not be resumed again on approval.
func (c *Controller) clearPlanResumePending() {
	c.streamMu.Lock()
	c.planGateBlocked = false
	c.planApprovalResumePending = false
	c.streamMu.Unlock()
}

func (c *Controller) publishPlan(plan session.Plan) {
	if c != nil {
		c.publish(PlanUpdatedMsg{Plan: plan.Clone()})
	}
}

// loadHooksManager discovers ~/.cozyphi/hooks and <cwd>/.cozyphi/hooks.
// Load errors are non-fatal (fail-open: no hooks). Child engines stay nil until spawn.
func loadHooksManager(proj *project.Project) *hooks.Manager {
	if proj == nil {
		return nil
	}
	mgr, warns, err := hooks.Load(proj.Global().HooksDir(), proj.HooksDir())
	if err != nil {
		debuglog.Logf("hooks: load failed: %v", err)
		return nil
	}
	hooks.LogWarnings(warns)
	return mgr
}

// ask publishes one ask message carrying reply and blocks for its single
// answer, dismissing the overlay on cancellation. It waits indefinitely: the
// answer arrives as an event on the reply channel, so the caller idles
// rather than polls.
func ask[T any](c *Controller, ctx context.Context, msg func(reply chan T) Msg, dismiss func() Msg) (T, error) {
	reply := make(chan T, 1)
	c.publish(msg(reply))
	select {
	case r := <-reply:
		c.streamMu.Lock()
		if ctx.Err() != nil {
			c.streamMu.Unlock()
			c.publish(dismiss())
			var zero T
			return zero, ctx.Err()
		}
		if a := c.assignment; a != nil && !a.Terminal {
			a.intervened = true
		}
		c.streamMu.Unlock()
		return r, nil
	case <-ctx.Done():
		c.publish(dismiss())
		var zero T
		return zero, ctx.Err()
	}
}

// askPermission blocks until the confirmation UI answers.
func (c *Controller) askPermission(
	ctx context.Context,
	req permission.Request,
	reason string,
) (permission.AskResult, error) {
	if c.allowAll.Load() {
		return permission.AskResult{Approved: true}, nil
	}
	r, err := ask(c, ctx,
		func(reply chan AskReply) Msg {
			return PermissionAskMsg{
				Request:     req,
				Reason:      reason,
				Reply:       reply,
				PersistPath: c.allowAllConfigPath(),
			}
		},
		func() Msg { return PermissionDismissMsg{} },
	)
	if err != nil {
		return permission.AskResult{}, err
	}
	if r.AllowSession || r.AllowPersistent {
		c.allowAll.Store(true)
	}
	if r.AllowPersistent && c.proj != nil {
		// The write's outcome is reported either way: a rule the user
		// believes exists but was never written is worse than the error.
		msg := PermissionPersistedMsg{Path: c.allowAllConfigPath()}
		if err := project.SetDangerouslyAllowAll(c.proj.Global(), true); err != nil {
			msg.ErrText = err.Error()
		}
		c.publish(msg)
	}
	return permission.AskResult{Approved: r.Approved, Feedback: r.Feedback}, nil
}

// allowAllConfigPath is the file the persistent allow-all rule lands in,
// empty when no project is loaded.
func (c *Controller) allowAllConfigPath() string {
	if c.proj == nil {
		return ""
	}
	return c.proj.Global().ConfigFile()
}

// askContinue blocks until the user chooses to continue or stop after max rounds.
func (c *Controller) askContinue(ctx context.Context, maxRounds int) (bool, error) {
	r, err := ask(c, ctx,
		func(reply chan ContinueReply) Msg {
			return ContinueAskMsg{MaxRounds: maxRounds, Reply: reply}
		},
		func() Msg { return ContinueDismissMsg{} },
	)
	if err != nil {
		return false, err
	}
	return r.Continue, nil
}

// askQuestion blocks until the interactive question overlay answers. It
// publishes a QuestionAskMsg and waits for the user's picks, mirroring
// askPermission's reply/timeout/dismiss select.
func (c *Controller) askQuestion(
	ctx context.Context,
	questions []questiontool.Question,
) ([]questiontool.Answer, error) {
	r, err := ask(c, ctx,
		func(reply chan QuestionReply) Msg {
			return QuestionAskMsg{Questions: questions, Reply: reply}
		},
		func() Msg { return QuestionDismissMsg{} },
	)
	if err != nil {
		return nil, err
	}
	return r.Answers, nil
}

// SetModel replaces the LLM client while keeping the same session tree.
func (c *Controller) SetModel(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("empty model name")
	}
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.closing || c.switchDone != nil {
		return errors.New("cannot change model while session is closing or switching")
	}
	if c.proj == nil {
		return errors.New("project not available")
	}
	cfg, err := c.resolveModelSelection(name)
	if err != nil {
		return err
	}
	selected := c.switchEffort(cfg)
	if effortSupported(cfg, cfg.ReasoningEffort) {
		cfg.ReasoningEffort = ""
	}
	return c.swapModel(cfg, selected)
}

// SetModelEffort commits the picker's selection as one choice: the model
// and its effort switch together or not at all. "default" (or an empty
// effort) clears to the provider-configured depth. Validation runs against
// the target model before anything mutates, so a rejected pick leaves the
// session untouched.
func (c *Controller) SetModelEffort(name, effort string) error {
	if c == nil {
		return errors.New("controller not initialized")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("empty model name")
	}
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.closing || c.switchDone != nil {
		return errors.New("cannot change model while session is closing or switching")
	}
	if c.proj == nil {
		return errors.New("project not available")
	}
	cfg, err := c.resolveModelSelection(name)
	if err != nil {
		return err
	}
	selected := strings.ToLower(strings.TrimSpace(effort))
	if selected == "default" {
		selected = ""
	}
	if selected != "" {
		if len(cfg.ReasoningEfforts) == 0 {
			return fmt.Errorf("model %q has no reasoning effort levels; pick a model that offers them", cfg.Name)
		}
		parsed, valid := llm.ParseReasoningEffort(selected)
		if !valid || !effortSupported(cfg, parsed) {
			return fmt.Errorf("model %q does not support reasoning effort %q", cfg.Name, effort)
		}
		selected = string(parsed)
	}
	if effortSupported(cfg, cfg.ReasoningEffort) {
		cfg.ReasoningEffort = ""
	}
	return c.swapModel(cfg, llm.ReasoningEffort(selected))
}

// switchEffort decides which effort selection survives a model switch: an
// effort the pick itself names — a legacy "name:effort" selector resolved by
// findModel — wins, and the previous selection is kept only when the new
// model supports it.
func (c *Controller) switchEffort(cfg llm.ModelConfig) llm.ReasoningEffort {
	if effortSupported(cfg, cfg.ReasoningEffort) {
		return cfg.ReasoningEffort
	}
	if effortSupported(cfg, c.modelEffort) {
		return c.modelEffort
	}
	return ""
}

// appliedEffort resolves the effort a model config runs at: the selection
// when the model accepts it, else the model's own configured depth.
func (c *Controller) appliedEffort(cfg llm.ModelConfig) llm.ReasoningEffort {
	if effortSupported(cfg, c.modelEffort) {
		return c.modelEffort
	}
	return cfg.ReasoningEffort
}

// runtimeModelFrom applies the selected effort to a base model config: the
// result is what an engine runs on, while the base stays what the pick and
// the persistence remember.
func (c *Controller) runtimeModelFrom(cfg llm.ModelConfig) llm.ModelConfig {
	applied := cfg
	applied.ReasoningEffort = c.appliedEffort(cfg)
	return applied
}

// runtimeModel is the active model as the engine should run it.
func (c *Controller) runtimeModel() llm.ModelConfig {
	return c.runtimeModelFrom(c.modelCfg)
}

// swapModel commits only model state. The engine retains tools, gate,
// origin-bound callbacks, hooks and jobs in one rebuild. Caller holds streamMu.
func (c *Controller) swapModel(cfg llm.ModelConfig, effort llm.ReasoningEffort) error {
	if c.engine == nil {
		return errors.New("agent not configured")
	}
	if err := c.engine.SelectModel(cfg, effort); err != nil {
		return err
	}
	c.modelCfg = cfg
	c.modelEffort = effort
	c.persistLastModel()
	return nil
}

// ContextView returns the /context browser snapshot for the current session:
// itemized entries plus window/threshold numbers. Read-only.
func (c *Controller) ContextView() agent.ContextView {
	if c == nil || c.engine == nil {
		return agent.ContextView{}
	}
	return c.engine.ContextReport()
}

// TrimContextFrom drops everything before the entry from the model's
// context (append-only). Refused while a reply or queued prompt runs.
func (c *Controller) TrimContextFrom(entryID string) error {
	if c == nil || c.engine == nil {
		return errors.New("controller: no engine")
	}
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.closing || c.streamRunning {
		return errors.New("cannot trim while a reply or queued prompt is running")
	}
	return c.engine.TrimContextFrom(entryID)
}

// DropContextEntries deletes the given entries from the model's context
// (append-only). Refused while a reply or queued prompt runs, like trims.
func (c *Controller) DropContextEntries(ids []string) error {
	if c == nil || c.engine == nil {
		return errors.New("controller: no engine")
	}
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.closing || c.streamRunning {
		return errors.New("cannot delete context blocks while a reply or queued prompt is running")
	}
	return c.engine.DropContextEntries(ids)
}

// SessionID returns the short-form-friendly session id.
func (c *Controller) SessionID() string {
	if c.engine == nil {
		return ""
	}
	return c.engine.SessionID()
}

// SessionDir returns the directory where session JSONL files are stored.
func (c *Controller) SessionDir() string {
	if c == nil {
		return ""
	}
	return c.sessionDir
}

// LiveJobCount returns in-flight sub-agent jobs (0 if jobs disabled).
func (c *Controller) LiveJobCount() int {
	if c == nil || c.jobs == nil {
		return 0
	}
	return c.jobs.LiveCountForOwner(c.jobOwnerID)
}

// WatchList returns a snapshot of every watch this session started, in
// start order, live and finished alike. Dumb widgets (footer, watch pane)
// read watches through this seam and never touch the manager — sub-agents
// and headless runs have no manager, so emptiness just hides the UI.
func (c *Controller) WatchList() []watch.Watch {
	if c == nil || c.watches == nil {
		return nil
	}
	return c.watches.List()
}

// WatchLog returns the last events of one watch, oldest first, for the
// watch pane's log view — the same data watch action=log serves the model.
func (c *Controller) WatchLog(id string, limit int) ([]watch.Event, error) {
	if c == nil || c.watches == nil {
		return nil, fmt.Errorf("no watch manager: cannot read log of %q", id)
	}
	return c.watches.Log(id, limit)
}

// StopWatch stops one live watch from the UI. The watch's Final event still
// reaches subscribers, so the transcript and the pane both update on redraw.
func (c *Controller) StopWatch(id string) error {
	if c == nil || c.watches == nil {
		return fmt.Errorf("no watch manager: cannot stop %q", id)
	}
	return c.watches.Stop(id)
}

// SessionFile returns the JSONL path when persisting.
func (c *Controller) SessionFile() string {
	if c.engine == nil {
		return ""
	}
	return c.engine.SessionFile()
}

// beginSessionSwitch reserves an idle engine without holding streamMu through
// hooks: hooks may synchronously submit prompts, wake watches, or request Close.
func (c *Controller) beginSessionSwitch(action string) error {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if err := c.requireRunIdleLocked(action); err != nil {
		return err
	}
	if c.assignment != nil && !c.assignment.Terminal {
		return fmt.Errorf("cannot %s: stop or finish the child assignment first", action)
	}
	c.switchDone = make(chan struct{})
	return nil
}

func (c *Controller) endSessionSwitch() {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	done := c.switchDone
	c.switchDone = nil
	stopped := c.streamStopped
	c.streamStopped = false
	c.startNextLocked(stopped)
	close(done)
}

// switchSession requires a reservation held through all caller-side updates.
// It acquires the replacement before shutting down the previous session. A busy
// or invalid target leaves the old engine and hooks untouched.
func (c *Controller) switchSession(
	reason string,
	opts agent.SessionOpts,
	hooksFor func() *hooks.Manager,
) (*agent.Engine, error) {
	prevID := c.SessionID()
	if out := c.sessionBeforeSwitch(reason, prevID, opts.ResumeID); out.Denied {
		c.publishSessionEffects(out)
		denied := out.Reason
		if denied == "" {
			denied = "session switch denied by hook"
		}
		return nil, errors.New(denied)
	}

	cfg := c.modelCfg
	if cfg.Name == "" {
		if c.proj == nil {
			return nil, errors.New("project not available")
		}
		if err := c.proj.LoadConfig(); err != nil {
			return nil, err
		}
		cfg = c.proj.Config().Model()
	}

	nextHooks := hooksFor()
	eng, err := c.newEngine(c.runtimeModelFrom(cfg), opts, nextHooks)
	if err != nil {
		return nil, err
	}
	c.sessionShutdown(reason, prevID)
	c.streamMu.Lock()
	previous := c.engine
	c.engine = eng
	c.hooksManager.Store(nextHooks)
	c.modelCfg = cfg
	c.streamMu.Unlock()
	if previous != nil {
		if err := previous.Session().Close(); err != nil {
			debuglog.Logf("session: close replaced owner: %v", err)
		}
	}
	id := eng.SessionID()
	c.progressSession.Store(&id)
	c.engineRef.Store(eng)
	c.resetUsage()
	c.publishPlan(eng.Plan())
	c.emitSessionStart(reason, eng.SessionID(), prevID)
	return eng, nil
}

// Resume loads a prior session by id (exact or unique prefix).
// On success the engine session is replaced; caller should refresh the UI transcript.
// If the resumed session cwd differs from the process cwd, cwdWarning is non-empty.
func (c *Controller) Resume(id string) (cwdWarning string, err error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("empty session id")
	}
	if c.sessionDir == "" {
		return "", errors.New("session directory not configured")
	}
	if err := c.beginSessionSwitch("resume a session"); err != nil {
		return "", err
	}
	defer c.endSessionSwitch()

	eng, err := c.switchSession("resume", agent.SessionOpts{
		Cwd:        c.cwd,
		SessionDir: c.sessionDir,
		Persist:    true,
		ResumeID:   id,
	}, func() *hooks.Manager {
		return loadHooksManager(c.proj)
	})
	if err != nil {
		return "", err
	}
	// The engine may have resolved the session's own model on resume. It is
	// the session's model, not a fresh choice, so it is not remembered; a
	// runtime level it carries reads as the session's selection, not as
	// configuration of the base.
	resumed := eng.ModelConfig()
	c.streamMu.Lock()
	if effortSupported(resumed, resumed.ReasoningEffort) {
		c.modelEffort = resumed.ReasoningEffort
		resumed.ReasoningEffort = ""
	}
	c.modelCfg = resumed
	c.streamMu.Unlock()
	if sessCwd := eng.SessionCwd(); sessCwd != "" && c.cwd != "" && sessCwd != c.cwd {
		cwdWarning = fmt.Sprintf("session cwd is %s (current %s); not changing directory", sessCwd, c.cwd)
	}
	return cwdWarning, nil
}

// Clear starts a brand-new persisted session (empty transcript, new id).
// Caller must ensure no agent stream / local bash is in flight.
func (c *Controller) Clear() error {
	if c.sessionDir == "" {
		return errors.New("session directory not configured")
	}
	if err := c.beginSessionSwitch("clear the session"); err != nil {
		return err
	}
	defer c.endSessionSwitch()

	_, err := c.switchSession("new", agent.SessionOpts{
		Cwd:        c.cwd,
		SessionDir: c.sessionDir,
		Persist:    true,
	}, c.Hooks)
	return err
}

// ReplaySnapshot builds a UI transcript snapshot from the engine session
// (user/assistant text; tool rows simplified away). The projection itself
// lives in internal/tui/transcript beside the Mapper.
func (c *Controller) ReplaySnapshot() session.Snapshot {
	if c.engine == nil || c.engine.Session() == nil {
		return session.Snapshot{}
	}
	return transcript.ReplaySnapshot(c.engine.Session().PathEntries())
}

// StartPrompt starts a new agent loop. When another run is already in flight
// the prompt queues instead of aborting it. userID is the transcript row id of
// the submitted message, so dequeue can promote it out of the queued state.
// A zero-model refusal is startPromptLocked's business: every start path
// funnels through it.
func (c *Controller) StartPrompt(text string, pendingSkills []string, userID string, media ...llm.Media) {
	if c == nil {
		return
	}
	pendingSkills = append([]string(nil), pendingSkills...)
	c.streamMu.Lock()
	// The user said something: whatever the watches have been doing, the
	// streak that throttles them starts over.
	c.wakeStreak = 0
	c.wakeSuppressed = false
	if c.closing {
		c.streamMu.Unlock()
		return
	}
	if c.childRole != "" && c.assignment != nil && c.assignment.Terminal && c.switchDone == nil {
		c.startFollowUpLocked(queuedPrompt{text: text, pendingSkills: pendingSkills, media: media, id: userID})
		c.streamMu.Unlock()
		return
	}
	a := c.assignment
	reserved := a != nil && !a.Terminal && (!a.claimed || c.childAttached != nil || a.Turn == TurnIdle)
	if a != nil && !a.Terminal {
		a.intervened = true
	}
	if c.switchDone != nil || c.streamRunning || reserved {
		c.promptQueue = append(
			c.promptQueue,
			queuedPrompt{text: text, pendingSkills: pendingSkills, media: media, id: userID},
		)
		c.streamMu.Unlock()
		return
	}
	c.startPromptLocked(text, pendingSkills, media)
	c.streamMu.Unlock()
}

// refuseNoModelSubmit answers a turn the session cannot run: no model is
// configured, so there is nothing to send and no connection to open. The
// error row says how to get a model, and RunEndedMsg resets the footer the
// submitter already spun into its waiting phase.
func (c *Controller) refuseNoModelSubmit() {
	text := c.ModelSetupNotice()
	c.publish(SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
		ID:    fmt.Sprintf("no-model-%d", time.Now().UnixNano()),
		State: session.StateError,
		Text:  text,
		Content: []session.ContentBlock{
			{Type: session.BlockText, Text: text},
		},
	}}})
	c.publish(RunEndedMsg{})
}

// queuedPrompt is a submit waiting for the in-flight run to finish.
type queuedPrompt struct {
	text          string
	pendingSkills []string
	media         []llm.Media
	id            string
}

// dropQueuedPromptsLocked clears the queue and tells the UI to un-queue each
// dropped row, so the transcript hint does not outlive the queue. The caller
// holds streamMu.
func (c *Controller) dropQueuedPromptsLocked() {
	for _, q := range c.promptQueue {
		if q.id != "" {
			c.publish(SessionEventMsg{Event: session.UserPromoted{ID: q.id}})
		}
	}
	c.promptQueue = nil
}

// RecallQueuedPrompt pops the most recently queued prompt back out of the
// queue and returns its text and transcript row id, newest first (Esc
// recall). Not-ok means nothing is left to recall — the queue is empty or
// the entry was just drained into the running turn, in which case the
// message was already delivered and Esc keeps its cancel meaning. The lock
// makes the pop atomic against drainQueuedForRun and finishRun.
func (c *Controller) RecallQueuedPrompt() (text, id string, ok bool) {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if len(c.promptQueue) == 0 {
		return "", "", false
	}
	last := c.promptQueue[len(c.promptQueue)-1]
	c.promptQueue = c.promptQueue[:len(c.promptQueue)-1]
	return last.text, last.id, true
}

// startPromptLocked launches a run; the caller holds streamMu and the stream
// is idle. A turn with no model anywhere is refused here — the one gate every
// start path funnels through (submit, queued submit, watch wake, plan-approval
// resume) — so none of them can connect with nothing to send to.
func (c *Controller) startPromptLocked(text string, pendingSkills []string, media []llm.Media) {
	if c.closing || c.switchDone != nil {
		return
	}
	if c.configuredModelName() == "" {
		c.refuseNoModelSubmit()
		return
	}
	// Whatever the watches queued rides along with this prompt — including
	// when the prompt is empty, which is what a wake turn is. The reminder
	// never reaches the transcript row: the submitter published that from the
	// text the user typed, and a resumed transcript strips reminders back out.
	if reminder := agent.WatchReminder(c.drainWatchLocked()); reminder != "" {
		if text == "" {
			text = reminder
		} else {
			text = reminder + "\n\n" + text
		}
	}
	if c.assignment != nil {
		if c.assignment.stop || c.assignment.Terminal {
			return
		}
		c.assignment.Turn = TurnRunning
	}
	c.streamRunning = true
	c.streamStopped = false
	c.planGateBlocked = false
	c.planApprovalResumePending = false
	c.streamGen++
	gen := c.streamGen
	ctx, cancel := context.WithCancel(context.Background())
	c.streamCancel = cancel
	engine := c.engine
	c.streamWG.Go(func() {
		defer cancel()
		c.runLoop(ctx, gen, engine, text, pendingSkills, media)
	})
}

// finishRun marks the stream idle, starts the next queued prompt, and — when
// the pipeline truly went quiet — tells the UI via RunEndedMsg so footer
// activity resets without anyone reconciling it from the snapshot.
func (c *Controller) finishRun(gen int) {
	c.streamMu.Lock()
	if gen != c.streamGen {
		c.streamMu.Unlock()
		return
	}
	// Esc means stop, and that includes stopping the watches from starting
	// the next turn. Their events stay queued and ride with whatever the user
	// sends next.
	stopped := c.streamStopped
	c.streamRunning = false
	c.streamStopped = false
	c.streamCancel = nil
	startedNext := c.startNextLocked(stopped)
	if a := c.assignment; a != nil && !startedNext && !a.Terminal {
		if stopped && !a.stop && !c.closing {
			a.Turn = TurnInterrupted
		} else {
			if c.closing {
				a.err = context.Canceled
			}
			c.completeAssignmentLocked()
		}
	}
	c.streamMu.Unlock()
	if !startedNext {
		c.publish(RunEndedMsg{})
	}
}

// startNextLocked restores admission after a turn or switch, including rollback.
// Queued user input takes priority over plan resumes and watch-only turns.
func (c *Controller) startNextLocked(stopped bool) bool {
	if c.closing || c.switchDone != nil {
		return false
	}
	startedNext := false
	assignmentStopped := c.assignment != nil && c.assignment.stop
	if !assignmentStopped && len(c.promptQueue) > 0 {
		next := c.promptQueue[0]
		c.promptQueue = c.promptQueue[1:]
		if c.childRole != "" && c.assignment != nil && c.assignment.Terminal {
			// A switch can queue input on a retained terminal child. Admit a
			// linked assignment instead of trying to revive the finished one.
			c.startFollowUpLocked(next)
		} else {
			c.startPromptLocked(next.text, next.pendingSkills, next.media)
		}
		if next.id != "" {
			c.publish(SessionEventMsg{Event: session.UserPromoted{ID: next.id}})
		}
		startedNext = true
	}
	if !startedNext && !assignmentStopped && (!stopped || c.assignment == nil) {
		c.maybeResumeApprovedWorkLocked()
		startedNext = c.streamRunning
	}
	if !startedNext && !assignmentStopped && !stopped && !c.wakeSuppressed &&
		(len(c.watchQueue) > 0 || c.reconcileOutcomeHintLocked()) &&
		c.wakeStreak < maxWakeStreak {
		// Events that arrived mid-turn but after the last tool round: the
		// turn had no boundary left to inject them at, so they get their own.
		c.wakeStreak++
		c.startPromptLocked("", nil, nil)
		startedNext = true
	}
	return startedNext
}

// RunActive reports whether a run or queued prompt is in flight. It is the
// single source of truth for gating user input (Submitter.CanSubmit) and
// flips on synchronously with StartPrompt, before the first stream event.
func (c *Controller) RunActive() bool {
	c.streamMu.Lock()
	active := c.switchDone != nil || c.streamRunning || len(c.promptQueue) > 0
	c.streamMu.Unlock()
	return active
}

func (c *Controller) requireRunIdleLocked(action string) error {
	if c.closing {
		return fmt.Errorf("cannot %s: session is closing", action)
	}
	if c.switchDone != nil {
		return fmt.Errorf("cannot %s: session is switching", action)
	}
	if c.streamRunning || len(c.promptQueue) > 0 {
		return fmt.Errorf("cannot %s while a reply or queued prompt is running", action)
	}
	return nil
}

// Cancel aborts only the current stream. Accepted prompts stay queued, and the
// pipeline remains busy until the current loop has actually exited; otherwise
// a fast submit after Esc could run two loops against one Engine concurrently.
func (c *Controller) Cancel() {
	c.streamMu.Lock()
	c.wakeSuppressed = true
	cancel := c.streamCancel
	if a := c.assignment; a != nil && !a.Terminal {
		a.intervened = true
	}
	if c.streamRunning || c.switchDone != nil {
		c.streamStopped = true
		if c.assignment != nil && !c.assignment.Terminal {
			c.assignment.Turn = TurnInterrupting
		}
	} else if a := c.assignment; a != nil && !a.Terminal && a.Turn == TurnIdle {
		// Admission and View assembly precede inference. Esc also interrupts
		// that reserved turn, so a late Ready cannot start it behind the user.
		a.Turn = TurnInterrupted
	}
	// Esc with nothing running still means something when watches are about
	// to start a turn: it calls that off. The events stay queued.
	if c.watchWake != nil {
		c.watchWake.Stop()
		c.watchWake = nil
	}
	c.streamMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *Controller) shutdownPrompts() {
	c.streamMu.Lock()
	c.closing = true
	if c.assignment != nil {
		c.stopAssignmentLocked(c.assignment, context.Canceled)
	}
	c.promptQueue = nil
	if c.watchWake != nil {
		c.watchWake.Stop()
		c.watchWake = nil
	}
	cancel := c.streamCancel
	if c.streamRunning {
		c.streamStopped = true
	}
	c.streamMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Compact summarizes the current session history on demand (/compact).
// It runs in the background; every outcome — including "nothing to
// compact" — reaches the UI as session events on the bus, exactly like
// stream errors. Callers must ensure the stream is idle first (the editor
// guards with Submitter.CanSubmit); Cancel aborts an in-flight run.
func (c *Controller) Compact() {
	c.streamMu.Lock()
	if c.closing || c.switchDone != nil || c.streamRunning {
		c.streamMu.Unlock()
		c.publishCompactError(errors.New("cannot compact while a reply or queued prompt is running"))
		return
	}
	engine := c.engine
	ctx, cancel := context.WithCancel(context.Background())
	c.streamRunning = true
	c.streamStopped = false
	c.streamGen++
	gen := c.streamGen
	c.streamCancel = cancel
	c.streamWG.Go(func() {
		defer cancel()
		defer c.finishRun(gen)
		if engine == nil {
			c.publishCompactError(errors.New("no session to compact yet"))
			return
		}
		if err := engine.CompactNow(ctx, func(ev session.Event) bool {
			if !c.Alive(gen) {
				return false
			}
			c.publish(SessionEventMsg{Event: ev})
			return true
		}); err != nil && ctx.Err() == nil && c.Alive(gen) {
			c.publishCompactError(err)
		}
	})
	c.streamMu.Unlock()
	// The pipeline announces compaction itself now; nothing reconciles
	// footer activity from the snapshot anymore.
	c.publish(SetActivityMsg{Activity: ActivityCompacting})
}

func (c *Controller) publishCompactError(err error) {
	// The /compact advice would be circular here, so an overflow falls
	// through to the bare headline.
	text := "Compact failed."
	if c := runerror.Classify(err); c.Cause != runerror.CauseContextOverflow {
		if headline := runerror.Hint(err, tuiRemedies); headline != "" {
			text += " " + headline
		}
	}
	text += "\n\n" + err.Error()
	c.publish(SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
		ID:    fmt.Sprintf("compact-error-%d", time.Now().UnixNano()),
		State: session.StateError,
		Text:  text,
		Content: []session.ContentBlock{
			{Type: session.BlockText, Text: text},
		},
	}}})
}

// Close cancels only this session and its owned assignments. The wait is bounded;
// a timed-out cleanup retains borrowed workspace resources until workers exit.
func (c *Controller) Close() {
	if c == nil {
		return
	}
	budget := c.closeBudget
	if budget <= 0 {
		budget = 3 * time.Second
	}
	c.stopSession()
	waitBudgeted(c.closeDone, budget, "the session to stop")
	if c.ownsRuntime {
		if err := c.runtime.Close(); err != nil {
			debuglog.Logf("controller: close owned runtime: %v", err)
		}
	}
}

func (c *Controller) stopSession() {
	c.closeOnce.Do(func() {
		if c.closeDone == nil {
			c.closeDone = make(chan struct{})
		}
		c.shutdownPrompts()
		// Admission is now closed; no lifetime can be added to this snapshot.
		c.streamMu.Lock()
		lifetimes := c.lifetimes
		c.streamMu.Unlock()
		for _, lifetime := range lifetimes {
			lifetime.cancel()
		}
		c.closeUsage()
		go c.cleanupSession()
	})
}

func (c *Controller) cleanupSession() {
	defer close(c.closeDone)
	jobsDone := make(chan struct{})
	go func() {
		defer close(jobsDone)
		if c.jobs == nil {
			return
		}
		var err error
		if c.runtime != nil {
			err = c.jobs.CloseOwner(context.Background(), c.jobOwnerID)
		} else {
			// Legacy zero-value test assemblies own their collaborators.
			err = c.jobs.Close()
		}
		if err != nil {
			debuglog.Logf("jobs: close session: %v", err)
		}
	}()
	// shutdownPrompts closed admission before cleanup started. A switch may
	// still be constructing or publishing its replacement in reentrant hooks;
	// neither owner nor borrowed services can be released until it has finished.
	c.streamMu.Lock()
	switchDone := c.switchDone
	c.streamMu.Unlock()
	if switchDone != nil {
		<-switchDone
	}
	c.streamWG.Wait()
	c.usageWork.workers.Wait()
	if c.unsubWatches != nil {
		c.unsubWatches()
	}
	if c.watches != nil {
		c.watches.Close()
	}
	if c.unsubJobs != nil {
		c.unsubJobs()
	}
	<-jobsDone
	for _, lifetime := range c.lifetimes {
		<-lifetime.done
	}
	c.sessionShutdown("quit", c.SessionID())
	if c.engine != nil {
		if err := c.engine.Session().Close(); err != nil {
			debuglog.Logf("session: close owner: %v", err)
		}
	}
	if c.runtime != nil {
		c.runtime.mu.Lock()
		delete(c.runtime.sessions, c)
		c.runtime.children = slices.DeleteFunc(c.runtime.children, func(child ChildSession) bool {
			return child.Controller == c
		})
		c.runtime.mu.Unlock()
		return
	}
	if c.mcpPool != nil {
		if err := c.mcpPool.Close(); err != nil {
			debuglog.Logf("mcp: close: %v", err)
		}
	}
	if c.lspMgr != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := c.lspMgr.Close(ctx); err != nil {
			debuglog.Logf("lsp: close: %v", err)
		}
		cancel()
	}
}

// waitBudgeted waits for done up to budget, logging what stalled when the
// budget expires. The stalled work is abandoned, not cancelled — at shutdown
// the process exit is the final reaper.
func waitBudgeted(done <-chan struct{}, budget time.Duration, what string) {
	select {
	case <-done:
	case <-time.After(budget):
		debuglog.Logf("tui: timed out waiting for %s", what)
	}
}

func (c *Controller) sessionBeforeSwitch(reason, fromID, targetID string) hooks.SessionOutcome {
	mgr := c.Hooks()
	if mgr == nil {
		return hooks.SessionOutcome{}
	}
	return mgr.SessionBeforeSwitch(context.Background(), hooks.SessionEvent{
		SessionID:       fromID,
		Cwd:             c.cwd,
		Reason:          reason,
		TargetSessionID: targetID,
		Usage:           c.sessionUsage(),
	})
}

func (c *Controller) sessionShutdown(reason, sessionID string) {
	mgr := c.Hooks()
	if mgr == nil {
		return
	}
	out := mgr.SessionShutdown(context.Background(), hooks.SessionEvent{
		SessionID: sessionID,
		Cwd:       c.cwd,
		Reason:    reason,
		Usage:     c.sessionUsage(),
	})
	c.publishSessionEffects(out)
}

func (c *Controller) emitSessionStart(reason, sessionID, previousID string) {
	mgr := c.Hooks()
	if mgr == nil {
		return
	}
	ctx := context.Background()
	if c.runtime != nil {
		ctx = c.runtime.constructionCtx
	}
	out := mgr.SessionStart(ctx, hooks.SessionEvent{
		SessionID:         sessionID,
		Cwd:               c.cwd,
		Reason:            reason,
		PreviousSessionID: previousID,
		Usage:             c.sessionUsage(),
	})
	c.publishSessionEffects(out)
}

// sessionUsage returns the token usage of the last completed turn observed by
// this controller's run loop; zero when no turn has completed (or the provider
// never reported usage). Usage comes from the stream, not the session store, so
// a resumed session reports zero until its first turn finishes.
func (c *Controller) sessionUsage() hooks.SessionUsage {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	return c.lastUsage
}

// recordUsage snapshots the completed turn's usage for session lifecycle hooks
// and fires post_turn audit hooks (cache metrics, etc.).
func (c *Controller) recordUsage(m session.Message) {
	usage := hooks.SessionUsage{
		PromptTokens:     m.Usage.PromptTokens,
		CompletionTokens: m.Usage.CompletionTokens,
		CachedTokens:     m.Usage.CachedTokens,
		TotalTokens:      m.Usage.TotalTokens,
	}
	c.streamMu.Lock()
	c.lastUsage = usage
	c.streamMu.Unlock()

	mgr := c.Hooks()
	if mgr == nil {
		return
	}
	mgr.PostTurn(context.Background(), hooks.SessionEvent{
		SessionID: c.SessionID(),
		Cwd:       c.cwd,
		MessageID: m.ID,
		Usage:     usage,
	})
}

// resetUsage clears captured usage when switching sessions so a new or resumed
// session does not inherit the previous one's counts.
func (c *Controller) resetUsage() {
	c.streamMu.Lock()
	c.lastUsage = hooks.SessionUsage{}
	c.streamMu.Unlock()
}

func (c *Controller) publishSessionEffects(out hooks.SessionOutcome) {
	if out.Toast == "" && !out.StatusSet {
		return
	}
	c.publish(HookSessionEffectsMsg{
		Toast:     out.Toast,
		Status:    out.Status,
		StatusSet: out.StatusSet,
	})
}

// Alive reports whether the stream generation still matches gen.
func (c *Controller) Alive(gen int) bool {
	c.streamMu.Lock()
	ok := c.streamGen == gen && !c.streamStopped
	c.streamMu.Unlock()
	return ok
}

func (c *Controller) waitOrDone(ctx context.Context, gen int, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
	}
	return c.Alive(gen)
}

func (c *Controller) publish(m Msg) {
	if c.bus != nil {
		c.bus.Publish(m)
	}
}

func (c *Controller) runLoop(
	ctx context.Context,
	gen int,
	engine *agent.Engine,
	prompt string,
	pendingSkills []string,
	media []llm.Media,
) {
	defer c.finishRun(gen)
	if !c.waitOrDone(ctx, gen, 120*time.Millisecond) {
		return
	}
	c.publish(SetActivityMsg{Activity: ActivityStreaming})

	if engine == nil {
		errText := "agent not configured"
		if !c.Alive(gen) {
			return
		}
		c.publish(SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
			ID:    fmt.Sprintf("assistant-error-%d", time.Now().UnixNano()),
			State: session.StateError,
			Text:  errText,
			Content: []session.ContentBlock{
				{Type: session.BlockText, Text: errText},
			},
		}}})
		return
	}

	// drainQueuedForRun hands the prompt queue to the running turn: the engine
	// polls it at every tool-round boundary and injects each prompt mid-turn,
	// so a follow-up submitted during a long agentic turn reaches the model on
	// the next round instead of after the whole turn ends. Draining is a no-op
	// once the turn is cancelled or superseded — finishRun keeps the fallback.
	// A prompt drained just before Esc lands in the model's context but gets no
	// follow-up run; the next submit carries it (the session persists it).
	drainQueuedForRun := func() []agent.InjectedPrompt {
		if !c.Alive(gen) {
			return nil
		}
		c.streamMu.Lock()
		if ctx.Err() != nil || gen != c.streamGen || c.streamStopped || c.closing {
			c.streamMu.Unlock()
			return nil
		}
		queued := c.promptQueue
		c.promptQueue = nil
		fired := c.drainWatchLocked()
		c.streamMu.Unlock()

		out := make([]agent.InjectedPrompt, 0, len(queued)+1)
		for _, q := range queued {
			out = append(out, agent.InjectedPrompt{Text: q.text, Skills: q.pendingSkills, Media: q.media, UserID: q.id})
		}
		// A watch event carries no transcript row id: it was never a queued
		// user message, so there is no "(queued)" hint to promote.
		if reminder := agent.WatchReminder(fired); reminder != "" {
			out = append(out, agent.InjectedPrompt{Text: reminder})
		}
		return out
	}

	opts := agent.LoopOpts{PendingSkills: pendingSkills, Media: media, Inject: drainQueuedForRun}
	if c.childRole == "" {
		opts.Inbox = func(parent *agent.Session) error { return c.deliverOutcomes(ctx, gen, parent) }
	}
	for ev, err := range engine.Loop(ctx, prompt, opts) {
		c.recordAssignmentEvent(gen, ev, err)
		if p, ok := ev.(session.UserPromoted); ok {
			// Row-scoped, not gen-scoped: publish even while the turn is being
			// cancelled — the engine appended the message before yielding, and
			// skipping here would leave the "(queued)" hint stuck forever.
			c.publish(SessionEventMsg{Event: p})
		}
		if !c.Alive(gen) {
			return
		}
		if err != nil {
			errText := runErrorText(err)
			c.publish(SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
				ID:    fmt.Sprintf("assistant-error-%d", time.Now().UnixNano()),
				State: session.StateError,
				Text:  errText,
				Content: []session.ContentBlock{
					{Type: session.BlockText, Text: errText},
				},
			}}})
			return
		}
		if ev != nil {
			c.publish(SessionEventMsg{Event: ev})
			if up, ok := ev.(session.AssistantMessageUpdate); ok && up.Message.State == session.StateComplete &&
				up.Message.Usage.Reported() {
				c.recordUsage(up.Message)
			}
			if td, ok := ev.(session.ToolData); ok {
				c.observeToolData(td)
			}
		}
	}
}

// workspaceRoot is the root the gate is built against.
func (c *Controller) workspaceRoot() string {
	if c.workspaceRootFn != nil {
		return c.workspaceRootFn()
	}
	if c.cwd != "" {
		return c.cwd
	}
	return permission.WorkspaceRoot()
}

// setGate publishes a freshly assembled boundary. Assembly happens on the UI
// goroutine while requests are judged on the run goroutine, so the pointer is
// swapped as a whole: a request is decided either by the old gate or by the
// new one, never by a torn read.
func (c *Controller) setGate(gate permission.Gate) {
	c.gate.Store(&gate)
}

// currentGate returns the installed boundary. A controller whose gate was
// never assembled denies rather than permits: the caller is asking to judge a
// request, and there is nothing to judge it with.
func (c *Controller) currentGate() permission.Gate {
	if c == nil {
		return permission.UnavailableGate{Reason: "no controller"}
	}
	if gate := c.gate.Load(); gate != nil && *gate != nil {
		return *gate
	}
	return permission.UnavailableGate{Reason: "the gate was never assembled"}
}

// GateFailure reports why the permission boundary could not be built from the
// configured policy, or "" when the gate is real. The session still runs: it
// denies every tool call, and the UI says so once instead of leaving the user
// to read the same refusal on every call.
func (c *Controller) GateFailure() string {
	if c == nil {
		return ""
	}
	return c.gateFailure
}
