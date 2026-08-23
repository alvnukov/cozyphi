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

	"github.com/pulseaiclub/phi/internal/agent"
	"github.com/pulseaiclub/phi/internal/debuglog"
	"github.com/pulseaiclub/phi/internal/hooks"
	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/mcp"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/project"
	"github.com/pulseaiclub/phi/internal/session"
)

// Controller owns agent.Engine lifecycle and stream cancellation.
// It talks to the UI only by publishing Msg values onto the Bus.
//
// Construction: NewController(bus, proj, cwd, resumePath). Callers (cmd)
// assemble collaborators; Controller does not call project.GetDefaultProject.
type Controller struct {
	engine *agent.Engine
	proj   *project.Project

	streamMu     sync.Mutex
	streamCancel context.CancelFunc
	streamGen    int
	lastUsage    hooks.SessionUsage // usage of the last completed turn (streamMu)

	bus *Bus

	sessionDir string
	cwd        string
	modelCfg   llm.ModelConfig
	jobs       *job.Manager
	unsubJobs  func()

	gate          permission.Gate
	askTimeoutSec int
	allowAll      atomic.Bool // session-wide allow-all for this process
	agentsEnabled atomic.Bool // when false, agent_* tools are not registered
	hooksManager  atomic.Pointer[hooks.Manager]
	mcpPool       *mcp.Pool

	// mode is the build/plan posture; plan overlays ModeReadonly on basePolicy.
	mode       agent.Mode
	basePolicy permission.Policy

	// lastJobProgress dedupes identical Progress publishes (key → signature).
	lastJobProgress sync.Map
}

// NewController wires bus + project into a ready Controller with a live Engine.
// proj must be non-nil (typically already LoadConfig'd by cmd). resumePath
// opens that session jsonl instead of starting a fresh one (phi --continue /
// --resume); empty means a new session. On failure it returns (nil, err) —
// never a half-initialized Controller.
func NewController(bus *Bus, proj *project.Project, cwd, resumePath string) (*Controller, error) {
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

	c := &Controller{
		bus:           bus,
		proj:          proj,
		cwd:           cwd,
		sessionDir:    proj.SessionDir(),
		askTimeoutSec: 120,
		modelCfg:      proj.Config().Model(),
	}
	config := proj.Config()

	c.basePolicy = config.Permissions
	c.initGate(config.Permissions)
	c.agentsEnabled.Store(config.Agents.Enabled)

	hooksManager := loadHooksManager(proj)
	c.hooksManager.Store(hooksManager)

	jobs, err := agent.NewJobManager(proj.JobsDir(), c.modelCfg, func() llm.ModelConfig {
		return c.modelCfg
	}, c.Hooks)
	if err != nil {
		return nil, err
	}
	c.jobs = jobs

	if pool, err := mcp.LoadPool(proj.MCPConfigFile()); err != nil {
		debuglog.Logf("mcp: load: %v", err)
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
		Gate:        c.gate,
		Ask:         c.askPermission,
		ContinueAsk: c.askContinue,
		Jobs:        c.engineJobs(),
		Hooks:       hooksManager,
		MCP:         c.mcpPool,
	})
	if err != nil {
		return nil, err
	}
	c.engine = eng
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

// Mode returns the current build/plan posture (build by default).
func (c *Controller) Mode() agent.Mode {
	if c == nil {
		return agent.ModeBuild
	}
	return c.mode
}

// SetMode switches the build/plan posture: the gate is rebuilt with the
// readonly overlay for plan, and the engine swaps its system prompt and
// read-only tool set. Takes effect from the next tool round.
func (c *Controller) SetMode(m agent.Mode) {
	if c == nil {
		return
	}
	if m != agent.ModePlan {
		m = agent.ModeBuild
	}
	c.mode = m
	c.initGate(c.basePolicy)
	if c.engine != nil {
		c.engine.SetPermission(c.gate, c.askPermission)
		c.engine.SetMode(c.mode)
	}
}

