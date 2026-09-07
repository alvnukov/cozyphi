package keys

import (
	"fmt"
	"maps"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/editmode"
)

// Command identifies one rebindable chat-view action. The id is what a
// config `keybinds` entry names, so it is part of the config surface: rename
// one and somebody's override breaks.
type Command string

// The rebindable commands, in dispatch and conflict-report order.
const (
	CmdHelp          Command = "help"
	CmdPalette       Command = "palette"
	CmdSettings      Command = "settings"
	CmdEffort        Command = "effort"
	CmdKeymap        Command = "keymap"
	CmdPlanEditor    Command = "plan-editor"
	CmdPlanFocus     Command = "plan-focus"
	CmdSidebarToggle Command = "sidebar-toggle"
	CmdPlanApprove   Command = "plan-approve"
	CmdPlanDetails   Command = "plan-details"
	CmdWatches       Command = "watches"
	CmdCopyLast      Command = "copy-last"
	CmdVerbose       Command = "transcript-verbose"
	CmdVoice         Command = "voice"
	CmdSessionNext   Command = "session-next"
	CmdSessionPrev   Command = "session-prev"
	CmdSessionBack   Command = "session-back"

	// The composer's reverse-i-search chords; forward only applies mid-search.
	CmdHistorySearch    Command = "history-search"
	CmdHistorySearchFwd Command = "history-search-forward"
)

// commands fixes the iteration order: deterministic conflict messages, and a
// stable order for GlobalCommand (which unique chords make order-free anyway).
var commands = []Command{
	CmdHelp, CmdPalette, CmdSettings, CmdEffort, CmdKeymap, CmdPlanEditor, CmdPlanFocus,
	CmdSidebarToggle, CmdPlanApprove, CmdPlanDetails, CmdWatches, CmdCopyLast, CmdVerbose,
	CmdVoice, CmdHistorySearch, CmdHistorySearchFwd,
	CmdSessionNext, CmdSessionPrev, CmdSessionBack,
}

// defaultBinds is each command's default spelling. A comma separates
// interchangeable chords for one command.
var defaultBinds = map[Command]string{
	CmdHelp:          "F1",
	CmdPalette:       "Ctrl+K",
	CmdSettings:      "Ctrl+,",
	CmdEffort:        "F5",
	CmdKeymap:        "F6",
	CmdPlanEditor:    "Ctrl+P",
	CmdPlanFocus:     "Alt+P",
	CmdSidebarToggle: "Ctrl+O",
	CmdPlanApprove:   "Ctrl+A",
	CmdPlanDetails:   "Ctrl+D",
	CmdWatches:       "Ctrl+W",
	CmdCopyLast:      "Ctrl+Shift+C, Cmd+C",
	CmdVerbose:       "Ctrl+E",
	CmdVoice:         "Ctrl+G",
	CmdSessionNext:   "Ctrl+F10",
	CmdSessionPrev:   "Shift+F10",
	CmdSessionBack:   "Alt+F10",

	CmdHistorySearch:    "Ctrl+R",
	CmdHistorySearchFwd: "Ctrl+S",
}

// table is the active binding table. Rebind initializes it at boot; SetProfile
// swaps it on the UI goroutine, which also owns dispatch and rendering.
var table = mustCompile(nil)

func mustCompile(overrides map[string]string) map[Command][]Chord {
	t, err := compile(overrides)
	if err != nil {
		panic(err)
	}
	return t
}

// compile resolves defaults plus overrides into chords, rejecting an unknown
// command id, a malformed spelling, and two commands on one chord. The value
// "none" unbinds a command: its chord is freed and its catalog rows disappear.
func compile(overrides map[string]string) (map[Command][]Chord, error) {
	return compileProfile(overrides, editmode.Standard)
}

