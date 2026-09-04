package agent

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/llm/skills"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// EngineRunner runs a child [Engine.Loop] as a [job.Runner].
//
// Child assembly lives in one place, buildChild: role spec, permission gate,
// tools, session location and prompt. The child's write boundary is the spawn
// workdir — job.Manager.Spawn has already validated it against the parent
// workspace, and buildChild re-asserts it so no Spawn caller can widen the
// boundary through persisted meta. Child engines get no Ask handler and no
// Jobs, so they have no agent_* tools.
//
// Hooks (or HooksFn) are inherited from the parent so org policy applies to
// sub-agents the same way. HooksFn wins when set (live reload).
//
// Memory is not inherited: a child gets no memory store, so remembering stays
// a decision of the session the user is actually in, and a child's scoped
// context never grows a fact directory it cannot act on.
//
// Watches are not inherited either, for a blunter reason: a watch outlives the
// turn that started it, and a child ends. A watch a child started would fire
// into a session that has no idea who asked for it, so children get no manager
// and therefore no watch tool.
type EngineRunner struct {
	Model        llm.ModelConfig
	ModelFn      func() llm.ModelConfig                 // if set, preferred over Model
	ModelForRole func(job.Role) (llm.ModelConfig, bool) // if set and the role resolves (agents.models), preferred over Model/ModelFn
	MaxRounds    int                                    // 0 → Engine default
	Hooks        *hooks.Manager                         // shared with parent; nil = no hooks
	HooksFn      func() *hooks.Manager                  // if set, preferred over Hooks
	LSP          tools.LSPQueryFunc                     // borrowed shared manager query; nil disables the tool
}

// Run implements [job.Runner].
func (r EngineRunner) Run(ctx context.Context, env job.RunEnv) (string, error) {
	if env.Job.Dir == "" {
		return "", errors.New("agent: EngineRunner requires job Dir")
	}

	engine, prompt, err := r.buildChild(env.Job)
	if err != nil {
		return "", err
	}

	env.Log(fmt.Sprintf(
		"sub-agent role=%s session=%s parent=%s",
		job.NormalizeRole(string(env.Job.Role)),
		engine.SessionID(),
		env.Job.ParentID,
	))

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

// buildChild is the single factory for a sub-agent: it turns one persisted
// job into a configured engine plus the assembled prompt. Everything that
// decides what a child may do — tools, permission mode, write boundary —
// is chosen here and nowhere else.
func (r EngineRunner) buildChild(meta job.Meta) (*Engine, string, error) {
	spec := SpecForRole(meta.Role)

	cwd := meta.WorkDir
	if cwd == "" {
		cwd = "."
	}
	if ws := meta.ParentWorkspace; ws != "" && !permission.WithinWorkspaceResolved(cwd, ws) {
		return nil, "", fmt.Errorf("agent: workdir %q outside parent workspace %q", cwd, ws)
	}

	policy := permission.DefaultPolicy()
	policy.Mode = spec.Mode
	gate, err := permission.NewGate(policy, cwd)
	if err != nil {
		return nil, "", err
	}

	model := r.Model
	if r.ModelFn != nil {
		model = r.ModelFn()
	}
	// A role pinned in agents.models wins over the session model: it is an
	// explicit per-role choice, while Model/ModelFn is the ambient default.
	// An unset role, or a name that no longer resolves, keeps the ambient
	// model — inheritance, not an error.
	if r.ModelForRole != nil {
		if m, ok := r.ModelForRole(meta.Role); ok {
			model = m
		}
	}

	// The parent's skills decision is durable in meta, but the bodies are
	// not: they load here, from the child model's catalog, before any engine
	// (and thus any child session) exists — a name that no longer resolves
	// fails the job rather than silently shrinking the child's guidance.
	skillsBlock, err := renderJobSkills(model.SkillPath, meta.Skills)
	if err != nil {
		return nil, "", err
	}

	hookMgr := r.Hooks
	if r.HooksFn != nil {
		hookMgr = r.HooksFn()
	}

	engine, err := NewEngine(EngineOpts{
		Model:     model,
		Gate:      gate,
		Ask:       nil,
		Tools:     spec.Tools,
		MaxRounds: r.MaxRounds,
		Hooks:     hookMgr,
		LSP:       r.LSP,
		SessionOpts: SessionOpts{
			Cwd:        cwd,
			SessionDir: filepath.Join(meta.Dir, "session"),
			Persist:    true,
			ParentID:   meta.ParentID,
		},
	})
	if err != nil {
		return nil, "", err
	}

	prompt := meta.Prompt
	if meta.Description != "" {
		prompt = meta.Description + "\n\n" + prompt
	}
	prompt += "\n\n" + spec.Hint
	if skillsBlock != "" {
		prompt += "\n\n" + skillsBlock
	}

	return engine, prompt, nil
}

// renderJobSkills loads and renders the skill set the parent pinned for a
// job: one intro line, then each body as plain text under its name — the
// drainPlanSkills format, so a child reads its skills the way a plan step
// does, spending no read call on them. An empty selection renders empty; a
// name the catalog cannot resolve is an error naming it, never a quiet skip.
func renderJobSkills(skillPath string, names []string) (string, error) {
	if len(names) == 0 {
		return "", nil
	}
	catalog, err := skills.LoadSkills(skillPath)
	if err != nil {
		return "", fmt.Errorf("agent: load skills for job from %s: %w", skillPath, err)
	}
	var out strings.Builder
	out.WriteString(
		"The parent equipped this job with these skills. Follow them; their SKILL.md files need no read call.",
	)
	for _, name := range names {
		skill := skills.Find(catalog, name)
		if skill == nil {
			return "", fmt.Errorf(
				"agent: job skill %q is not installed in %s — re-spawn the job with a skill that exists, or skills: []",
				name, skillPath,
			)
		}
		out.WriteString("\n\n## Skill: ")
		out.WriteString(skill.Name)
		out.WriteString("\n\n")
		out.WriteString(skill.Body)
	}
	return out.String(), nil
}
