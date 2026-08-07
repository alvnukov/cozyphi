package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tools"
)

// ChildTools returns the tool set for a sub-agent Engine.
// DefaultTools only — never includes agent_* — so children cannot spawn.
func ChildTools() []tools.Tool {
	return tools.DefaultTools()
}

// EngineRunner runs a child [Engine.Loop] as a [job.Runner].
//
// Each Run creates a fresh Engine with a persisted session under
// <job.Dir>/session/, ParentID from the job, and no Ask handler.
// Child engines do not receive Jobs, so they have no agent_* tools.
type EngineRunner struct {
	Model     llm.ModelConfig
	ModelFn   func() llm.ModelConfig // if set, preferred over Model
	Gate      permission.Gate        // nil → headless-strict on job WorkDir
	Tools     []tools.Tool           // nil → ChildTools()
	MaxRounds int                    // 0 → Engine default
}

// Run implements [job.Runner].
func (r EngineRunner) Run(ctx context.Context, env job.RunEnv) (string, error) {
	if env.Job.Dir == "" {
		return "", fmt.Errorf("agent: EngineRunner requires job Dir")
	}

	cwd := env.Job.WorkDir
	if cwd == "" {
		cwd = "."
	}

	gate := r.Gate
	if gate == nil {
		policy := permission.DefaultPolicy()
		policy.Mode = permission.ModeHeadlessStrict
		g, err := permission.NewGate(policy, cwd)
		if err != nil {
			return "", err
		}
		gate = g
	}

	toolList := r.Tools
	if toolList == nil {
		toolList = ChildTools()
	}

	model := r.Model
	if r.ModelFn != nil {
		model = r.ModelFn()
	}

	sessionDir := filepath.Join(env.Job.Dir, "session")
	engine, err := NewEngine(EngineOpts{
		Model:     model,
		Gate:      gate,
		Ask:       nil,
		Tools:     toolList,
		MaxRounds: r.MaxRounds,
		SessionOpts: SessionOpts{
			Cwd:        cwd,
			SessionDir: sessionDir,
			Persist:    true,
			ParentID:   env.Job.ParentID,
		},
	})
	if err != nil {
		return "", err
	}

	env.Log(fmt.Sprintf("sub-agent session=%s parent=%s", engine.SessionID(), env.Job.ParentID))

	prompt := env.Job.Prompt
	if env.Job.Description != "" {
		prompt = env.Job.Description + "\n\n" + prompt
	}
	prompt = prompt + "\n\n" + subagentSummaryHint

	var (
		lastText string
		lastErr  error
	)
	for ev, loopErr := range engine.Loop(ctx, prompt, LoopOpts{}) {
		if loopErr != nil {
			lastErr = loopErr
			env.Log("error: " + loopErr.Error())
			break
		}
		switch e := ev.(type) {
		case session.AssistantMessageUpdate:
			if e.Message.State == session.StateComplete {
				if t := strings.TrimSpace(e.Message.FlatText()); t != "" {
					lastText = t
				}
			}
		case session.ToolData:
			detail := e.Run.Detail
			if detail == "" {
				detail = e.Run.Name
			}
			env.Log(fmt.Sprintf("tool %s %s: %s", e.Run.Name, e.Run.Status, detail))
			if env.OnProgress != nil {
				env.OnProgress(job.Progress{
					ToolUseID: e.Run.ToolUseID,
					Name:      e.Run.Name,
					Status:    e.Run.Status.String(),
					Detail:    detail,
				})
			}
		}
	}

	if path := engine.SessionFile(); path != "" {
		env.Log("session_file=" + path)
	}

	if lastErr != nil {
		if lastText != "" {
			_ = env.WriteResult(lastText)
		}
		return lastText, lastErr
	}
	if ctx.Err() != nil {
		return lastText, ctx.Err()
	}
	if lastText == "" {
		lastText = "sub-agent finished with no text reply"
	}
	return lastText, nil
}

const subagentSummaryHint = `When you are done, reply with a concise summary of findings and any file paths that matter. The parent agent will only see your final reply, not the full tool transcript.`
