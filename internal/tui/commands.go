package tui

import (
	"fmt"
	"strings"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/mention"
	"github.com/pulseaiclub/phi/internal/components/palette"
	"github.com/pulseaiclub/phi/internal/hooks"
	"github.com/pulseaiclub/phi/internal/llm/skills"
)

// SlashCommand is one composer `/` command shown in the slash picker.
type SlashCommand struct {
	Name        string // without leading slash, e.g. "resume"
	Description string
	// Insert is written into the composer on accept (include trailing space when args follow).
	Insert string
}

// SlashCommands is the built-in slash catalog.
func SlashCommands() []SlashCommand {
	return []SlashCommand{
		{
			Name:        "sessions",
			Description: "List sessions for this directory",
			Insert:      "/sessions",
		},
		{
			Name:        "resume",
			Description: "Resume a session in this directory — /resume <id>",
			Insert:      "/resume ",
		},
	}
}

// FilterSlashCommands returns commands whose name starts with query (case-insensitive).
func FilterSlashCommands(query string) []mention.Item {
	q := strings.ToLower(strings.TrimSpace(query))
	all := SlashCommands()
	out := make([]mention.Item, 0, len(all))
	for _, c := range all {
		if q != "" && !strings.HasPrefix(strings.ToLower(c.Name), q) {
			continue
		}
		out = append(out, mention.Item{
			Path:        c.Name,
			Description: c.Description,
		})
	}
	return out
}

// LookupSlashInsert returns the Insert string for a command name, or empty.
func LookupSlashInsert(name string) string {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	for _, c := range SlashCommands() {
		if strings.EqualFold(c.Name, name) {
			return c.Insert
		}
	}
	return ""
}

// PaletteCommands returns model-switch commands for the command palette,
// one entry per configured model name.
func PaletteCommands(onModel func(name string), modelNames []string) []palette.PaletteCommand {
	models := make([]palette.PaletteCommand, 0, len(modelNames))
	for _, name := range modelNames {
		models = append(models, palette.PaletteCommand{
			ID:   "model-" + name,
			Verb: name,
			Run: func() {
				if onModel != nil {
					onModel(name)
				}
			},
		})
	}
	if len(models) == 0 {
		models = append(models, palette.PaletteCommand{
			ID:       "model-empty",
			Verb:     "No models configured",
			Disabled: true,
		})
	}
	return []palette.PaletteCommand{
		{
			ID:           "settings-model",
			Noun:         "settings",
			Verb:         "model",
			Keywords:     []string{"model"},
			SubmenuTitle: "Select Model",
			Submenu:      models,
		},
	}
}

// ThemeCommand returns a settings → theme submenu listing builtin palettes.
func ThemeCommand(apply func(name string)) palette.PaletteCommand {
	names := components.ThemeNames()
	submenu := make([]palette.PaletteCommand, 0, len(names))
	for _, name := range names {
		submenu = append(submenu, palette.PaletteCommand{
			ID:       "theme-" + strings.ToLower(name),
			Verb:     name + " (builtin)",
			Keywords: []string{name, "theme", "color"},
			Run: func() {
				if apply != nil {
					apply(name)
				}
			},
		})
	}
	return palette.PaletteCommand{
		ID:           "settings-theme",
		Noun:         "settings",
		Verb:         "theme",
		Keywords:     []string{"theme", "color", "appearance", "dark", "darcula", "pink"},
		SubmenuTitle: "Select Theme",
		Submenu:      submenu,
	}
}

// PermissionsCommand returns settings → permissions to toggle session bypass.
// bypass=true means no permission prompts (allow all).
func PermissionsCommand(set func(bypass bool)) palette.PaletteCommand {
	return palette.PaletteCommand{
		ID:           "settings-permissions",
		Noun:         "settings",
		Verb:         "permissions",
		Keywords:     []string{"permission", "bypass", "allow all", "ask", "gate", "security"},
		SubmenuTitle: "Permissions",
		Submenu: []palette.PaletteCommand{
			{
				ID:       "permissions-off",
				Verb:     "off — allow all (no prompts)",
				Keywords: []string{"bypass", "disable", "off"},
				Run: func() {
					if set != nil {
						set(true)
					}
				},
			},
			{
				ID:       "permissions-on",
				Verb:     "on — ask before gated tools",
				Keywords: []string{"enable", "ask", "on", "interactive"},
				Run: func() {
					if set != nil {
						set(false)
					}
				},
			},
		},
	}
}

