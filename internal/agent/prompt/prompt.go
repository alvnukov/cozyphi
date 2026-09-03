// Package prompt builds the agent system prompt from templates and catalogs.
package prompt

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/alvnukov/cozyphi/internal/llm/skills"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/tasks"
)

var (
	//go:embed system-prompt.tmpl
	systemPromptTmpl string
	//go:embed skills-prompt.tmpl
	skillsPromptTmpl string
	//go:embed mcp-prompt.tmpl
	mcpPromptTmpl string
	//go:embed plan-prompt.tmpl
	planPromptTmpl string

	systemPrompt = template.Must(template.New("system").Parse(systemPromptTmpl))
	skillsPrompt = template.Must(template.New("skills").Parse(skillsPromptTmpl))
	mcpPrompt    = template.Must(template.New("mcp").Parse(mcpPromptTmpl))
	planPrompt   = template.Must(template.New("plan").Parse(planPromptTmpl))
)

type systemData struct {
	Cwd           string
	Workspace     string
	AgentsEnabled bool
	LSPEnabled    bool
	WatchEnabled  bool
	TasksEnabled  bool
	// TasksAccess is the level the paragraph is written for: read tells the
	// model to describe changes, ask to make each one whole, write nothing
	// more.
	TasksAccess string
}

type skillsData struct {
	Catalog string
}

type mcpData struct {
	Servers []string
}

// planData selects the appendix variant: the closed authoring_policy decides
// whether the grammar block renders, Tasks whether the plan may shape the
// task registry (a writable level: ask or write).
type planData struct {
	Grammar bool
	Tasks   bool
}

// Options says which optional capabilities this engine actually has. Every
// flag must match whether the matching tools are registered: the prompt tells
// the model what to reach for, and a prompt that names a tool the engine does
// not carry is worse than one that says nothing.
//
// It is a struct rather than a parameter list because the flags are all bools
// that read the same at a call site — `engine.jobs != nil, engine.lsp != nil,
// engine.watches != nil` transposes silently and compiles.
type Options struct {
	SkillPath string
	// Agents reports whether agent_* tools are registered.
	Agents bool
	// LSP reports whether the lsp tool is registered.
	LSP bool
	// Watches reports whether the watch tool is registered.
	Watches bool
	// Tasks is the task registry level the tool was registered at. Empty or
	// off means no task tool, and the prompt says nothing about a registry.
	Tasks tasks.Access
	// MCPServers are configured server names only (no tool schemas).
	MCPServers []string
	// Plan appends the plan-mode appendix (read-only exploration, numbered plan).
	Plan bool
	// PlanGrammar carries plangate's closed authoring_policy to the appendix:
	// legacy renders the pre-grammar appendix; anything else (the empty
	// default) appends the authoring grammar.
	PlanGrammar plangate.AuthoringPolicy
}

// Build assembles the system prompt.
func Build(opts Options) string {
	var buf strings.Builder
	data := systemData{
		Cwd:           currentDir(),
		Workspace:     workspaceDir(),
		AgentsEnabled: opts.Agents,
		LSPEnabled:    opts.LSP,
		WatchEnabled:  opts.Watches,
		TasksEnabled:  tasksEnabled(opts.Tasks),
		TasksAccess:   string(opts.Tasks.Normalized()),
	}
	if err := systemPrompt.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("system prompt: %v", err))
	}
	parts := []string{buf.String()}
	if ctx := formatProjectContext(loadProjectContextFiles(currentDir(), cozyPhiAgentDir())); ctx != "" {
		parts = append(parts, ctx)
	}
	if skillBlock := skillsBlock(opts.SkillPath); skillBlock != "" {
		parts = append(parts, skillBlock)
	}
	if mcpBlock := mcpBlock(opts.MCPServers); mcpBlock != "" {
		parts = append(parts, mcpBlock)
	}
	if opts.Plan {
		parts = append(parts, execTmpl(planPrompt, planData{
			Grammar: opts.PlanGrammar != plangate.AuthoringLegacy,
			Tasks:   tasksEnabled(opts.Tasks) && opts.Tasks.Writable(),
		}))
	}
	return strings.Join(parts, "\n\n")
}

// tasksEnabled reads Options.Tasks the way the engine sets it: empty is no
// registry, off is a registry the user switched off; both mean silence.
func tasksEnabled(level tasks.Access) bool {
	return level != "" && level != tasks.AccessOff
}

func execTmpl(t *template.Template, data any) string {
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("%s prompt: %v", t.Name(), err))
	}
	return strings.TrimSpace(buf.String())
}

func skillsBlock(skillDir string) string {
	if skillDir == "" {
		return ""
	}
	list, err := skills.LoadSkills(skillDir)
	if err != nil || len(list) == 0 {
		return ""
	}
	catalog := strings.TrimSpace(skills.ToPromptMarkdown(list))
	if catalog == "" {
		return ""
	}
	return execTmpl(skillsPrompt, skillsData{Catalog: catalog})
}

func mcpBlock(serverNames []string) string {
	servers := make([]string, 0, len(serverNames))
	for _, name := range serverNames {
		name = strings.TrimSpace(name)
		if name != "" {
			servers = append(servers, name)
		}
	}
	if len(servers) == 0 {
		return ""
	}
	return execTmpl(mcpPrompt, mcpData{Servers: servers})
}

func currentDir() string {
	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return path
}

// workspaceDir returns the nearest ancestor of cwd that contains .git, or "".
func workspaceDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
