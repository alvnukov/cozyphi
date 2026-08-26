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
}

type skillsData struct {
	Catalog string
}

type mcpData struct {
	Servers []string
}

// Build assembles the system prompt.
// agentsEnabled must match whether agent_* tools are registered.
// lspEnabled must match whether the lsp tool is registered.
// mcpServers are configured server names only (no tool schemas).
// plan appends the plan-mode appendix (read-only exploration, numbered plan).
func Build(skillPath string, agentsEnabled, lspEnabled bool, mcpServers []string, plan bool) string {
	var buf strings.Builder
	data := systemData{
		Cwd:           currentDir(),
		Workspace:     workspaceDir(),
		AgentsEnabled: agentsEnabled,
		LSPEnabled:    lspEnabled,
	}
	if err := systemPrompt.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("system prompt: %v", err))
	}
	parts := []string{buf.String()}
	if ctx := formatProjectContext(loadProjectContextFiles(currentDir(), cozyPhiAgentDir())); ctx != "" {
		parts = append(parts, ctx)
	}
	if skillBlock := skillsBlock(skillPath); skillBlock != "" {
		parts = append(parts, skillBlock)
	}
	if mcpBlock := mcpBlock(mcpServers); mcpBlock != "" {
		parts = append(parts, mcpBlock)
	}
	if plan {
		parts = append(parts, execTmpl(planPrompt, nil))
	}
	return strings.Join(parts, "\n\n")
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
