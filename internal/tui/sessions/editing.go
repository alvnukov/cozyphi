package sessions

import (
	"errors"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/components/mention"
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/editmode"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

func (e *View) configureEditing() {
	if e.ctrl != nil {
		mode, err := e.ctrl.EditingMode()
		if err == nil {
			err = keys.CheckProfile(mode)
		}
		if err != nil {
			e.Toast("Cannot load keymap: "+err.Error(), toast.ToastWarning, 6*time.Second)
		} else {
			e.composer.Chat.SetEditingMode(mode)
		}
	}
	e.composer.Chat.KeyHints = func(width int) string {
		if width < 30 {
			return keys.Label(keys.CmdHelp) + " help"
		}
		if width < 70 {
			return "Enter send · " + keys.Label(keys.CmdHelp) + " help"
		}
		return "Enter send · Alt+Enter newline · " + keys.Label(keys.CmdHelp) + " help · " +
			keys.Label(keys.CmdPalette) + " commands"
	}
	e.commands.Register(commands.Command{
		Name: "keymap", Description: "Choose input style: standard, Bash/Readline or Vim",
		Slash: true, Insert: "/keymap",
		ArgCompleter: func(args []string, partial string) []mention.Item {
			if len(args) != 0 {
				return nil
			}
			var items []mention.Item
			for _, name := range []string{"standard", "readline", "vim"} {
				if strings.HasPrefix(name, strings.ToLower(partial)) {
					items = append(items, mention.Item{Path: name})
				}
			}
			return items
		},
		Run: func(ctx commands.CommandContext) error {
			if len(ctx.Args) == 0 {
				e.PushSubmenu("Input style", e.editingChoices())
				return nil
			}
			if len(ctx.Args) != 1 {
				return errors.New("usage: /keymap [standard|readline|vim]")
			}
			mode, err := editmode.Parse(ctx.Args[0])
			if err != nil {
				return err
			}
			return e.applyEditingMode(mode)
		},
		PaletteRoot: func(commands.CommandContext) palette.PaletteCommand {
			return palette.PaletteCommand{
				ID: "keymap", Noun: "input", Verb: "style: " + e.composer.Chat.EditingMode().String(),
				Keywords:     []string{"keymap", "vim", "bash", "readline", "emacs", "keyboard", "editing"},
				SubmenuTitle: "Input style", Submenu: e.editingChoices(),
			}
		},
	})
}

func (e *View) editingChoices() []palette.PaletteCommand {
	choices := make([]palette.PaletteCommand, 0, 3)
	for _, mode := range []editmode.Mode{editmode.Standard, editmode.Readline, editmode.Vim} {
		label := mode.String()
		if mode == e.composer.Chat.EditingMode() {
			label += " (active)"
		}
		choices = append(choices, palette.PaletteCommand{
			ID: "keymap-" + mode.String(), Verb: label,
			Run: func() {
				if err := e.applyEditingMode(mode); err != nil {
					e.Toast(err.Error(), toast.ToastError, 6*time.Second)
				}
			},
		})
	}
	return choices
}

func (e *View) applyEditingMode(mode editmode.Mode) error {
	if err := keys.CheckProfile(mode); err != nil {
		return err
	}
	if e.ctrl != nil {
		if err := e.ctrl.SaveEditingMode(mode); err != nil {
			return err
		}
	}
	if e.Active() {
		if err := keys.SetProfile(mode); err != nil {
			return err
		}
	}
	e.composer.HideCompleters()
	e.composer.Chat.SetEditingMode(mode)
	e.Toast("Input: "+mode.String()+" · "+keys.Label(keys.CmdHelp)+" help", toast.ToastSuccess, 3*time.Second)
	e.RequestRedraw()
	return nil
}
