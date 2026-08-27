package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/debuglog"
	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/memory"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/usage"
)

// Controller owns agent.Engine lifecycle and stream cancellation.
// It talks to the UI only by publishing Msg values onto the Bus.
//
// Construction: NewController(bus, proj, cwd, resumePath). Callers (cmd)
// assemble collaborators; Controller does not call project.GetDefaultProject.
type Controller struct {
	engine *agent.Engine
	proj   *project.Project

	streamMu      sync.Mutex
	streamCancel  context.CancelFunc
	streamGen     int
	streamRunning bool
	streamStopped bool
	promptQueue   []queuedPrompt
	streamWG      sync.WaitGroup
	closing       bool
	lastUsage     hooks.SessionUsage // usage of the last completed turn (streamMu)
	// planGateBlocked records a tool denied by the approval gate (streamMu).
	planGateBlocked bool
	// planApprovalResumePending records an approved active plan waiting for an
	// idle stream. Real tool progress clears it (streamMu).
	planApprovalResumePending bool

	bus *Bus

	sessionDir string
	cwd        string
	modelCfg   llm.ModelConfig
	providers  *provider.Manager
	jobs       *job.Manager
	unsubJobs  func()

	gate          permission.Gate
	askTimeoutSec int
	allowAll      atomic.Bool // session-wide allow-all for this process
	agentsEnabled atomic.Bool // when false, agent_* tools are not registered
	hooksManager  atomic.Pointer[hooks.Manager]
	mcpPool       *mcp.Pool
	mcpLoadFailed bool
	memory        *memory.Store
	lspMgr        *lsp.Manager

	// mode is the build/plan/useplan posture; plan overlays ModeReadonly on basePolicy.
	mode       agent.Mode
	basePolicy permission.Policy

	// lastJobProgress dedupes identical Progress publishes (key → signature).
	lastJobProgress sync.Map
}

// NewController wires bus + project into a ready Controller with a live Engine.
// proj must be non-nil (typically already LoadConfig'd by cmd). resumePath
// opens that session jsonl instead of starting a fresh one (cozyphi --continue /
// --resume); empty means a new session. On failure it returns (nil, err) —
// never a half-initialized Controller.
// NewController takes the usage history the same way the command registries do:
// optionally and variadically, so a test needs no history at all. Memory uses
// it to tell a fact that is waiting for its topic from one that is finished.
func NewController(
	bus *Bus,
	proj *project.Project,
	cwd, resumePath string,
	histories ...*usage.Store,
) (*Controller, error) {
	if bus == nil {
		return nil, errors.New("tui: nil bus")
	}
	if proj == nil {
		return nil, errors.New("tui: nil project")
	}
	if strings.TrimSpace(cwd) == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("tui: getwd: %w", err)
		}
	}

	if err := proj.LoadConfig(); err != nil {
		return nil, err
	}
	providers, err := provider.Open(provider.Options{
		CachePath:       proj.Global().ProviderCatalogFile(),
		CredentialsPath: proj.Global().CredentialsFile(),
	})
	if err != nil {
		return nil, fmt.Errorf("tui: initialize providers: %w", err)
	}

	c := &Controller{
		bus:           bus,
		proj:          proj,
		cwd:           cwd,
		sessionDir:    proj.SessionDir(),
		askTimeoutSec: 120,
		modelCfg:      proj.Config().Model(),
		providers:     providers,
		mode:          agent.ModeUsePlan,
	}
	config := proj.Config()

	// Before initGate: the gate carries the memory directory, which is the
	// one write target outside the workspace the agent is allowed.
	var history *usage.Store
	if len(histories) > 0 {
		history = histories[0]
	}
	if store, err := memory.Open(proj.MemoryDir(), usage.Memory{
		Store: history,
		Dir:   proj.MemoryDir(),
	}); err != nil {
		debuglog.Logf("memory: open: %v", err)
	} else {
		c.memory = store
	}

	c.basePolicy = config.Permissions
	c.initGate(config.Permissions)
	c.agentsEnabled.Store(config.Agents.Enabled)

	if mgr, err := lsp.Open(context.Background(), cwd, lsp.DefaultConfig()); err != nil {
		debuglog.Logf("lsp: open: %v", err)
	} else {
		c.lspMgr = mgr
	}

	hooksManager := loadHooksManager(proj)
	c.hooksManager.Store(hooksManager)

	jobs, err := agent.NewJobManager(proj.JobsDir(), c.modelCfg, func() llm.ModelConfig {
		return c.modelCfg
	}, c.Hooks, c.lspQuery())
	if err != nil {
		return nil, err
	}
	c.jobs = jobs

	if pool, err := mcp.LoadPool(proj.MCPConfigFile()); err != nil {
		debuglog.Logf("mcp: load: %v", err)
		c.mcpLoadFailed = true
	} else {
		c.mcpPool = pool
	}

	eng, err := agent.NewEngine(agent.EngineOpts{
		Model: c.modelCfg,
		SessionOpts: agent.SessionOpts{
			Cwd:        cwd,
			SessionDir: c.sessionDir,
			Persist:    true,
			ResumePath: resumePath,
		},
		Gate:         c.gate,
		Ask:          c.askPermission,
		ContinueAsk:  c.askContinue,
		Jobs:         c.engineJobs(),
		Hooks:        hooksManager,
		MCP:          c.mcpPool,
		Memory:       c.memory,
		LSP:          c.lspQuery(),
		QuestionAsk:  c.askQuestion,
		PlanUpdated:  c.publishPlan,
		ResolveModel: c.findModel,
	})
	if err != nil {
		return nil, err
	}
	c.engine = eng
	c.modelCfg = eng.ModelConfig()
	c.startJobProgress()
	c.emitSessionStart("startup", eng.SessionID(), "")
	return c, nil
}

