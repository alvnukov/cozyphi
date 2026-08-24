package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/mention"
	"github.com/pulseaiclub/phi/internal/components/palette"
	"github.com/pulseaiclub/phi/internal/components/toast"
	"github.com/pulseaiclub/phi/internal/hooks"
	"github.com/pulseaiclub/phi/internal/llm/skills"
)

// NewBuiltinRegistry returns the built-in slash + palette catalog.
func NewBuiltinRegistry() *CommandRegistry {
	r := NewCommandRegistry()
	registerBuiltinCommands(r)
	return r
}

func registerBuiltinCommands(r *CommandRegistry) {
	r.Register(Command{
		Name:        "sessions",
		Description: "List sessions for this directory",
		Slash:       true,
		Insert:      "/sessions",
		Run: func(ctx CommandContext) error {
			if ctx.Host != nil {
				ctx.Host.ShowSessions()
			}
			return nil
		},
	})
	r.Register(Command{
		Name:        "resume",
		Description: "Resume a session in this directory — /resume <id>",
		Slash:       true,
		Insert:      "/resume ",
		Run: func(ctx CommandContext) error {
			if len(ctx.Args) < 1 {
				ctx.toast("Usage: /resume <session-id>", toast.ToastWarning, 3*time.Second)
				return nil
			}
			if ctx.Host != nil {
				ctx.Host.ResumeSession(ctx.Args[0])
			}
			return nil
		},
	})
	r.Register(Command{
		Name:        "clear",
		Description: "Start a new empty session",
		Slash:       true,
		Insert:      "/clear",
		Run: func(ctx CommandContext) error {
			if ctx.Host != nil {
				ctx.Host.ClearSession()
			}
			return nil
		},
	})
	r.Register(Command{
		Name:        "new",
		Description: "Start a new empty session (same as /clear)",
		Slash:       true,
		Insert:      "/new",
		Run: func(ctx CommandContext) error {
			if ctx.Host != nil {
				ctx.Host.ClearSession()
			}
			return nil
		},
	})
	r.Register(Command{
		Name:        "theme",
		Description: "Switch theme — /theme <name>",
		Slash:       true,
		Insert:      "/theme ",
		ArgCompleter: func(partial string) []mention.Item {
			return prefixItems(components.ThemeNames(), partial)
		},
		Run: func(ctx CommandContext) error {
			names := components.ThemeNames()
			if len(ctx.Args) != 1 {
				ctx.toast(
					"Usage: /theme <name> — one of: "+strings.Join(names, ", "),
					toast.ToastWarning,
					5*time.Second,
				)
				return nil
			}
			if _, ok := components.ThemeByName(ctx.Args[0]); !ok {
				ctx.toast(
					"Unknown theme "+ctx.Args[0]+" — one of: "+strings.Join(names, ", "),
					toast.ToastError,
					5*time.Second,
				)
				return nil
			}
			if apply := hostFn(ctx, func(h Host) func(string) { return h.ApplyTheme }); apply != nil {
				apply(ctx.Args[0])
			}
			return nil
		},
	})
	r.Register(Command{
		Name:        "export",
		Description: "Export the transcript as markdown — /export [path]",
		Slash:       true,
		Insert:      "/export ",
		Run: func(ctx CommandContext) error {
			path := ""
			if len(ctx.Args) > 0 {
				path = ctx.Args[0]
			}
			if ctx.Host != nil {
				ctx.Host.ExportSession(path)
			}
			return nil
		},
	})

	r.Register(Command{
		Name:        "compact",
		Description: "Summarize the session history to free context",
		Slash:       true,
		Insert:      "/compact",
		Run: func(ctx CommandContext) error {
			if ctx.Host != nil {
				ctx.Host.RunCompact()
			}
			return nil
		},
	})
	r.Register(Command{
		Name:        "connect",
		Description: "Connect an API provider",
		Slash:       true,
		Insert:      "/connect",
		Run: func(ctx CommandContext) error {
			if ctx.Host != nil {
				ctx.Host.ConnectProvider()
			}
			return nil
		},
	})

	r.Register(Command{
		Name: "settings-model",
		PaletteRoot: func(ctx CommandContext) palette.PaletteCommand {
			setModel := hostFn(ctx, func(h Host) func(string) { return h.SetModel })
			names := hostFn(ctx, Host.ModelNames)
			return modelSettingsCommand(setModel, names)
		},
	})
	r.Register(Command{
		Name: "settings-theme",
		PaletteRoot: func(ctx CommandContext) palette.PaletteCommand {
			apply := hostFn(ctx, func(h Host) func(string) { return h.ApplyTheme })
			return ThemeCommand(apply)
		},
	})
	r.Register(Command{
		Name: "settings-permissions",
		PaletteRoot: func(ctx CommandContext) palette.PaletteCommand {
			set := hostFn(ctx, func(h Host) func(bool) { return h.SetPermissions })
			return PermissionsCommand(set)
		},
	})
	r.Register(Command{
		Name: "settings-agents",
		PaletteRoot: func(ctx CommandContext) palette.PaletteCommand {
			set := hostFn(ctx, func(h Host) func(bool) { return h.SetAgents })
			return AgentsCommand(set)
		},
	})
	r.Register(Command{
		Name: "hooks",
		PaletteRoot: func(ctx CommandContext) palette.PaletteCommand {
			list := hostFn(ctx, func(h Host) func() []palette.PaletteCommand { return h.ListHooks })
			reload := hostFn(ctx, func(h Host) func() { return h.ReloadHooks })
			push := hostFn(ctx, func(h Host) func(string, []palette.PaletteCommand) { return h.PushSubmenu })
			return HooksCommand(list, reload, push)
		},
	})
	r.Register(Command{
		Name: "skills",
		PaletteRoot: func(ctx CommandContext) palette.PaletteCommand {
			path := hostFn(ctx, Host.SkillPath)
			add := hostFn(ctx, func(h Host) func(string) { return h.AddSkill })
			return SkillsCommand(path, add)
		},
	})
	r.Register(Command{
		Name: "clipboard-copy-last",
		PaletteRoot: func(ctx CommandContext) palette.PaletteCommand {
			return palette.PaletteCommand{
				ID:       "clipboard-copy-last",
				Noun:     "clipboard",
				Verb:     "copy last message",
				Keywords: []string{"yank", "selection"},
				Shortcut: "Ctrl+Shift+C",
				Run: func() {
					if ctx.Host != nil {
						ctx.Host.CopyLastMessage()
					}
				},
			}
		},
	})
}

