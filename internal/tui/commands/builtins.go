package commands

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/mention"
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm/skills"
	"github.com/alvnukov/cozyphi/internal/usage"
)

// NewBuiltinRegistry returns the built-in slash + palette catalog.
func NewBuiltinRegistry(histories ...*usage.Store) *CommandRegistry {
	r := NewCommandRegistry(histories...)
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
		Name:        "context",
		Description: "Browse the model context — inspect, trim, compact",
		Slash:       true,
		Insert:      "/context",
		Run: func(ctx CommandContext) error {
			if ctx.Host != nil {
				ctx.Host.ShowContext()
			}
			return nil
		},
	})
	r.Register(Command{
		Name:        "settings",
		Description: "Open harness settings",
		Slash:       true,
		Insert:      "/settings",
		Run: func(ctx CommandContext) error {
			if ctx.Host != nil {
				ctx.Host.ShowSettings()
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
				return usagef("usage: /resume <session-id>")
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
				return usagef("usage: /theme <name> — one of: %s", strings.Join(names, ", "))
			}
			if _, ok := components.ThemeByName(ctx.Args[0]); !ok {
				return fmt.Errorf("unknown theme %q — one of: %s", ctx.Args[0], strings.Join(names, ", "))
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
			setModel := hostFn(ctx, func(h Host) func(string) error { return h.SetModel })
			onModel := setModel
			if setModel != nil {
				// Palette rows have no error channel, so this wrapper keeps
				// the one-toast rule for them; slash dispatch toasts on the
				// dispatcher and needs no wrapper.
				onModel = func(name string) error {
					if err := setModel(name); err != nil {
						ctx.toast(err.Error(), toast.ToastError, 5*time.Second)
						return err
					}
					return nil
				}
			}
			names := hostFn(ctx, Host.ModelNames)
			return modelSettingsCommand(onModel, names, r.history)
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
		Name: "settings-harness",
		PaletteRoot: func(ctx CommandContext) palette.PaletteCommand {
			return palette.PaletteCommand{
				ID:       "harness-settings",
				Noun:     "settings",
				Verb:     "harness",
				Keywords: []string{"plan", "defaults", "config", "policy"},
				Shortcut: "Ctrl+,",
				Run: func() {
					if ctx.Host != nil {
						ctx.Host.ShowSettings()
					}
				},
			}
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
			return SkillsCommand(path, add, r.history)
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

// ModelSlashCommand builds the /model command for a configured model list.
// Registered by the editor assembly, which knows the names; SetModel comes
// from the command Host at run time.
func ModelSlashCommand(names []string, histories ...*usage.Store) Command {
	var history *usage.Store
	if len(histories) > 0 {
		history = histories[0]
	}
	rankedNames := func() []string {
		return usage.Rank(history, usage.Models, names, func(name string) string { return name })
	}
	return Command{
		Name:        "model",
		Description: "Switch model — /model <name>",
		Slash:       true,
		Insert:      "/model ",
		ArgCompleter: func(partial string) []mention.Item {
			return prefixItems(rankedNames(), partial)
		},
		Run: func(ctx CommandContext) error {
			if len(ctx.Args) != 1 {
				return usagef("usage: /model <name>")
			}
			for _, name := range names {
				if !strings.EqualFold(name, ctx.Args[0]) {
					continue
				}
				set := hostFn(ctx, func(h Host) func(string) error { return h.SetModel })
				if set == nil {
					return errors.New("model host is unavailable")
				}
				if err := set(name); err != nil {
					return err
				}
				_ = history.Record(usage.Models, name)
				return nil
			}
			return fmt.Errorf("unknown model %q", ctx.Args[0])
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

// modelSettingsCommand returns settings → model submenu.
func modelSettingsCommand(
	onModel func(name string) error,
	modelNames []string,
	history *usage.Store,
) palette.PaletteCommand {
	modelNames = usage.Rank(history, usage.Models, modelNames, func(name string) string { return name })
	models := make([]palette.PaletteCommand, 0, len(modelNames))
	for _, name := range modelNames {
		models = append(models, palette.PaletteCommand{
			ID:   "model-" + name,
			Verb: name,
			Run: func() {
				if onModel != nil && onModel(name) == nil {
					_ = history.Record(usage.Models, name)
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
func SkillsCommand(skillPath string, add func(name string), histories ...*usage.Store) palette.PaletteCommand {
	var history *usage.Store
	if len(histories) > 0 {
		history = histories[0]
	}
	submenu := skillSubcommands(skillPath, add, history)
	return palette.PaletteCommand{
		ID:           "skills",
		Noun:         "skills",
		Verb:         "invoke",
		Keywords:     []string{"skill", "use skill", "load skill", "pending"},
		SubmenuTitle: "Select skill",
		Submenu:      submenu,
	}
}

func skillSubcommands(skillPath string, add func(name string), history *usage.Store) []palette.PaletteCommand {
	list, err := skills.LoadSkills(skillPath)
	if err != nil || len(list) == 0 {
		return []palette.PaletteCommand{{
			ID:       "skills-empty",
			Verb:     "No skills found",
			Disabled: true,
		}}
	}

	list = usage.Rank(history, usage.Skills, list, skillName)
	out := make([]palette.PaletteCommand, 0, len(list))
	for _, skill := range list {
		name := skillName(skill)
		out = append(out, palette.PaletteCommand{
			ID:       "skill-" + name,
			Verb:     name,
			Keywords: []string{skill.Description, "skill"},
			Run: func() {
				if add != nil {
					add(name)
				}
			},
		})
	}
	return out
}

func skillName(skill *skills.Skill) string {
	if skill == nil {
		return ""
	}
	if name := strings.TrimSpace(skill.Name); name != "" {
		return name
	}
	return strings.TrimSpace(skill.Path)
}