// ToggleMode flips build ↔ plan and returns the new mode for the UI label.
func (c *Controller) ToggleMode() agent.Mode {
	if c == nil {
		return agent.ModeBuild
	}
	if c.mode == agent.ModePlan {
		c.SetMode(agent.ModeBuild)
	} else {
		c.SetMode(agent.ModePlan)
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

// engineJobs returns the job manager only when sub-agents are enabled.
func (c *Controller) engineJobs() *job.Manager {
	if c == nil || !c.agentsEnabled.Load() {
		return nil
	}
	return c.jobs
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

// loadHooksManager discovers ~/.phi/hooks and <cwd>/.phi/hooks.
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

// SetModel replaces the LLM client while keeping the same session tree.
func (c *Controller) SetModel(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("empty model name")
	}
	if c.proj == nil {
		return errors.New("project not available")
	}
	if err := c.proj.LoadConfig(); err != nil {
		return err
	}
	cfg, ok := c.proj.Config().FindModel(name)
	if !ok {
		// Not a configured model: keep the primary's connection settings and
		// only swap the name (arbitrary-model workflow).
		cfg = c.proj.Config().Model()
		cfg.Name = name
	}
	c.Cancel()
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

	prevID := c.SessionID()
	if out := c.sessionBeforeSwitch("resume", prevID, id); out.Denied {
		c.publishSessionEffects(out)
		reason := out.Reason
		if reason == "" {
			reason = "session switch denied by hook"
		}
		return "", errors.New(reason)
	}

	c.Cancel()
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
		Gate:        c.gate,
		Ask:         c.askPermission,
		ContinueAsk: c.askContinue,
		Jobs:        c.engineJobs(),
		Hooks:       mgr,
		MCP:         c.mcpPool,
	})
	if err != nil {
		return "", err
	}
	if sessCwd := eng.SessionCwd(); sessCwd != "" && c.cwd != "" && sessCwd != c.cwd {
		cwdWarning = fmt.Sprintf("session cwd is %s (current %s); not changing directory", sessCwd, c.cwd)
	}
	c.engine = eng
	c.modelCfg = cfg
	c.resetUsage()
	c.emitSessionStart("resume", eng.SessionID(), prevID)
	return cwdWarning, nil
}

// Clear starts a brand-new persisted session (empty transcript, new id).
// Caller must ensure no agent stream / local bash is in flight.
func (c *Controller) Clear() error {
	if c.sessionDir == "" {
		return errors.New("session directory not configured")
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
	})
	if err != nil {
		return err
	}
	c.engine = engine
	c.modelCfg = cfg
	c.resetUsage()
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
	for _, entry := range c.engine.Session().PathEntries() {
		switch entry.GetType() {
		case session.EntryCompaction:
			snap = session.Apply(snap, session.CompactionComplete{ID: entry.GetID()})
		case session.EntryMessage:
			msg := entry.(session.SessionMessageEntry).Message
			switch msg.Role {
			case llm.RoleUser:
				snap = session.Apply(snap, session.UserAppend{ID: entry.GetID(), Text: msg.Content})
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
				}})
			}
		}
	}
	return snap
}

// StartPrompt cancels any in-flight stream and starts a new agent loop.
func (c *Controller) StartPrompt(text string, pendingSkills []string) {
	ctx, cancel := context.WithCancel(context.Background())
	c.streamMu.Lock()
	if c.streamCancel != nil {
		c.streamCancel()
	}
	c.streamCancel = cancel
	c.streamGen++
	gen := c.streamGen
	c.streamMu.Unlock()

	go c.runLoop(ctx, gen, text, pendingSkills)
}

// Cancel aborts the current stream context (if any).
func (c *Controller) Cancel() {
	c.streamMu.Lock()
	cancel := c.streamCancel
	c.streamMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Compact summarizes the current session history on demand (/compact).
// It runs in the background; every outcome — including "nothing to
// compact" — reaches the UI as session events on the bus, exactly like
// stream errors. Callers must ensure the stream is idle first (the editor
// guards with Submitter.StreamActive); Cancel aborts an in-flight run.
func (c *Controller) Compact() {
	ctx, cancel := context.WithCancel(context.Background())
	c.streamMu.Lock()
	engine := c.engine
	c.streamCancel = cancel
	c.streamMu.Unlock()

	go func() {
		defer cancel()
		if engine == nil {
			c.publishCompactError(errors.New("no session to compact yet"))
			return
		}
		if err := engine.CompactNow(ctx, func(ev session.Event) bool {
			c.publish(SessionEventMsg{Event: ev})
			return true
		}); err != nil {
			c.publishCompactError(err)
		}
	}()
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
	c.Cancel()
	if c.unsubJobs != nil {
		c.unsubJobs()
		c.unsubJobs = nil
	}
	if c.jobs != nil {
		_ = c.jobs.Close(context.Background())
	}
	if c.mcpPool != nil {
		_ = c.mcpPool.Close()
		c.mcpPool = nil
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
	ok := c.streamGen == gen
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

func (c *Controller) runLoop(ctx context.Context, gen int, prompt string, pendingSkills []string) {
	if !c.waitOrDone(ctx, gen, 120*time.Millisecond) {
		return
	}
	c.publish(SetActivityMsg{Activity: ActivityStreaming})

	if c.engine == nil {
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

	for ev, err := range c.engine.Loop(ctx, prompt, agent.LoopOpts{PendingSkills: pendingSkills}) {
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
		}
	}
}
