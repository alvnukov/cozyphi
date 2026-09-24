// Package plugin finds the Claude Code plugins cozyphi loads — those enabled
// in Claude Code's own settings and those listed under plugins.paths — and
// describes each one as the skill sources and hook files the rest of cozyphi
// consumes. It reads the disk and nothing else: a plugin is data.
package plugin

import (
	"os"
	"strings"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm/skills"
)

// EnvPlugins set to "off" (any case) disables plugin loading for the process.
const EnvPlugins = "COZYPHI_PLUGINS"

// Disabled reports whether COZYPHI_PLUGINS=off.
func Disabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(EnvPlugins)), "off")
}

// Config is the plugins section of config.yaml after defaults and ~ expansion.
type Config struct {
	Enabled bool
	// ClaudeDir is Claude Code's home (~/.claude): installed_plugins.json and
	// settings.json are read from it, never written.
	ClaudeDir string
	// Paths are local plugin roots, absolute, always enabled.
	Paths []string
	// LocalDataDir is the parent of a local plugin's data directory.
	LocalDataDir string
}

// Plugin is one enabled plugin, resolved to absolute paths.
type Plugin struct {
	ID        string   // "superpowers@claude-plugins-official"; a local plugin's Name
	Name      string   // namespace for skills and hook names
	Root      string   // ${CLAUDE_PLUGIN_ROOT}
	DataDir   string   // ${CLAUDE_PLUGIN_DATA}; created by the first hook run
	SkillDirs []string // skills/, a root holding SKILL.md, then manifest extras
	HookFiles []string // hooks/hooks.json, then manifest extras
}

// Warning is a discovery problem that skipped something without failing.
type Warning struct {
	Plugin string // ID, or the path when no ID could be read
	Msg    string // says what is wrong and what to do
}

func (w Warning) String() string {
	if w.Plugin == "" {
		return w.Msg
	}
	return w.Plugin + ": " + w.Msg
}

// Vars are the placeholder values the plugin's skills and hooks see.
func (p Plugin) Vars(projectRoot string) map[string]string {
	return map[string]string{
		hooks.EnvClaudePluginRoot: p.Root,
		hooks.EnvClaudePluginData: p.DataDir,
		hooks.EnvClaudeProjectDir: projectRoot,
	}
}

// SkillSources is one namespaced source per skill directory.
func (p Plugin) SkillSources(projectRoot string) skills.Sources {
	vars := p.Vars(projectRoot)
	out := make(skills.Sources, 0, len(p.SkillDirs))
	for _, dir := range p.SkillDirs {
		out = append(out, skills.Source{Dir: dir, Namespace: p.Name, Vars: vars})
	}
	return out
}

// HookSources describes the plugin's hook files for hooks.Discover.
func (p Plugin) HookSources(projectRoot string) hooks.PluginHooks {
	return hooks.PluginHooks{Name: p.Name, Files: p.HookFiles, Vars: p.Vars(projectRoot)}
}
