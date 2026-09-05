package keys

import (
	"fmt"
	"maps"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/editmode"
)

var (
	activeProfile editmode.Mode
	userBinds     map[string]string
)

// CheckProfile validates before a preference is saved or the live table changes.
func CheckProfile(mode editmode.Mode) error {
	_, err := compileProfile(userBinds, mode)
	return err
}

// SetProfile runs on the UI goroutine together with rendering and dispatch.
func SetProfile(mode editmode.Mode) error {
	t, err := compileProfile(userBinds, mode)
	if err != nil {
		return err
	}
	table, activeProfile = t, mode
	return nil
}

func profileDefaults(mode editmode.Mode) map[Command]string {
	binds := maps.Clone(defaultBinds)
	if mode == editmode.Readline {
		binds[CmdPalette] = "F2"
		binds[CmdPlanEditor] = "F3"
		binds[CmdWatches] = "F4"
		binds[CmdVerbose] = "Shift+F6"
		binds[CmdPlanApprove] = "F7"
		binds[CmdPlanDetails] = "F8"
	}
	return binds
}

func checkProfileChord(mode editmode.Mode, cmd Command, c Chord) error {
	if c.code != xui.KeyRune {
		return nil
	}
	if (c.mods == xui.ModCtrl && strings.ContainsRune("zy", c.r)) ||
		(c.mods == xui.ModCtrl|xui.ModShift && c.r == 'z') {
		return fmt.Errorf("keybinds: %s for %s is reserved by input undo/yank; choose another chord", c, cmd)
	}
	if mode == editmode.Readline &&
		((c.mods == xui.ModCtrl && strings.ContainsRune("abdefhknpuw", c.r)) ||
			(c.mods == xui.ModAlt && strings.ContainsRune("bdf", c.r))) {
		return fmt.Errorf(
			"keybinds: %s for %s is reserved by readline editing; rebind %s before switching",
			c,
			cmd,
			cmd,
		)
	}
	return nil
}

func profileGroup(g Group) Group {
	if g.Scope == ScopeComposer {
		g.Note = "Use /keymap to choose standard, readline or vim. The choice is saved."
		g.Bindings = append([]Binding(nil), g.Bindings...)
		g.Bindings = append(g.Bindings,
			Binding{Keys: []string{"Ctrl+Z", "Ctrl+Shift+Z"}, Desc: "undo or redo an input edit"},
		)
		if activeProfile == editmode.Readline {
			for i := range g.Bindings {
				if g.Bindings[i].Label() == "Ctrl+A" {
					g.Bindings[i].Desc = "jump to the beginning of the logical line (Cmd+A selects all)"
				}
				if g.Bindings[i].Label() == "Ctrl+U" {
					g.Bindings[i].Desc = "kill the text before the caret; Ctrl+Y restores it"
				}
			}
			g.Bindings = append(
				g.Bindings,
				Binding{Keys: []string{"Ctrl+E"}, Desc: "jump to the end of the logical line"},
				Binding{Keys: []string{"Ctrl+B", "Ctrl+F"}, Desc: "move one character backward or forward"},
				Binding{Keys: []string{"Alt+B", "Alt+F"}, Desc: "move one word backward or forward"},
				Binding{Keys: []string{"Ctrl+P", "Ctrl+N"}, Desc: "move a line; at the edges walk prompt history"},
				Binding{
					Keys: []string{"Ctrl+K", "Ctrl+W", "Alt+D"},
					Desc: "kill to line end, previous whitespace word, or next word",
				},
				Binding{Keys: []string{"Ctrl+Y"}, Desc: "insert the last killed text"},
				Binding{Keys: []string{"Ctrl+D", "Ctrl+H"}, Desc: "delete the next or previous character"},
			)
		}
		if activeProfile == editmode.Vim {
			g.Note += " Vim starts in INSERT; NORMAL never sends on Enter. Ctrl+C still interrupts."
			g.Bindings = append(
				g.Bindings,
				Binding{Keys: []string{"Esc"}, Desc: "INSERT to NORMAL; a picker or voice dialog closes first"},
				Binding{
					Keys: []string{"i", "a", "I", "A"},
					Desc: "NORMAL: insert before/after caret or at line start/end",
				},
				Binding{
					Keys: []string{"h/j/k/l", "w/b", "0/^/$", "gg/G"},
					Desc: "NORMAL: move by character, line, word or document",
				},
				Binding{
					Keys: []string{"x", "dd", "dw", "D"},
					Desc: "NORMAL: delete character, line, word or to line end",
				},
				Binding{
					Keys: []string{"cc", "cw", "C", "o/O"},
					Desc: "NORMAL: change line/word/rest, or open a line below/above",
				},
				Binding{Keys: []string{"yy", "yw", "y$", "p"}, Desc: "NORMAL: yank line/word/rest, or put after caret"},
				Binding{Keys: []string{"u", "Ctrl+R"}, Desc: "NORMAL: undo or redo"},
			)
		}
	}
	if g.Scope == ScopeGlobal && activeProfile == editmode.Vim {
		g.Note += " In Vim input, Esc changes editing mode; Ctrl+C interrupts."
		g.Bindings = append([]Binding(nil), g.Bindings...)
		for i := range g.Bindings {
			if g.Bindings[i].Label() == "Esc" {
				g.Bindings[i].Desc = "close a picker/dialog; in Vim input enter NORMAL (Ctrl+C interrupts work)"
			}
		}
	}
	return g
}
