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

// Facts is safe metadata about one prompt render: how much of each kind of
// source the assembled prompt carries, and how large it came out. Counts,
// sizes and scope labels only — never a path, never a name, never a byte of
// what was loaded.
//
// It exists because Build reads the instruction files and the skill catalog
// from disk every time it runs. A read-only observer must never do that, so
// the render that already happened records what it took in and the observer
// reports the record rather than repeating the load.
type Facts struct {
	// Bytes is the size of the prompt this render produced, before anything
	// the caller appends to it.
	Bytes int
	// Instructions is how many project-instruction files the prompt carries.
	Instructions int
	// InstructionScopes labels where each of them was found, in load order:
	// agent_dir, ancestor or workspace.
	InstructionScopes []string
	// SkillDir is whether a skill directory was configured for this render.
	// Without one no catalog is read at all, which is a different answer from
	// a directory that held nothing.
	SkillDir bool
	// Skills is how many skills the catalog block names. Their bodies are not
	// in the prompt and are not counted here.
	Skills int
}

// Build assembles the system prompt.
func Build(opts Options) string {
	text, _ := BuildWithFacts(opts)
	return text
}

// BuildWithFacts assembles the system prompt and reports what the render
// loaded. The facts are a by-product of the assembly, measured as it happens:
// nothing here is read twice, and a caller that wants the metadata never has
// to build the prompt again to get it.
func BuildWithFacts(opts Options) (string, Facts) {
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
	files := loadProjectContextFiles(currentDir(), cozyPhiAgentDir())
	facts := Facts{
		Instructions:      len(files),
		InstructionScopes: contextScopes(files),
		SkillDir:          strings.TrimSpace(opts.SkillPath) != "",
	}
	if ctx := formatProjectContext(files); ctx != "" {
		parts = append(parts, ctx)
	}
	skillBlock, loadedSkills := skillsBlock(opts.SkillPath)
	facts.Skills = loadedSkills
	if skillBlock != "" {
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
	out := strings.Join(parts, "\n\n")
	facts.Bytes = len(out)
	return out, facts
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

// skillsBlock renders the catalog and reports how many skills it names, so
// the render's own count is the one recorded rather than a second load's.
func skillsBlock(skillDir string) (string, int) {
	if skillDir == "" {
		return "", 0
	}
	list, err := skills.LoadSkills(skillDir)
	if err != nil || len(list) == 0 {
		return "", 0
	}
	catalog := strings.TrimSpace(skills.ToPromptMarkdown(list))
	if catalog == "" {
		return "", 0
	}
	return execTmpl(skillsPrompt, skillsData{Catalog: catalog}), len(list)
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