func compileProfile(overrides map[string]string, mode editmode.Mode) (map[Command][]Chord, error) {
	if mode != editmode.Standard && mode != editmode.Readline && mode != editmode.Vim {
		return nil, fmt.Errorf("unknown keymap %q: choose standard, readline or vim", mode)
	}
	binds := profileDefaults(mode)
	for id, spec := range overrides {
		if _, ok := defaultBinds[Command(id)]; !ok {
			return nil, fmt.Errorf("keybinds: unknown command %q (commands: %s)",
				id, commandList())
		}
		binds[Command(id)] = spec
	}
	out := make(map[Command][]Chord, len(binds))
	owner := make(map[Chord]Command)
	for _, cmd := range commands {
		chords, err := parseChordList(binds[cmd])
		if err != nil {
			return nil, fmt.Errorf("keybinds: %s: %w", cmd, err)
		}
		for _, c := range chords {
			if err := checkProfileChord(mode, cmd, c); err != nil {
				return nil, err
			}
			if prev, dup := owner[c]; dup {
				return nil, fmt.Errorf("keybinds: %s is bound to both %s and %s",
					c, prev, cmd)
			}
			owner[c] = cmd
		}
		out[cmd] = chords
	}
	return out, nil
}

// parseChordList parses a comma-separated chord spec; "none" is the explicit
// empty list. A piece ending in "+" is a chord whose key IS the comma
// ("Ctrl+,"), cut in half by the separator — it is joined back together.
func parseChordList(spec string) ([]Chord, error) {
	if strings.EqualFold(strings.TrimSpace(spec), "none") {
		return nil, nil
	}
	parts := strings.Split(spec, ",")
	chords := make([]Chord, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		p := strings.TrimSpace(parts[i])
		if strings.HasSuffix(p, "+") && i+1 < len(parts) {
			p += "," + strings.TrimSpace(parts[i+1])
			i++
		}
		c, err := ParseChord(p)
		if err != nil {
			return nil, err
		}
		chords = append(chords, c)
	}
	return chords, nil
}

func commandList() string {
	ids := make([]string, len(commands))
	for i, cmd := range commands {
		ids[i] = string(cmd)
	}
	return strings.Join(ids, ", ")
}

// CheckBinds validates a keybinds section without applying it — the config
// loader's half of Rebind, so a broken override fails the load instead of
// surfacing as a dead key later.
func CheckBinds(overrides map[string]string) error {
	_, err := compile(overrides)
	return err
}

// Rebind applies keybind overrides on top of the defaults; nil restores the
// defaults. It runs once at boot, before any pane renders a chord, so the
// catalog, the palette and the dispatch can never disagree about a spelling.
func Rebind(overrides map[string]string) error {
	t, err := compile(overrides)
	if err != nil {
		return err
	}
	table = t
	userBinds = maps.Clone(overrides)
	activeProfile = editmode.Standard
	return nil
}

// GlobalCommand resolves a key event against the table: which rebindable
// command, if any, this press is. Chords are unique across the table, so at
// most one command can claim an event.
func GlobalCommand(ev xui.KeyEvent) (Command, bool) {
	for _, cmd := range commands {
		for _, c := range table[cmd] {
			if c.Match(ev) {
				return cmd, true
			}
		}
	}
	return "", false
}

// Is reports whether the event is one of cmd's chords — for the handler that
// owns a command's action away from the editor's dispatch (the palette lives
// in the composer's flow).
func Is(ev xui.KeyEvent, cmd Command) bool {
	for _, c := range table[cmd] {
		if c.Match(ev) {
			return true
		}
	}
	return false
}

// Label is the command's first chord spelling, or "" when unbound — the form
// a palette row or a group title wants.
func Label(cmd Command) string {
	if chords := table[cmd]; len(chords) > 0 {
		return chords[0].String()
	}
	return ""
}

// labelsOf lists every current spelling of cmd, for catalog rows.
func labelsOf(cmd Command) []string {
	chords := table[cmd]
	if len(chords) == 0 {
		return nil
	}
	out := make([]string, len(chords))
	for i, c := range chords {
		out[i] = c.String()
	}
	return out
}
