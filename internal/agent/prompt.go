package agent

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pulseaiclub/phi/internal/llm/skills"
)

var (
	//go:embed system-prompt.tmpl
	systemPromptTmpl string
)

const defaultMaxToolRounds = 64

func GetCurrentDir() string {
	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return path
}

func GetWorkspaceDir() string {
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

// Prompt builds the system prompt. Loads AGENTS.md/CLAUDE.md (global ~/.phi +
// cwd ancestors, same rules as pi-main) into <project_context>, then optionally
// appends a Skills block when skillPath is non-empty.
func Prompt(skillPath string) string {
	base := fmt.Sprintf(systemPromptTmpl, GetCurrentDir(), GetWorkspaceDir())
	parts := []string{base}
	if ctx := formatProjectContext(loadProjectContextFiles(GetCurrentDir(), phiAgentDir())); ctx != "" {
		parts = append(parts, ctx)
	}
	if skillBlock := loadSkillsBlock(skillPath); skillBlock != "" {
		parts = append(parts, "# Skills\n\n"+skillBlock)
	}
	return strings.Join(parts, "\n\n")
}

// loadSkillsBlock loads skills from the given directory and returns a
// Markdown-formatted skills block. Returns "" if no skills are found.
func loadSkillsBlock(skillDir string) string {
	if skillDir == "" {
		return ""
	}
	list, err := skills.LoadSkills(skillDir)
	if err != nil || len(list) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Skills are reusable workflows loaded from the agent's skill directories.\n")
	sb.WriteString("Treat skills as constraints and shortcuts—apply the smallest relevant part.\n")
	sb.WriteString("\n")
	sb.WriteString("## How to use skills\n")
	sb.WriteString("- When a task matches a skill, read that skill's SKILL.md first and follow it.\n")
	sb.WriteString("- Interpret script/asset paths relative to that skill's directory.\n")
	sb.WriteString("- Follow each skill's name, description, and guidance; do not invent a parallel workflow.\n")
	sb.WriteString("\n")
	sb.WriteString("## Available skills\n")
	sb.WriteString(skills.ToPromptMarkdown(list))
	return sb.String()
}
