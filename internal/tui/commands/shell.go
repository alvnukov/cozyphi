package commands

import (
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

func registerShellCommands(r *CommandRegistry) {
	open := func(ctx CommandContext) error {
		if ctx.Host != nil {
			ctx.Host.ShowShellTasks()
		}
		return nil
	}
	for _, name := range []string{"tasks", "bashes"} {
		cmd := Command{
			Name:        name,
			Description: "Browse shell tasks — live output, background, stop",
			Slash:       true,
			Run:         open,
		}
		if name == "tasks" {
			cmd.PaletteRoot = func(ctx CommandContext) palette.PaletteCommand {
				return palette.PaletteCommand{
					ID: "shell-tasks", Noun: "shell tasks", Verb: "browse",
					Keywords: []string{"bash", "background", "output", "stop"},
					Run: func() {
						if ctx.Host != nil {
							ctx.Host.ShowShellTasks()
						}
					},
				}
			}
		}
		r.Register(cmd)
	}
	r.Register(Command{
		Name:        "background-shell",
		Description: "Move a running shell command to the background",
		PaletteRoot: func(ctx CommandContext) palette.PaletteCommand {
			return palette.PaletteCommand{
				ID: "background-shell", Noun: "shell", Verb: "background",
				Keywords: []string{"bash", "running", "detach"},
				Shortcut: keys.Label(keys.CmdBackgroundShell),
				Run: func() {
					if ctx.Host != nil {
						ctx.Host.BackgroundShell()
					}
				},
			}
		},
	})
}
