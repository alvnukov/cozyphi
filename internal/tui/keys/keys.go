// Package keys is cozyphi's key-binding catalog: one declarative table that
// both the /help screen and the panes' footer hint rows read. A binding is
// written down once, so the row a pane advertises cannot drift from the row
// the help screen shows.
//
// The rebindable global chords additionally live in a binding table
// (table.go): command id → chord, defaults overridden by the config's
// `keybinds` section. The editor dispatches those chords through the table
// (GlobalCommand), and the catalog renders the table's current spellings, so
// an override changes the behavior, the footer and the help screen together.
//
// The catalog owns the standing hint rows. Transient prompts — "delete 3
// blocks? y confirm · n cancel" — stay local to the pane that asks them.
package keys

import (
	"strings"
)

// Separator joins hint fragments in a footer row.
const Separator = " · "

// Scope names one keyboard context: a pane, a modal, or the chat view as a
// whole. A footer row renders a single scope; the help screen renders all.
type Scope string

// The scopes, in the order the help screen shows them.
const (
	ScopeGlobal        Scope = "global"
	ScopeComposer      Scope = "composer"
	ScopeTranscript    Scope = "transcript"
	ScopeSidebar       Scope = "sidebar"
	ScopePlanFocus     Scope = "plan-focus"
	ScopePlanPicker    Scope = "plan-picker"
	ScopeAsk           Scope = "ask"
	ScopeAskDetail     Scope = "ask-detail"
	ScopeContinue      Scope = "continue"
	ScopeQuestion      Scope = "question"
	ScopeAnswer        Scope = "answer"
	ScopeConnect       Scope = "connect"
	ScopeConnectMethod Scope = "connect-method"
	ScopeConnectKey    Scope = "connect-key"
	ScopeContext       Scope = "context"
	ScopeContextRaw    Scope = "context-block"
	ScopeWatches       Scope = "watches"
	ScopeWatchLog      Scope = "watches-log"
	ScopePlan          Scope = "plan"
	ScopePlanDetail    Scope = "plan-detail"
	ScopePlanText      Scope = "plan-text"
	ScopeJump          Scope = "jump"
	ScopeMenu          Scope = "menu"
	ScopePlanChoice    Scope = "plan-choice"
	ScopePlanSkills    Scope = "plan-skills"
	ScopeSettings      Scope = "settings"
	ScopeHelp          Scope = "help"
)

// Binding is one key — or a set of interchangeable keys — and what it does.
// Hint is the terse footer wording ("apply"), Desc the help-screen sentence.
// An empty Hint keeps a binding out of footer rows; an empty Desc keeps it
// out of the help screen. A binding for a rebindable command names the
// command instead of spelling keys: Cmd set and Keys empty means the current
// chords come from the binding table, overrides included, so a footer can
// never advertise a chord the dispatch no longer answers.
type Binding struct {
	Keys []string
	Cmd  Command
	Hint string
	Desc string
}

// Label joins the interchangeable spellings for display: "Shift/Ctrl+Enter".
func (b Binding) Label() string { return strings.Join(b.Keys, "/") }

// Group is one scope's bindings, in the order they should be shown.
type Group struct {
	Scope Scope
	Title string
	// TitleCmd appends the command's current chord to the title —
	// "Settings (Ctrl+,)" — so a retitled chord retitles the group too.
	TitleCmd Command
	// Note is an optional line under the title: what the scope covers, or a
	// platform caveat.
	Note     string
	Bindings []Binding
}

// Groups returns the whole catalog in display order, with every
// command-bound row carrying the binding table's current spellings.
func Groups() []Group {
	out := make([]Group, len(catalog))
	for i, g := range catalog {
		out[i] = displayGroup(g)
	}
	return out
}

// Find returns one scope's group, materialized like Groups.
func Find(s Scope) (Group, bool) {
	for _, g := range catalog {
		if g.Scope == s {
			return displayGroup(g), true
		}
	}
	return Group{}, false
}