// FilterSlashCommands returns commands whose name starts with query (case-insensitive).
// Prefer CommandRegistry.FilterSlash when a registry is available.
func FilterSlashCommands(query string) []mention.Item {
	return NewBuiltinRegistry().FilterSlash(query)
}

// ModelSlashCommand builds the /model command for a configured model list.
// Registered by the editor assembly, which knows the names; SetModel comes
// from the command Host at run time.
func ModelSlashCommand(names []string) Command {
	return Command{
		Name:        "model",
		Description: "Switch model — /model <name>",
		Slash:       true,
		Insert:      "/model ",
		ArgCompleter: func(partial string) []mention.Item {
			return prefixItems(names, partial)
		},
		Run: func(ctx CommandContext) error {
			if len(ctx.Args) != 1 {
				ctx.toast("Usage: /model <name>", toast.ToastWarning, 3*time.Second)
				return nil
			}
			for _, n := range names {
				if strings.EqualFold(n, ctx.Args[0]) {
					if set := hostFn(ctx, func(h Host) func(string) { return h.SetModel }); set != nil {
						set(n)
					}
					return nil
				}
			}
			ctx.toast("Unknown model "+ctx.Args[0], toast.ToastError, 3*time.Second)
			return nil
		},
	}
}

// prefixItems filters values by case-insensitive prefix into mention items
// (arg completion for commands whose first argument is one of a fixed set).
func prefixItems(values []string, partial string) []mention.Item {
	q := strings.ToLower(partial)
	out := make([]mention.Item, 0, len(values))
	for _, v := range values {
		if q == "" || strings.HasPrefix(strings.ToLower(v), q) {
			out = append(out, mention.Item{Path: v})
		}
	}
	return out
}

// LookupSlashInsert returns the Insert string for a command name, or empty.
func LookupSlashInsert(name string) string {
	return NewBuiltinRegistry().LookupInsert(name)
}

// modelSettingsCommand returns settings → model submenu.
func modelSettingsCommand(onModel func(name string), modelNames []string) palette.PaletteCommand {
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
	return palette.PaletteCommand{
		ID:           "settings-model",
		Noun:         "settings",
		Verb:         "model",
		Keywords:     []string{"model"},
		SubmenuTitle: "Select Model",
		Submenu:      models,
	}
}

// PaletteCommands returns model-switch commands for the command palette
// (legacy helper; prefer registry BuildPalette).
func PaletteCommands(onModel func(name string), modelNames []string) []palette.PaletteCommand {
	return []palette.PaletteCommand{modelSettingsCommand(onModel, modelNames)}
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
// push opens a nested list page (shell owns *CommandPalette; commands do not).
func HooksCommand(
	listFn func() []palette.PaletteCommand,
	reload func(),
	push func(title string, cmds []palette.PaletteCommand),
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
					if push != nil {
						push("Hooks on disk", cmds)
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
