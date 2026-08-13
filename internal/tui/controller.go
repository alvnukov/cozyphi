package tui

import (
	"context"
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
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/project"
	"github.com/pulseaiclub/phi/internal/session"
)

// Controller owns agent.Engine lifecycle and stream cancellation.
// It talks to the UI only by publishing Msg values onto the Bus.
type Controller struct {
	engine    *agent.Engine
	engineErr error

	streamMu     sync.Mutex
	streamCancel context.CancelFunc
	streamGen    int

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
	hooksMgr      atomic.Pointer[hooks.Manager]

	// lastJobProgress dedupes identical Progress publishes (key → signature).
	lastJobProgress sync.Map
}

func NewController(bus *Bus) *Controller {
	c := &Controller{bus: bus, askTimeoutSec: 120}
	// Default: no permission prompts. Toggle via command palette → settings → permissions.
	c.allowAll.Store(true)
	proj := project.GetDefaultProject()
	cwd, _ := os.Getwd()
	c.cwd = cwd
	c.sessionDir = proj.SessionDir()

	if err := proj.LoadConfig(); err != nil {
		c.engineErr = err
		return c
	}
	c.modelCfg = proj.Config().Model()
	c.initGate(proj.Config().Permissions)
	c.agentsEnabled.Store(proj.Config().Agents.Enabled)

	hooksMgr := loadHooksManager(proj, cwd)
	c.hooksMgr.Store(hooksMgr)
	jobs, err := agent.NewJobManager(proj.JobsDir(), c.modelCfg, func() llm.ModelConfig {
		return c.modelCfg
	}, c.Hooks)
	if err != nil {
		c.engineErr = err
		return c
	}
	c.jobs = jobs

	eng, err := agent.NewEngine(agent.EngineOpts{
		Model: c.modelCfg,
		SessionOpts: agent.SessionOpts{
			Cwd:        cwd,
			SessionDir: c.sessionDir,
			Persist:    true,
		},
		Gate:        c.gate,
		Ask:         c.askPermission,
		ContinueAsk: c.askContinue,
		Jobs:        c.engineJobs(),
		Hooks:       hooksMgr,
	})
	if err != nil {
		c.engineErr = err
		return c
	}
	c.engine = eng
	c.startJobProgress()
	return c
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
	if policy.DangerouslyAllowAll {
		c.allowAll.Store(true)
	}
	// Do not clear allowAll when config omits dangerously_allow_all — TUI defaults
	// to bypass, and the palette toggle must survive SetModel / re-init.
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
	return c.hooksMgr.Load()
}

// ReloadHooks re-discovers hooks from disk and swaps the manager on the engine
// (and on future sub-agents via Hooks()).
func (c *Controller) ReloadHooks() (loaded int, warns []hooks.Warning, err error) {
	if c == nil {
		return 0, nil, fmt.Errorf("controller not initialized")
	}
	proj := project.GetDefaultProject()
	if proj == nil {
		return 0, nil, fmt.Errorf("project not available")
	}
	found, warns, err := hooks.DiscoverForCwd(proj.Global().HooksDir(), c.cwd)
	if err != nil {
		return 0, warns, err
	}
	mgr := hooks.NewManager(hooks.EntriesFromDiscovered(found)...)
	hooks.LogWarnings(warns)
	c.hooksMgr.Store(mgr)
	if c.engine != nil {
		c.engine.SetHooks(mgr)
	}
	return len(found), warns, nil
}

// ListHooks returns the current on-disk discovery (does not swap the manager).
func (c *Controller) ListHooks() ([]hooks.Discovered, []hooks.Warning, error) {
	if c == nil {
		return nil, nil, fmt.Errorf("controller not initialized")
	}
	proj := project.GetDefaultProject()
	if proj == nil {
		return nil, nil, fmt.Errorf("project not available")
	}
	return hooks.DiscoverForCwd(proj.Global().HooksDir(), c.cwd)
}

// loadHooksManager discovers ~/.phi/hooks and <cwd>/.phi/hooks.
// Load errors are non-fatal (fail-open: no hooks). Child engines stay nil until S9.
func loadHooksManager(proj *project.Project, cwd string) *hooks.Manager {
	if proj == nil {
		return nil
	}
	mgr, warns, err := hooks.LoadForCwd(proj.Global().HooksDir(), cwd)
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
			_ = project.SetDangerouslyAllowAll(project.GetDefaultProject().Global(), true)
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
		return fmt.Errorf("empty model name")
	}
	proj := project.GetDefaultProject()
	if err := proj.LoadConfig(); err != nil {
		return err
	}
	cfg, ok := proj.Config().FindModel(name)
	if !ok {
		// Not a configured model: keep the primary's connection settings and
		// only swap the name (arbitrary-model workflow).
		cfg = proj.Config().Model()
		cfg.Name = name
	}
	c.Cancel()
	c.initGate(proj.Config().Permissions)
	if c.engine == nil {
		mgr := loadHooksManager(proj, c.cwd)
		c.hooksMgr.Store(mgr)
		eng, err := agent.NewEngine(agent.EngineOpts{
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
			Hooks:       mgr,
		})
		if err != nil {
			return err
		}
		c.engine = eng
		c.modelCfg = cfg
		c.engineErr = nil
		return nil
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
	c.engineErr = nil
	return nil
}

// SessionID returns the short-form-friendly session id.
func (c *Controller) SessionID() string {
	if c.engine == nil {
		return ""
	}
	return c.engine.SessionID()
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
		return "", fmt.Errorf("empty session id")
	}
	if c.sessionDir == "" {
		return "", fmt.Errorf("session directory not configured")
	}
	c.Cancel()

	cfg := c.modelCfg
	if cfg.Name == "" {
		proj := project.GetDefaultProject()
		if err := proj.LoadConfig(); err != nil {
			return "", err
		}
		cfg = proj.Config().Model()
	}

	mgr := loadHooksManager(project.GetDefaultProject(), c.cwd)
	c.hooksMgr.Store(mgr)
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
	})
	if err != nil {
		return "", err
	}
	if sessCwd := eng.SessionCwd(); sessCwd != "" && c.cwd != "" && sessCwd != c.cwd {
		cwdWarning = fmt.Sprintf("session cwd is %s (current %s); not changing directory", sessCwd, c.cwd)
	}
	c.engine = eng
	c.modelCfg = cfg
	c.engineErr = nil
	return cwdWarning, nil
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
				blocks := []session.ContentBlock{}
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

// Close cancels the stream and shuts down the job manager.
func (c *Controller) Close() {
	c.Cancel()
	if c.unsubJobs != nil {
		c.unsubJobs()
		c.unsubJobs = nil
	}
	if c.jobs != nil {
		_ = c.jobs.Close(context.Background())
	}
}

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
		if c.engineErr != nil {
			errText = c.engineErr.Error()
		}
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
		}
	}
}