// displayGroup resolves a group's command-bound rows against the table. A
// row whose command is unbound is dropped — no key, nothing to document —
// which keeps an unbound chord out of footers and help alike.
func displayGroup(g Group) Group {
	bindings := make([]Binding, 0, len(g.Bindings))
	for _, b := range g.Bindings {
		if b.Cmd != "" {
			b.Keys = labelsOf(b.Cmd)
			if len(b.Keys) == 0 {
				continue
			}
		}
		bindings = append(bindings, b)
	}
	g.Bindings = bindings
	if g.TitleCmd != "" {
		if label := Label(g.TitleCmd); label != "" {
			g.Title += " (" + label + ")"
		}
	}
	return g
}

// Hints renders a scope's one-line hint row: "↑↓ select · Enter open · Esc
// close". Bindings without a Hint are left out.
func Hints(s Scope) string {
	g, ok := Find(s)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(g.Bindings))
	for _, b := range g.Bindings {
		if b.Hint == "" {
			continue
		}
		if label := b.Label(); label != "" {
			parts = append(parts, label+" "+b.Hint)
			continue
		}
		parts = append(parts, b.Hint)
	}
	return strings.Join(parts, Separator)
}

// Footer is Hints padded with one space on each side, the shape a border
// label or a bottom row wants.
func Footer(s Scope) string {
	h := Hints(s)
	if h == "" {
		return ""
	}
	return " " + h + " "
}