// AgentsCommand returns settings → agents to toggle sub-agent tools.
func AgentsCommand(set func(enabled bool)) palette.PaletteCommand {
	return palette.PaletteCommand{
		ID:           "settings-agents",
		Noun:         "settings",
		Verb:         "agents",
		Keywords:     []string{"agent", "subagent", "spawn", "jobs", "parallel"},
		SubmenuTitle: "Sub-agents",
		Submenu: []palette.PaletteCommand{
			{
				ID:       "agents-on",
				Verb:     "on — register agent_* tools",
				Keywords: []string{"enable", "on", "spawn"},
				Run: func() {
					if set != nil {
						set(true)
					}
				},
			},
			{
				ID:       "agents-off",
				Verb:     "off — no sub-agents (fewer tools)",
				Keywords: []string{"disable", "off"},
				Run: func() {
					if set != nil {
						set(false)
					}
				},
			},
		},
	}
}

// HooksCommand returns hooks → list / reload for the command palette.
// listFn builds the nested list page (called when the user picks "list").
// reload runs a disk re-discovery and swaps the live manager.
func HooksCommand(
	pal *palette.CommandPalette,
	listFn func() []palette.PaletteCommand,
	reload func(),
) palette.PaletteCommand {
	return palette.PaletteCommand{
		ID:           "hooks",
		Noun:         "hooks",
		Verb:         "manage",
		Keywords:     []string{"hook", "plugin", "policy", "reload", "list"},
		SubmenuTitle: "Hooks",
		Submenu: []palette.PaletteCommand{
			{
				ID:       "hooks-list",
				Verb:     "list",
				Keywords: []string{"show", "status", "loaded"},
				Run: func() {
					cmds := []palette.PaletteCommand{{
						ID:       "hooks-list-empty",
						Verb:     "No hooks found",
						Disabled: true,
					}}
					if listFn != nil {
						if built := listFn(); len(built) > 0 {
							cmds = built
						}
					}
					if pal != nil {
						pal.Push("Hooks on disk", cmds)
					}
				},
			},
			{
				ID:       "hooks-reload",
				Verb:     "reload",
				Keywords: []string{"refresh", "rescan", "discover"},
				Run: func() {
					if reload != nil {
						reload()
					}
				},
			},
		},
	}
}

// HookListEntries builds disabled palette rows from discovery results + warnings.
func HookListEntries(found []hooks.Discovered, warns []hooks.Warning, err error) []palette.PaletteCommand {
	if err != nil {
		return []palette.PaletteCommand{{
			ID:       "hooks-list-err",
			Verb:     "error: " + err.Error(),
			Disabled: true,
		}}
	}
	out := make([]palette.PaletteCommand, 0, len(found)+len(warns)+1)
	if len(found) == 0 && len(warns) == 0 {
		out = append(out, palette.PaletteCommand{
			ID:       "hooks-list-empty",
			Verb:     "No hooks found",
			Disabled: true,
		})
		return out
	}
	for _, d := range found {
		name := d.Manifest.Name
		out = append(out, palette.PaletteCommand{
			ID:       "hook-" + name,
			Verb:     hooks.FormatDiscovered(d),
			Keywords: []string{name, string(d.Manifest.Kind), d.Source},
			Disabled: true,
		})
	}
	for i, w := range warns {
		out = append(out, palette.PaletteCommand{
			ID:       fmt.Sprintf("hooks-warn-%d", i),
			Verb:     "warn: " + w.String(),
			Keywords: []string{"warning", "error"},
			Disabled: true,
		})
	}
	return out
}

// SkillsCommand returns a top-level "skills" palette entry whose submenu lists
// every skill discovered under skillPath. Selecting one adds it as a pending skill.
func SkillsCommand(skillPath string, add func(name string)) palette.PaletteCommand {
	submenu := skillSubcommands(skillPath, add)
	return palette.PaletteCommand{
		ID:           "skills",
		Noun:         "skills",
		Verb:         "invoke",
		Keywords:     []string{"skill", "use skill", "load skill", "pending"},
		SubmenuTitle: "Select skill",
		Submenu:      submenu,
	}
}

func skillSubcommands(skillPath string, add func(name string)) []palette.PaletteCommand {
	list, err := skills.LoadSkills(skillPath)
	if err != nil || len(list) == 0 {
		return []palette.PaletteCommand{{
			ID:       "skills-empty",
			Verb:     "No skills found",
			Disabled: true,
		}}
	}

	out := make([]palette.PaletteCommand, 0, len(list))
	for _, s := range list {
		name := s.Name
		if strings.TrimSpace(name) == "" {
			name = strings.TrimSpace(s.Path)
		}
		out = append(out, palette.PaletteCommand{
			ID:       "skill-" + name,
			Verb:     name,
			Keywords: []string{s.Description, "skill"},
			Run: func() {
				if add != nil {
					add(name)
				}
			},
		})
	}
	return out
}
