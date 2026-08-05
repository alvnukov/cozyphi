package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pulseaiclub/phi/internal/agent"
	"github.com/pulseaiclub/phi/internal/llm"
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
}

func NewController(bus *Bus) *Controller {
	c := &Controller{bus: bus}
	proj := project.GetDefaultProject()
	cwd, _ := os.Getwd()
	c.cwd = cwd
	c.sessionDir = proj.SessionDir()

	if err := proj.LoadConfig(); err != nil {
		c.engineErr = err
		return c
	}
	c.modelCfg = proj.Config().Model()
	eng, err := agent.NewEngine(agent.EngineOpts{
		Model: c.modelCfg,
		SessionOpts: agent.SessionOpts{
			Cwd:        cwd,
			SessionDir: c.sessionDir,
			Persist:    true,
		},
	})
	if err != nil {
		c.engineErr = err
		return c
	}
	c.engine = eng
	return c
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
	cfg := proj.Config().Model()
	cfg.Name = name
	c.Cancel()
	if c.engine == nil {
		eng, err := agent.NewEngine(agent.EngineOpts{
			Model: cfg,
			SessionOpts: agent.SessionOpts{
				Cwd:        c.cwd,
				SessionDir: c.sessionDir,
				Persist:    true,
			},
		})
		if err != nil {
			return err
		}
		c.engine = eng
		c.modelCfg = cfg
		c.engineErr = nil
		return nil
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

	eng, err := agent.NewEngine(agent.EngineOpts{
		Model: cfg,
		SessionOpts: agent.SessionOpts{
			Cwd:        c.cwd,
			SessionDir: c.sessionDir,
			Persist:    true,
			ResumeID:   id,
		},
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
					blocks = append(blocks, session.ContentBlock{Type: session.BlockThinking, Text: msg.ReasoningContent})
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