var catalog = []Group{
	{
		Scope: ScopeGlobal,
		Title: "Anywhere",
		Note:  "Work from the chat view whatever has focus. On macOS Cmd stands in for Ctrl.",
		Bindings: []Binding{
			{Cmd: CmdHelp, Desc: "open this help screen — /help does the same"},
			{Cmd: CmdPalette, Desc: "open the command palette"},
			{Cmd: CmdSettings, Desc: "open settings"},
			{Cmd: CmdPlanEditor, Desc: "open the plan viewer and editor"},
			{Cmd: CmdPlanFocus, Desc: "move focus to the plan in the sidebar"},
			{Cmd: CmdWatches, Desc: "open the watch browser — /watches does the same"},
			{Cmd: CmdSidebarToggle, Desc: "show or hide the sidebar"},
			{Keys: []string{"Tab"}, Desc: "switch the permission mode"},
			{Keys: []string{"Ctrl+C"}, Desc: "interrupt the run; pressed twice in a row, quit"},
			{Keys: []string{"Esc"}, Desc: "close the picker, else stop the run, else drop the selection"},
		},
	},
	{
		Scope: ScopeComposer,
		Title: "Message input",
		Bindings: []Binding{
			{Keys: []string{"Enter"}, Desc: "send the message"},
			{Keys: []string{"Shift+Enter", "Alt+Enter", "Ctrl+Enter"}, Desc: "start a new line"},
			{Keys: []string{"↑↓"}, Desc: "move a row; past the last row, walk the prompt history"},
			{Keys: []string{"Home", "End"}, Desc: "jump to the row start or end; Shift extends the selection"},
			{Keys: []string{"Ctrl+Backspace", "Alt+Backspace"}, Desc: "delete the word before the caret"},
			{Keys: []string{"Ctrl+Del", "Alt+Del"}, Desc: "delete the word after the caret"},
			{Keys: []string{"Ctrl+U"}, Desc: "discard the line before the caret"},
			{Keys: []string{"Ctrl+A"}, Desc: "select the whole message"},
			{Keys: []string{"Ctrl+C", "Ctrl+X"}, Desc: "copy or cut the selection"},
			{Keys: []string{"Ctrl+V"}, Desc: "attach the clipboard image; without one, paste text as usual"},
			{Keys: []string{"/"}, Desc: "open the slash-command picker"},
			{Keys: []string{"@"}, Desc: "open the file mention picker"},
		},
	},
	{
		Scope: ScopeTranscript,
		Title: "Transcript",
		Note: "The wheel scrolls; dragging selects text. Turns older than the " +
			"last two fold to a summary row; a click unfolds one.",
		Bindings: []Binding{
			{Keys: []string{"PgUp", "PgDn"}, Desc: "scroll one screen"},
			{Keys: []string{"Shift+PgUp", "Shift+PgDn"}, Desc: "jump to the previous or next turn"},
			{Cmd: CmdVerbose, Desc: "unfold every condensed turn, or fold them back"},
			{Cmd: CmdCopyLast, Desc: "copy the selected block, or the last message"},
		},
	},
	{
		Scope: ScopeSidebar,
		Title: "Sidebar and plan",
		Bindings: []Binding{
			{Cmd: CmdPlanFocus, Hint: "plan", Desc: "move focus into the plan"},
			{Cmd: CmdSidebarToggle, Hint: "hide", Desc: "show or hide the sidebar"},
			{Cmd: CmdPlanApprove, Desc: "approve the plan, or stop an approved one"},
			{Cmd: CmdPlanDetails, Desc: "expand or collapse the step details"},
			{Keys: []string{"Ctrl+↑↓"}, Desc: "scroll the plan one row"},
			{Keys: []string{"Ctrl+PgUp", "Ctrl+PgDn"}, Desc: "scroll the plan one screen"},
		},
	},
	{
		Scope: ScopePlanFocus,
		Title: "Plan, once focused",
		Note: "After Alt+P, plain keys go to the plan until Esc gives them back. " +
			"Moves like every list: counts work (3j, 12G), gg/G jump. " +
			"A letter outside the dialect returns the keyboard to the message input.",
		Bindings: []Binding{
			{Keys: []string{"↑↓", "j/k"}, Desc: "move between steps"},
			{Keys: []string{"gg", "G"}, Desc: "jump to the first or last step"},
			{Keys: []string{"PgUp", "PgDn"}, Desc: "move a screen of steps"},
			{Keys: []string{"Enter", "m"}, Hint: "model", Desc: "open the model picker for the step"},
			{Keys: []string{"Space"}, Desc: "open the model picker, like Enter"},
			{Keys: []string{"Esc"}, Hint: "back", Desc: "hand the keyboard back to the message input"},
		},
	},
	{
		Scope: ScopePlanPicker,
		Title: "Model picker in the sidebar",
		Note: "The arrows wrap at the edges; page keys clamp. A letter outside " +
			"the dialect abandons the picker for the message input, key included.",
		Bindings: []Binding{
			{Keys: []string{"↑↓", "j/k"}, Desc: "move between models, wrapping at the edges"},
			{Keys: []string{"g", "G"}, Desc: "jump to the first or last entry"},
			{Keys: []string{"PgUp", "PgDn"}, Desc: "move a screenful, stopping at the edge"},
			{Keys: []string{"Enter"}, Hint: "pick", Desc: "commit the highlighted model to the step"},
			{Keys: []string{"Space"}, Desc: "commit the highlighted model, like Enter"},
			{Keys: []string{"Esc"}, Hint: "back", Desc: "close the picker and keep the step's model"},
		},
	},
	{
		Scope: ScopeAsk,
		Title: "Permission ask",
		Note:  "The options wrap: stepping past the last lands on the first.",
		Bindings: []Binding{
			{Keys: []string{"1-9"}, Desc: "take the numbered option"},
			{Keys: []string{"y", "n"}, Desc: "allow or deny"},
			{Keys: []string{"↑↓"}, Hint: "move", Desc: "move between options"},
			{Keys: []string{"Enter"}, Hint: "select", Desc: "take the highlighted option"},
			{Keys: []string{"Esc"}, Hint: "deny", Desc: "deny the request"},
			{Keys: []string{"j/k"}, Desc: "move between options, like ↑↓"},
			{Keys: []string{"Space"}, Desc: "take the highlighted option, like Enter"},
			{Keys: []string{"Tab"}, Desc: "move down, like ↓"},
			{Keys: []string{"v"}, Desc: "expand the clipped request detail"},
		},
	},
	{
		Scope: ScopeAskDetail,
		Title: "Permission ask — expanded detail",
		Note:  "Digits, y/n, Enter and Space still answer while the detail is open.",
		Bindings: []Binding{
			{Keys: []string{"↑↓"}, Hint: "scroll", Desc: "scroll the detail"},
			{Keys: []string{"v"}, Hint: "collapse", Desc: "fold the detail back down"},
			{Keys: []string{"Esc"}, Desc: "fold the detail back down, keeping the ask"},
			{Keys: []string{"j/k"}, Desc: "scroll the detail, like ↑↓"},
		},
	},
	{
		Scope: ScopeContinue,
		Title: "Continue prompt",
		Note:  "Shown when a run reaches its step budget. The options wrap.",
		Bindings: []Binding{
			{Keys: []string{"1-9"}, Desc: "take the numbered option"},
			{Keys: []string{"y", "n"}, Desc: "continue or stop"},
			{Keys: []string{"↑↓"}, Hint: "move", Desc: "move between options"},
			{Keys: []string{"Enter"}, Hint: "select", Desc: "take the highlighted option"},
			{Keys: []string{"Esc"}, Hint: "stop", Desc: "stop the run here"},
			{Keys: []string{"j/k"}, Desc: "move between options, like ↑↓"},
			{Keys: []string{"Space"}, Desc: "take the highlighted option, like Enter"},
			{Keys: []string{"Tab"}, Desc: "move down, like ↓"},
		},
	},
	{
		Scope: ScopeQuestion,
		Title: "Question ask",
		Note:  "The answers wrap: stepping past the last lands on the first.",
		Bindings: []Binding{
			{Keys: []string{"Tab"}, Hint: "next", Desc: "move to the next question"},
			{Keys: []string{"↑↓"}, Hint: "select", Desc: "move between answers"},
			{Keys: []string{"Enter"}, Hint: "confirm", Desc: "confirm the answers"},
			{Keys: []string{"Esc"}, Hint: "dismiss", Desc: "dismiss the ask"},
			{Keys: []string{"1-9"}, Desc: "take the numbered answer"},
			{Keys: []string{"j/k"}, Desc: "move between answers, like ↑↓"},
			{Keys: []string{"h/l", "←→"}, Desc: "switch between the questions"},
			{Keys: []string{"Space"}, Desc: "take the highlighted answer, like Enter"},
		},
	},
	{
		Scope: ScopeAnswer,
		Title: "Question ask, own answer",
		Bindings: []Binding{
			{Keys: []string{"Enter"}, Hint: "send", Desc: "send the typed answer"},
			{Keys: []string{"Esc"}, Hint: "cancel", Desc: "drop the typed answer and go back to the options"},
		},
	},
	{
		Scope: ScopeConnect,
		Title: "Provider picker",
		Bindings: []Binding{
			{Hint: "Type to filter", Desc: ""},
			{Keys: []string{"↑↓"}, Hint: "navigate", Desc: "move between providers"},
			{Keys: []string{"Enter"}, Hint: "select", Desc: "pick the provider"},
			{Keys: []string{"Esc"}, Hint: "cancel", Desc: "close without connecting"},
		},
	},
	{
		Scope: ScopeConnectMethod,
		Title: "Sign-in method",
		Bindings: []Binding{
			{Keys: []string{"↑↓"}, Hint: "navigate", Desc: "move between sign-in methods"},
			{Keys: []string{"Enter"}, Hint: "select", Desc: "start this sign-in method"},
			{Keys: []string{"←"}, Hint: "back", Desc: "return to the provider list"},
			{Keys: []string{"Esc"}, Hint: "cancel", Desc: "close without connecting"},
		},
	},
	{
		Scope: ScopeConnectKey,
		Title: "API key entry",
		Bindings: []Binding{
			{Hint: "Paste or type key", Desc: ""},
			{Keys: []string{"Enter"}, Hint: "save", Desc: "store the key and connect"},
			{Keys: []string{"Esc"}, Hint: "cancel", Desc: "close without storing anything"},
		},
	},
	{
		Scope: ScopeContext,
		Title: "Context browser (/context)",
		Bindings: []Binding{
			{Keys: []string{"↑↓", "j/k"}, Hint: "move", Desc: "move between entries"},
			{Keys: []string{"Shift+↑↓"}, Hint: "select", Desc: "extend the selection over a range"},
			{Keys: []string{"Enter"}, Hint: "view", Desc: "open the selected block"},
			{Keys: []string{"/"}, Hint: "jump", Desc: "fuzzy-jump to an entry by its text"},
			{Keys: []string{"."}, Hint: "menu", Desc: "the commands for the selected entry, chords named"},
			{Keys: []string{"Esc"}, Hint: "close", Desc: "close the browser"},
			{Keys: []string{"Del"}, Desc: "drop the selected entries from the context"},
			{Keys: []string{"t"}, Desc: "drop everything before the selected entry"},
			{Keys: []string{"c"}, Desc: "summarize the history now"},
			{Keys: []string{"r"}, Desc: "re-read the context"},
			{Keys: []string{"d"}, Desc: "delete, like Del"},
			{Keys: []string{"gg", "G"}, Desc: "jump to the first or last entry — 12G goes to the twelfth"},
			{Keys: []string{"3j"}, Desc: "a digit prefix repeats a move"},
			{Keys: []string{"Home", "End"}, Desc: "jump to the first or last entry"},
			{Keys: []string{"PgUp", "PgDn"}, Desc: "move a screen"},
			{Keys: []string{"Ctrl+U", "Ctrl+D"}, Desc: "move half a screen"},
		},
	},
	{
		Scope: ScopeContextRaw,
		Title: "Context browser, block viewer",
		Note:  "Scrolls like every list: counts work (3j), gg/G jump, Ctrl+U/D half a screen.",
		Bindings: []Binding{
			{Keys: []string{"j/k"}, Hint: "scroll", Desc: "scroll the block"},
			{Keys: []string{"Enter"}, Hint: "close", Desc: "close the block"},
			{Keys: []string{"Esc", "q"}, Desc: "close the block, like Enter"},
			{Keys: []string{"PgUp", "PgDn"}, Desc: "scroll a screen"},
			{Keys: []string{"Home", "End"}, Desc: "jump to the top or the bottom"},
		},
	},
	{
		Scope:    ScopeWatches,
		Title:    "Watch browser (/watches)",
		TitleCmd: CmdWatches,
		Note:     "The session's watches: state, event count, age. Moves like every list.",
		Bindings: []Binding{
			{Keys: []string{"↑↓", "j/k"}, Hint: "select", Desc: "move between watches"},
			{Keys: []string{"Enter"}, Hint: "log", Desc: "open the selected watch's event log"},
			{Keys: []string{"s"}, Hint: "stop", Desc: "stop the selected live watch, after a y/n"},
			{Keys: []string{"r"}, Desc: "re-read the watch list"},
			{Keys: []string{"Esc", "q"}, Hint: "close", Desc: "close the browser"},
			{Keys: []string{"gg", "G"}, Desc: "jump to the first or last watch"},
			{Keys: []string{"PgUp", "PgDn"}, Desc: "move a screen"},
			{Keys: []string{"Ctrl+U", "Ctrl+D"}, Desc: "move half a screen"},
		},
	},
	{
		Scope: ScopeWatchLog,
		Title: "Watch browser, log viewer",
		Note:  "The selected watch's event tail, fetched when it opened. Scrolls like every list.",
		Bindings: []Binding{
			{Keys: []string{"j/k"}, Hint: "scroll", Desc: "scroll the log"},
			{Keys: []string{"Enter", "Esc", "q"}, Hint: "back", Desc: "back to the watch list"},
			{Keys: []string{"PgUp", "PgDn"}, Desc: "scroll a screen"},
			{Keys: []string{"Home", "End"}, Desc: "jump to the top or the bottom"},
		},
	},
	{
		Scope: ScopePlan,
		Title: "Plan editor (/plan)",
		Note:  "Moves like every list: counts work (3j, 12G), gg/G jump, Ctrl+U/D half a screen.",
		Bindings: []Binding{
			{Keys: []string{"↑↓", "j/k"}, Hint: "select", Desc: "move between rows"},
			{Keys: []string{"Enter"}, Hint: "open", Desc: "open the selected row"},
			{Keys: []string{"/"}, Hint: "jump", Desc: "fuzzy-jump: type to select the matching row"},
			{Keys: []string{"."}, Hint: "menu", Desc: "open the action menu for the selected row"},
			{Keys: []string{"Ctrl+S"}, Hint: "apply", Desc: "write the edits back to the plan"},
			{Keys: []string{"Esc"}, Hint: "close", Desc: "close the editor, dropping unapplied edits"},
			{Keys: []string{"Alt+↑↓"}, Desc: "move the selected step up or down the plan"},
			{Keys: []string{"Del"}, Desc: "delete the selected criterion, constraint or step"},
			{Keys: []string{"Ctrl+Z", "Ctrl+Y"}, Desc: "undo or redo the last edit"},
			{Keys: []string{"Space"}, Desc: "open the selected row, like Enter"},
			{Keys: []string{"gg", "G"}, Desc: "jump to the first or last row"},
			{Keys: []string{"Home", "End"}, Desc: "jump to the first or last row"},
			{Keys: []string{"PgUp", "PgDn"}, Desc: "move a screen"},
			{Keys: []string{"Ctrl+U", "Ctrl+D"}, Desc: "move half a screen"},
		},
	},
	{
		Scope: ScopePlanDetail,
		Title: "Plan editor, step details",
		Note:  "Moves like the step list: counts work (3j), gg/G, PgUp/PgDn, Ctrl+U/D.",
		Bindings: []Binding{
			{Keys: []string{"Enter"}, Hint: "edit/action", Desc: "edit the field, or run the action on the row"},
			{Keys: []string{"/"}, Hint: "jump", Desc: "fuzzy-jump: type to select the matching row"},
			{Keys: []string{"."}, Hint: "menu", Desc: "open the action menu for this step"},
			{Keys: []string{"Esc"}, Hint: "back", Desc: "go back to the step list"},
			{Keys: []string{"Alt+↑↓"}, Desc: "move this step up or down the plan"},
			{Keys: []string{"Del"}, Desc: "delete the step this screen edits"},
			{Keys: []string{"Ctrl+Z", "Ctrl+Y"}, Desc: "undo or redo the last edit"},
			{Keys: []string{"Space"}, Desc: "activate the selected row, like Enter"},
		},
	},
	{
		Scope: ScopePlanText,
		Title: "Plan editor, field editor",
		Note:  "Opens at the bottom of the panel; the list stays visible above it.",
		Bindings: []Binding{
			{Keys: []string{"Enter", "Ctrl+S"}, Hint: "save", Desc: "save the field and go back to the plan"},
			{Keys: []string{"Shift/Ctrl+Enter"}, Hint: "newline", Desc: "start a new line"},
			{Keys: []string{"Esc"}, Hint: "cancel", Desc: "close without saving"},
		},
	},
	{
		Scope: ScopeJump,
		Title: "Fuzzy jump (/)",
		Note:  "On every long list. Opens at the bottom; the best match is selected live in the list above.",
		Bindings: []Binding{
			{Keys: []string{"↑↓"}, Hint: "cycle", Desc: "cycle through the matches"},
			{Keys: []string{"Enter"}, Hint: "keep", Desc: "close the jump, keeping the selection"},
			{Keys: []string{"Esc"}, Hint: "back", Desc: "close the jump and restore the selection"},
		},
	},
	{
		Scope: ScopeMenu,
		Title: "Action menu (.)",
		Note:  "On every long list: the commands for the selected row, each naming the chord that runs it directly.",
		Bindings: []Binding{
			{Keys: []string{"↑↓", "j/k"}, Hint: "move", Desc: "move between commands"},
			{Keys: []string{"Enter"}, Hint: "run", Desc: "run the command and close the menu"},
			{Keys: []string{"Esc"}, Hint: "back", Desc: "close the menu without acting"},
			{Keys: []string{"Space"}, Desc: "run, like Enter"},
		},
	},
	{
		Scope: ScopePlanChoice,
		Title: "Plan editor, choice list",
		Note:  "Picking a step type, a model, or an action's event or type. The cursor starts on the current value.",
		Bindings: []Binding{
			{Keys: []string{"↑↓", "j/k"}, Hint: "move", Desc: "move between options"},
			{Keys: []string{"Enter"}, Hint: "choose", Desc: "choose the option and go back"},
			{Keys: []string{"Esc"}, Hint: "back", Desc: "go back without choosing"},
			{Keys: []string{"Space"}, Desc: "choose, like Enter"},
		},
	},
	{
		Scope: ScopePlanSkills,
		Title: "Plan editor, skills picker",
		Note:  "Multi-select from the installed skill catalog: what is checked rides the step's inject_skill action.",
		Bindings: []Binding{
			{Keys: []string{"↑↓", "j/k"}, Hint: "move", Desc: "move between skills"},
			{Keys: []string{"Enter", "Space"}, Hint: "toggle", Desc: "check or uncheck the selected skill"},
			{Keys: []string{"/"}, Hint: "filter", Desc: "fuzzy-jump to a skill"},
			{Keys: []string{"Esc"}, Hint: "done", Desc: "keep the checked set and go back"},
		},
	},
	{
		Scope:    ScopeSettings,
		Title:    "Settings",
		TitleCmd: CmdSettings,
		Note:     "Moves like every list: counts work (3j), gg/G jump, Ctrl+U/D half a screen.",
		Bindings: []Binding{
			{Keys: []string{"/"}, Hint: "jump", Desc: "fuzzy-jump to a row"},
			{Keys: []string{"."}, Hint: "menu", Desc: "open the action menu for this row"},
			{Keys: []string{"Ctrl+S"}, Hint: "apply", Desc: "write the settings to disk"},
			{Keys: []string{"Esc"}, Hint: "discard", Desc: "close a picker, then the modal, discarding the changes"},
			{Keys: []string{"Tab", "Shift+Tab"}, Desc: "switch between the tabs"},
			{Keys: []string{"↑↓", "j/k"}, Desc: "move between rows"},
			{Keys: []string{"Enter", "Space"}, Desc: "activate the selected row"},
			{Keys: []string{"gg", "G"}, Desc: "jump to the first or last row"},
			{Keys: []string{"Home", "End"}, Desc: "jump to the first or last row"},
			{Keys: []string{"PgUp", "PgDn"}, Desc: "move a screen"},
			{Keys: []string{"Ctrl+U", "Ctrl+D"}, Desc: "move half a screen"},
		},
	},
	{
		Scope: ScopeHelp,
		Title: "This screen",
		Note:  "Moves like every list: counts work (3j, 5G), gg/G jump, Ctrl+U/D half a screen.",
		Bindings: []Binding{
			{Keys: []string{"↑↓", "j/k"}, Hint: "scroll", Desc: "scroll a row"},
			{Keys: []string{"PgUp", "PgDn"}, Hint: "page", Desc: "scroll a screen"},
			{Keys: []string{"gg", "G"}, Desc: "jump to the top or the bottom"},
			{Keys: []string{"Home", "End"}, Desc: "jump to the top or the bottom"},
			{Keys: []string{"Ctrl+U", "Ctrl+D"}, Desc: "scroll half a screen"},
			{Keys: []string{"Esc", "q"}, Hint: "close", Desc: "close the help screen"},
		},
	},
}