func (c *Controller) startJobProgress() {
	if c.jobs == nil || c.bus == nil {
		return
	}
	ch, cancel := c.jobs.Subscribe()
	c.unsubJobs = cancel
	go func() {
		for p := range ch {
			if c.shouldPublishJobProgress(p) {
				c.publish(JobProgressMsg{Progress: p})
			}
		}
	}()
}

// shouldPublishJobProgress drops duplicate progress for the same child tool
// slot (same status/detail/name). Status transitions and new children still publish.
func (c *Controller) shouldPublishJobProgress(p job.Progress) bool {
	key := p.JobID + "\x00" + p.ToolUseID
	if p.ToolUseID == "" {
		key = p.JobID + "\x00" + p.Name + "\x00" + p.Detail
	}
	sig := p.Status + "\x00" + p.Name + "\x00" + p.Detail
	if prev, ok := c.lastJobProgress.Load(key); ok && prev.(string) == sig {
		return false
	}
	c.lastJobProgress.Store(key, sig)
	return true
}

func (c *Controller) initGate(policy permission.Policy) {
	if policy.AskTimeoutSec > 0 {
		c.askTimeoutSec = policy.AskTimeoutSec
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
	inner, err := permission.NewGate(policy, permission.WorkspaceRoot())
	if err != nil {
		inner, err = permission.NewGate(permission.DefaultPolicy(), permission.WorkspaceRoot())
		if err != nil {
			c.gate = &permission.BypassGate{Inner: permission.AllowAll{}, Enabled: &c.allowAll}
			return
		}
	}
	c.gate = &permission.BypassGate{Inner: inner, Enabled: &c.allowAll}
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
// read-only tool set. Takes effect from the next tool round.
func (c *Controller) SetMode(m agent.Mode) {
	if c == nil {
		return
	}
	switch m {
	case agent.ModeBuild, agent.ModePlan, agent.ModeUsePlan:
	default:
		m = agent.ModeUsePlan
	}
	c.mode = m
	c.initGate(c.basePolicy)
	if c.engine != nil {
		c.engine.SetPermission(c.gate, c.askPermission)
		c.engine.SetMode(c.mode)
	}
}

// ToggleMode cycles build → plan → useplan → build and returns the new mode.
// An empty/unknown mode is treated as the useplan default, so the first
// toggle from a zero-value controller lands on build.
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
	if c.mcpLoadFailed {
		return []mcp.ServerStatus{{Name: "configuration", State: mcp.StateFailed}}
	}
	return c.mcpPool.ServerStatuses()
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

// maybeResumeApprovedWorkLocked resumes approved work once the stream is idle.
// A real gate denial preserves the legacy resume path; direct approval resumes
// only while the current plan still has active work. The caller holds streamMu.
func (c *Controller) maybeResumeApprovedWorkLocked() {
	if c.streamRunning || c.engine == nil {
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
		if item.Status == session.PlanInProgress {
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

// ConnectProvider stores a credential for the exact endpoint approved by the user.
func (c *Controller) ConnectProvider(req provider.ConnectRequest) error {
	if c == nil || c.providers == nil {
		return errors.New("provider manager not available")
	}
	return c.providers.Connect(req)
}

// ModelNames returns configured and connected catalog models without duplicates.
func (c *Controller) ModelNames() []string {
	if c == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var names []string
	if c.proj != nil && c.proj.Config() != nil {
		for _, cfg := range c.proj.Config().AllModels() {
			if _, ok := seen[cfg.Name]; ok || cfg.Name == "" {
				continue
			}
			seen[cfg.Name] = struct{}{}
			names = append(names, cfg.Name)
		}
	}
	if c.providers != nil {
		for _, cfg := range c.providers.Models() {
			if _, ok := seen[cfg.Name]; ok || cfg.Name == "" {
				continue
			}
			seen[cfg.Name] = struct{}{}
			names = append(names, cfg.Name)
		}
	}
	return names
}

func (c *Controller) findModel(name string) (llm.ModelConfig, bool) {
	if c.proj != nil && c.proj.Config() != nil {
		if cfg, ok := c.proj.Config().FindModel(name); ok {
			return cfg, true
		}
	}
	if c.providers != nil {
		for _, cfg := range c.providers.Models() {
			if cfg.Name == name {
				if cfg.SkillPath == "" && c.proj != nil && c.proj.Config() != nil {
					cfg.SkillPath = c.proj.Config().SkillPath
				}
				return cfg, true
			}
		}
	}
	return llm.ModelConfig{}, false
}

// ModelName returns the active model label.
func (c *Controller) ModelName() string {
	if c == nil {
		return ""
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
	}, nil
}

// SaveSidebarWidth atomically persists the global preferred panel width.
func (c *Controller) SaveSidebarWidth(width int) error {
	if c == nil || c.proj == nil {
		return errors.New("controller not initialized")
	}
	state, err := project.LoadUIState(c.proj.Global())
	if err != nil {
		return err
	}
	state.SidebarWidth = width
	return project.SaveUIState(c.proj.Global(), state)
}

// SaveSidebarVisibility atomically persists the global panel visibility.
func (c *Controller) SaveSidebarVisibility(visible bool) error {
	if c == nil || c.proj == nil {
		return errors.New("controller not initialized")
	}
	state, err := project.LoadUIState(c.proj.Global())
	if err != nil {
		return err
	}
	state.SidebarHidden = !visible
	return project.SaveUIState(c.proj.Global(), state)
}

// SaveStopLimit atomically persists whether the tool-round stop is enabled.
func (c *Controller) SaveStopLimit(enabled bool) error {
	if c == nil || c.proj == nil {
		return errors.New("controller not initialized")
	}
	state, err := project.LoadUIState(c.proj.Global())
	if err != nil {
		return err
	}
	state.StopLimitDisabled = !enabled
	return project.SaveUIState(c.proj.Global(), state)
}

// observeToolData folds a tool run event into the blocked-on-approval signal.
func (c *Controller) observeToolData(td session.ToolData) {
	if td.Run.Status == session.ToolRejected && td.Run.Error == plangate.ReasonPlanNotApproved {
		c.markPlanGateBlocked()
		return
	}
	// A terminal run of a gateable tool means the model moved past the plan
	// gate; exempt tools (plan/context) never clear the pending resume.
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

// askPermission blocks until the confirmation UI answers.
func (c *Controller) askPermission(
	ctx context.Context,
	req permission.Request,
	reason string,
) (permission.AskResult, error) {
	if c.allowAll.Load() {
		return permission.AskResult{Approved: true}, nil
	}
	reply := make(chan AskReply, 1)
	c.publish(PermissionAskMsg{Request: req, Reason: reason, Reply: reply})

	timeout := time.Duration(c.askTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case r := <-reply:
		if r.AllowSession || r.AllowPersistent {
			c.allowAll.Store(true)
		}
		if r.AllowPersistent {
			if c.proj != nil {
				_ = project.SetDangerouslyAllowAll(c.proj.Global(), true)
			}
		}
		return permission.AskResult{Approved: r.Approved, Feedback: r.Feedback}, nil
	case <-ctx.Done():
		c.publish(PermissionDismissMsg{})
		return permission.AskResult{}, ctx.Err()
	case <-timer.C:
		c.publish(PermissionDismissMsg{})
		return permission.AskResult{}, nil
	}
}

// askContinue blocks until the user chooses to continue or stop after max rounds.
func (c *Controller) askContinue(ctx context.Context, maxRounds int) (bool, error) {
	reply := make(chan ContinueReply, 1)
	c.publish(ContinueAskMsg{MaxRounds: maxRounds, Reply: reply})

	timeout := time.Duration(c.askTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case r := <-reply:
		return r.Continue, nil
	case <-ctx.Done():
		c.publish(ContinueDismissMsg{})
		return false, ctx.Err()
	case <-timer.C:
		c.publish(ContinueDismissMsg{})
		return false, nil
	}
}

// askQuestion blocks until the interactive question overlay answers. It
// publishes a QuestionAskMsg and waits for the user's picks, mirroring
// askPermission's reply/timeout/dismiss select.
func (c *Controller) askQuestion(
	ctx context.Context,
	questions []questiontool.Question,
) ([]questiontool.Answer, error) {
	reply := make(chan QuestionReply, 1)
	c.publish(QuestionAskMsg{Questions: questions, Reply: reply})

	timeout := time.Duration(c.askTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case r := <-reply:
		return r.Answers, nil
	case <-ctx.Done():
		c.publish(QuestionDismissMsg{})
		return nil, ctx.Err()
	case <-timer.C:
		c.publish(QuestionDismissMsg{})
		return nil, nil
	}
}

// SetModel replaces the LLM client while keeping the same session tree.
func (c *Controller) SetModel(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("empty model name")
	}
	if err := c.requireRunIdle("change model"); err != nil {
		return err
	}
	if c.proj == nil {
		return errors.New("project not available")
	}
	if err := c.proj.LoadConfig(); err != nil {
		return err
	}
	cfg, ok := c.findModel(name)
	if !ok {
		// Not a configured model: keep the primary's connection settings and
		// only swap the name (arbitrary-model workflow).
		cfg = c.proj.Config().Model()
		cfg.Name = name
	}
	c.basePolicy = c.proj.Config().Permissions
	c.initGate(c.basePolicy)
	if c.engine == nil {
		return errors.New("agent not configured")
	}
	c.engine.SetPermission(c.gate, c.askPermission)
	c.engine.SetContinueAsk(c.askContinue)
	c.engine.SetJobs(c.engineJobs())
	if _, _, err := c.ReloadHooks(); err != nil {
		debuglog.Logf("hooks: reload on SetModel: %v", err)
	}
	if err := c.engine.SetModel(cfg); err != nil {
		return err
	}
	c.modelCfg = cfg
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
	return c.jobs.LiveCount()
}

// SessionFile returns the JSONL path when persisting.
func (c *Controller) SessionFile() string {
	if c.engine == nil {
		return ""
	}
	return c.engine.SessionFile()
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
	if err := c.requireRunIdle("resume a session"); err != nil {
		return "", err
	}

	prevID := c.SessionID()
	if out := c.sessionBeforeSwitch("resume", prevID, id); out.Denied {
		c.publishSessionEffects(out)
		reason := out.Reason
		if reason == "" {
			reason = "session switch denied by hook"
		}
		return "", errors.New(reason)
	}

	c.sessionShutdown("resume", prevID)

	cfg := c.modelCfg
	if cfg.Name == "" {
		if c.proj == nil {
			return "", errors.New("project not available")
		}
		if err := c.proj.LoadConfig(); err != nil {
			return "", err
		}
		cfg = c.proj.Config().Model()
	}

	mgr := loadHooksManager(c.proj)
	c.hooksManager.Store(mgr)
	eng, err := agent.NewEngine(agent.EngineOpts{
		Model: cfg,
		SessionOpts: agent.SessionOpts{
			Cwd:        c.cwd,
			SessionDir: c.sessionDir,
			Persist:    true,
			ResumeID:   id,
		},
		Gate:         c.gate,
		Ask:          c.askPermission,
		ContinueAsk:  c.askContinue,
		Jobs:         c.engineJobs(),
		Hooks:        mgr,
		MCP:          c.mcpPool,
		Memory:       c.memory,
		LSP:          c.lspQuery(),
		QuestionAsk:  c.askQuestion,
		PlanUpdated:  c.publishPlan,
		ResolveModel: c.findModel,
	})
	if err != nil {
		return "", err
	}
	if sessCwd := eng.SessionCwd(); sessCwd != "" && c.cwd != "" && sessCwd != c.cwd {
		cwdWarning = fmt.Sprintf("session cwd is %s (current %s); not changing directory", sessCwd, c.cwd)
	}
	c.engine = eng
	c.modelCfg = eng.ModelConfig()
	c.resetUsage()
	c.publishPlan(eng.Plan())
	c.emitSessionStart("resume", eng.SessionID(), prevID)
	return cwdWarning, nil
}

// Clear starts a brand-new persisted session (empty transcript, new id).
// Caller must ensure no agent stream / local bash is in flight.
func (c *Controller) Clear() error {
	if c.sessionDir == "" {
		return errors.New("session directory not configured")
	}
	if err := c.requireRunIdle("clear the session"); err != nil {
		return err
	}

	prevID := c.SessionID()
	if out := c.sessionBeforeSwitch("new", prevID, ""); out.Denied {
		c.publishSessionEffects(out)
		reason := out.Reason
		if reason == "" {
			reason = "session switch denied by hook"
		}
		return errors.New(reason)
	}
	c.sessionShutdown("new", prevID)

	cfg := c.modelCfg
	if cfg.Name == "" {
		if c.proj == nil {
			return errors.New("project not available")
		}
		if err := c.proj.LoadConfig(); err != nil {
			return err
		}
		cfg = c.proj.Config().Model()
	}

	hooksMgr := c.Hooks()
	engine, err := agent.NewEngine(agent.EngineOpts{
		Model: cfg,
		SessionOpts: agent.SessionOpts{
			Cwd:        c.cwd,
			SessionDir: c.sessionDir,
			Persist:    true,
		},
		Gate:        c.gate,
		Ask:         c.askPermission,
		ContinueAsk: c.askContinue,
		Jobs:        c.engineJobs(),
		Hooks:       hooksMgr,
		MCP:         c.mcpPool,
		Memory:      c.memory,
		LSP:         c.lspQuery(),
		QuestionAsk: c.askQuestion,
		PlanUpdated: c.publishPlan,
	})
	if err != nil {
		return err
	}
	c.engine = engine
	c.modelCfg = cfg
	c.resetUsage()
	c.publishPlan(engine.Plan())
	c.emitSessionStart("new", engine.SessionID(), prevID)
	return nil
}

// ReplaySnapshot builds a UI transcript snapshot from the engine session
// (user/assistant text; tool rows simplified away).
func (c *Controller) ReplaySnapshot() session.Snapshot {
	var snap session.Snapshot
	if c.engine == nil || c.engine.Session() == nil {
		return snap
	}
	var pendingCompaction *session.CompactionEntry
	emitCompaction := func() {
		if pendingCompaction == nil {
			return
		}
		snap = session.Apply(snap, session.CompactionComplete{
			ID:         pendingCompaction.ID,
			Compaction: pendingCompaction.Compaction,
		})
		pendingCompaction = nil
	}
	for _, entry := range c.engine.Session().PathEntries() {
		switch entry.GetType() {
		case session.EntryCompaction:
			compacted := entry.(session.CompactionEntry)
			pendingCompaction = &compacted
		case session.EntryMessage:
			messageEntry := entry.(session.SessionMessageEntry)
			if pendingCompaction != nil && session.MessageFollowsCompaction(*pendingCompaction, messageEntry) {
				emitCompaction()
			}
			msg := messageEntry.Message
			switch msg.Role {
			case llm.RoleUser:
				// Recall blocks are prepended by the turn, not typed by the
				// user; a replayed transcript shows the prompt as it was sent.
				snap = session.Apply(snap, session.UserAppend{
					ID:   entry.GetID(),
					Text: memory.StripReminders(msg.Content),
				})
			case llm.RoleAssistant:
				text := msg.Content
				var blocks []session.ContentBlock
				if strings.TrimSpace(msg.ReasoningContent) != "" {
					blocks = append(
						blocks,
						session.ContentBlock{Type: session.BlockThinking, Text: msg.ReasoningContent},
					)
				}
				if text != "" {
					blocks = append(blocks, session.ContentBlock{Type: session.BlockText, Text: text})
				}
				snap = session.Apply(snap, session.AssistantMessageUpdate{Message: session.Message{
					ID:      entry.GetID(),
					State:   session.StateComplete,
					Text:    text,
					Content: blocks,
					Usage: session.TokenUsage{
						PromptTokens:     msg.Usage.PromptTokens,
						CompletionTokens: msg.Usage.CompletionTokens,
						CachedTokens:     msg.Usage.CachedTokens(),
						TotalTokens:      msg.Usage.TotalTokens,
					},
				}})
			}
		}
	}
	emitCompaction()
	return snap
}

// StartPrompt starts a new agent loop. When another run is already in flight
// the prompt queues instead of aborting it. userID is the transcript row id of
// the submitted message, so dequeue can promote it out of the queued state.
func (c *Controller) StartPrompt(text string, pendingSkills []string, userID string, media ...llm.Media) {
	pendingSkills = append([]string(nil), pendingSkills...)
	c.streamMu.Lock()
	if c.closing {
		c.streamMu.Unlock()
		return
	}
	if c.streamRunning {
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

// startPromptLocked launches a run; the caller holds streamMu and the stream
// is idle.
func (c *Controller) startPromptLocked(text string, pendingSkills []string, media []llm.Media) {
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
	c.streamRunning = false
	c.streamStopped = false
	c.streamCancel = nil
	startedNext := false
	if !c.closing && len(c.promptQueue) > 0 {
		next := c.promptQueue[0]
		c.promptQueue = c.promptQueue[1:]
		c.startPromptLocked(next.text, next.pendingSkills, next.media)
		if next.id != "" {
			c.publish(SessionEventMsg{Event: session.UserPromoted{ID: next.id}})
		}
		startedNext = true
	}
	if !startedNext && !c.closing {
		c.maybeResumeApprovedWorkLocked()
		startedNext = c.streamRunning
	}
	c.streamMu.Unlock()
	if !startedNext {
		c.publish(RunEndedMsg{})
	}
}

// RunActive reports whether a run or queued prompt is in flight. It is the
// single source of truth for gating user input (Submitter.CanSubmit) and
// flips on synchronously with StartPrompt, before the first stream event.
func (c *Controller) RunActive() bool {
	c.streamMu.Lock()
	active := c.streamRunning || len(c.promptQueue) > 0
	c.streamMu.Unlock()
	return active
}

func (c *Controller) requireRunIdle(action string) error {
	c.streamMu.Lock()
	active := c.streamRunning || len(c.promptQueue) > 0
	c.streamMu.Unlock()
	if active {
		return fmt.Errorf("cannot %s while a reply or queued prompt is running", action)
	}
	return nil
}

// Cancel aborts only the current stream. Accepted prompts stay queued, and the
// pipeline remains busy until the current loop has actually exited; otherwise
// a fast submit after Esc could run two loops against one Engine concurrently.
func (c *Controller) Cancel() {
	c.streamMu.Lock()
	cancel := c.streamCancel
	if c.streamRunning {
		c.streamStopped = true
	}
	c.streamMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *Controller) shutdownPrompts() {
	c.streamMu.Lock()
	c.closing = true
	c.promptQueue = nil
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
	if c.closing || c.streamRunning {
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
	text := "Compact: " + err.Error()
	c.publish(SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
		ID:    fmt.Sprintf("compact-error-%d", time.Now().UnixNano()),
		State: session.StateError,
		Text:  text,
		Content: []session.ContentBlock{
			{Type: session.BlockText, Text: text},
		},
	}}})
}

// Close cancels the stream and shuts down the job manager.
func (c *Controller) Close() {
	c.sessionShutdown("quit", c.SessionID())
	c.shutdownPrompts()
	done := make(chan struct{})
	go func() {
		c.streamWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		debuglog.Logf("tui: timed out waiting for the active model run to stop")
	}
	if c.unsubJobs != nil {
		c.unsubJobs()
		c.unsubJobs = nil
	}
	if c.jobs != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = c.jobs.Close(ctx)
		cancel()
	}
	if c.mcpPool != nil {
		_ = c.mcpPool.Close()
		c.mcpPool = nil
	}
	if c.lspMgr != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = c.lspMgr.Close(ctx)
		cancel()
		c.lspMgr = nil
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
	out := mgr.SessionStart(context.Background(), hooks.SessionEvent{
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
		queued := c.promptQueue
		c.promptQueue = nil
		c.streamMu.Unlock()
		if len(queued) == 0 {
			return nil
		}
		out := make([]agent.InjectedPrompt, 0, len(queued))
		for _, q := range queued {
			out = append(out, agent.InjectedPrompt{Text: q.text, Skills: q.pendingSkills, Media: q.media, UserID: q.id})
		}
		return out
	}

	for ev, err := range engine.Loop(ctx, prompt, agent.LoopOpts{PendingSkills: pendingSkills, Media: media, Inject: drainQueuedForRun}) {
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
			errText := err.Error()
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
