// Package keys is cozyphi's key-binding catalog: one declarative table that
// both the /help screen and the panes' footer hint rows read. A binding is
// written down once, so the row a pane advertises cannot drift from the row
// the help screen shows.
//
// The catalog owns the standing hint rows. Transient prompts — "delete 3
// blocks? y confirm · n cancel" — stay local to the pane that asks them.
package keys

import (
	"slices"
	"strings"
)

// Separator joins hint fragments in a footer row.
const Separator = " · "

// Scope names one keyboard context: a pane, a modal, or the chat view as a
// whole. A footer row renders a single scope; the help screen renders all.
type Scope string

// The scopes, in the order the help screen shows them.
const (
	ScopeGlobal     Scope = "global"
	ScopeComposer   Scope = "composer"
	ScopeTranscript Scope = "transcript"
	ScopeSidebar    Scope = "sidebar"
	ScopePlanFocus  Scope = "plan-focus"
	ScopeAsk        Scope = "ask"
	ScopeContinue   Scope = "continue"
	ScopeQuestion   Scope = "question"
	ScopeAnswer     Scope = "answer"
	ScopeConnect    Scope = "connect"
	ScopeConnectKey Scope = "connect-key"
	ScopeContext    Scope = "context"
	ScopeContextRaw Scope = "context-block"
	ScopePlan       Scope = "plan"
	ScopePlanDetail Scope = "plan-detail"
	ScopePlanText   Scope = "plan-text"
	ScopePlanChoice Scope = "plan-choice"
	ScopeSettings   Scope = "settings"
	ScopeHelp       Scope = "help"
)

// Binding is one key — or a set of interchangeable keys — and what it does.
// Hint is the terse footer wording ("apply"), Desc the help-screen sentence.
// An empty Hint keeps a binding out of footer rows; an empty Desc keeps it
// out of the help screen.
type Binding struct {
	Keys []string
	Hint string
	Desc string
}

// Label joins the interchangeable spellings for display: "Shift/Ctrl+Enter".
func (b Binding) Label() string { return strings.Join(b.Keys, "/") }

// Group is one scope's bindings, in the order they should be shown.
type Group struct {
	Scope Scope
	Title string
	// Note is an optional line under the title: what the scope covers, or a
	// platform caveat.
	Note     string
	Bindings []Binding
}

// Groups returns the whole catalog in display order.
func Groups() []Group { return slices.Clone(catalog) }

// Find returns one scope's group.
func Find(s Scope) (Group, bool) {
	for _, g := range catalog {
		if g.Scope == s {
			return g, true
		}
	}
	return Group{}, false
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
			{Keys: []string{"F1"}, Desc: "open this help screen — /help does the same"},
			{Keys: []string{"Ctrl+K"}, Desc: "open the command palette"},
			{Keys: []string{"Ctrl+,"}, Desc: "open settings"},
			{Keys: []string{"Ctrl+P"}, Desc: "open the plan viewer and editor"},
			{Keys: []string{"Alt+P"}, Desc: "move focus to the plan in the sidebar"},
			{Keys: []string{"Ctrl+O"}, Desc: "show or hide the sidebar"},
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
		Note:  "The wheel scrolls; dragging selects text.",
		Bindings: []Binding{
			{Keys: []string{"PgUp", "PgDn"}, Desc: "scroll one screen"},
			{Keys: []string{"Ctrl+Shift+C", "Cmd+C"}, Desc: "copy the selected block, or the last message"},
		},
	},
	{
		Scope: ScopeSidebar,
		Title: "Sidebar and plan",
		Bindings: []Binding{
			{Keys: []string{"Ctrl+O"}, Desc: "show or hide the sidebar"},
			{Keys: []string{"Ctrl+A"}, Desc: "approve the plan, or stop an approved one"},
			{Keys: []string{"Ctrl+D"}, Desc: "expand or collapse the step details"},
			{Keys: []string{"Ctrl+↑↓"}, Desc: "scroll the plan one row"},
			{Keys: []string{"Ctrl+PgUp", "Ctrl+PgDn"}, Desc: "scroll the plan one screen"},
			{Keys: []string{"Alt+P"}, Desc: "move focus into the plan"},
		},
	},
	{
		Scope: ScopePlanFocus,
		Title: "Plan, once focused",
		Note:  "After Alt+P, plain keys go to the plan until Esc gives them back.",
		Bindings: []Binding{
			{Keys: []string{"↑↓", "j/k"}, Desc: "move between steps"},
			{Keys: []string{"Enter", "m"}, Desc: "open the model picker for the step"},
			{Keys: []string{"g", "G"}, Desc: "jump to the first or last entry"},
			{Keys: []string{"Esc"}, Desc: "hand the keyboard back to the message input"},
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
			{Keys: []string{"Del"}, Hint: "delete", Desc: "drop the selected entries from the context"},
			{Keys: []string{"t"}, Hint: "trim", Desc: "drop everything before the selected entry"},
			{Keys: []string{"c"}, Hint: "compact", Desc: "summarize the history now"},
			{Keys: []string{"r"}, Hint: "refresh", Desc: "re-read the context"},
			{Keys: []string{"Esc"}, Hint: "close", Desc: "close the browser"},
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
		Scope: ScopePlan,
		Title: "Plan editor (/plan)",
		Note:  "Moves like every list: counts work (3j, 12G), gg/G jump, Ctrl+U/D half a screen.",
		Bindings: []Binding{
			{Keys: []string{"↑↓", "j/k"}, Hint: "select", Desc: "move between rows"},
			{Keys: []string{"Enter"}, Hint: "open", Desc: "open the selected row"},
			{Keys: []string{"Alt+↑↓"}, Hint: "move step", Desc: "move the selected step up or down the plan"},
			{Keys: []string{"Del"}, Hint: "delete", Desc: "delete the selected criterion, constraint or step"},
			{Keys: []string{"Ctrl+S"}, Hint: "apply", Desc: "write the edits back to the plan"},
			{Keys: []string{"Esc"}, Hint: "close", Desc: "close the editor, dropping unapplied edits"},
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
			{Keys: []string{"Alt+↑↓"}, Hint: "move step", Desc: "move this step up or down the plan"},
			{Keys: []string{"Del"}, Hint: "delete step", Desc: "delete the step this screen edits"},
			{Keys: []string{"Esc"}, Hint: "back", Desc: "go back to the step list"},
			{Keys: []string{"Space"}, Desc: "activate the selected row, like Enter"},
		},
	},
	{
		Scope: ScopePlanText,
		Title: "Plan editor, text popup",
		Bindings: []Binding{
			{Keys: []string{"Enter", "Ctrl+S"}, Hint: "save", Desc: "save the field and go back to the plan"},
			{Keys: []string{"Shift/Ctrl+Enter"}, Hint: "newline", Desc: "start a new line"},
			{Keys: []string{"Esc"}, Hint: "cancel", Desc: "close without saving"},
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
		Scope: ScopeSettings,
		Title: "Settings (Ctrl+,)",
		Note:  "Moves like every list: counts work (3j), gg/G jump, Ctrl+U/D half a screen.",
		Bindings: []Binding{
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
