package sessions

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/components/mention"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
)

// forkTabDoor is what the shell tells a view about tabs: whether another one
// may be opened right now, and how to open one on a session file. Both halves
// are asked before the copy is written, because a session file written for a
// tab that cannot be opened is a file nobody asked for.
type forkTabDoor struct {
	room func() error
	open func(path string) (*View, error)
}

// ConfigureSessionFork wires opening a forked session as another tab. The
// shell owns what a tab is made of, so this side asks it whether there is
// room, hands it a session file and receives the view that now shows it.
//
// It is its own call rather than part of ConfigureSessionNavigation so the
// two can be wired in either order. A view left without it does not fork at
// all: a sub-agent's screen has no tabs, and the copy would have nowhere to
// go.
func (e *View) ConfigureSessionFork(room func() error, open func(path string) (*View, error)) {
	if e != nil {
		e.openFork = &forkTabDoor{room: room, open: open}
	}
}

// forkRoom asks the shell whether a fork has somewhere to land, and says why
// not in the words the user reads. It is asked before anything is written.
func (e *View) forkRoom() error {
	if e.openFork == nil || e.openFork.open == nil {
		return errors.New("this screen does not open tabs")
	}
	if e.openFork.room == nil {
		return nil
	}
	return e.openFork.room()
}

// configureFork registers /fork. Like /rewind it lives here rather than among
// the builtins, because its completer has to ask this session where its turns
// begin and end.
func (e *View) configureFork() {
	e.commands.Register(commands.Command{
		Name:        "fork",
		Description: "Copy this conversation into a new tab - /fork [<id>]",
		Slash:       true,
		Insert:      "/fork ",
		ArgCompleter: func(args []string, partial string) []mention.Item {
			if len(args) > 0 {
				return nil
			}
			return e.forkItems(partial)
		},
		Run: func(ctx commands.CommandContext) error {
			switch len(ctx.Args) {
			case 0:
				// No id at all is the copy of the whole conversation, taken
				// from wherever the cursor stands.
				e.ForkFrom("")
				return nil
			case 1:
				e.ForkFrom(ctx.Args[0])
				return nil
			default:
				return errors.New("usage: /fork [<entry id>], or /fork to copy the whole conversation")
			}
		},
	})
}

// forkItems offers the turn boundaries of this session, newest first, because
// the place a fork is usually aimed at is a turn or two back.
func (e *View) forkItems(partial string) []mention.Item {
	if e.ctrl == nil {
		return nil
	}
	boundaries := e.ctrl.ForkBoundaries()
	items := make([]mention.Item, 0, len(boundaries))
	for _, boundary := range slices.Backward(boundaries) {
		if !strings.HasPrefix(boundary.EntryID, partial) {
			continue
		}
		items = append(items, mention.Item{
			Path:        boundary.EntryID,
			Description: boundary.Preview,
		})
	}
	return items
}

// ForkFrom copies the conversation up to the entry into a session of its own
// and opens it as another tab. An empty entry copies the whole conversation.
// This session is left alone: its transcript, its cursor and its draft are
// where they were, and the next turn taken here is not in the copy.
func (e *View) ForkFrom(entryID string) {
	if e == nil || e.ctrl == nil {
		return
	}
	// Asked before the copy is written, not after. A screen with no tabs and
	// a shell with no room left both leave the fork nowhere to go, and a
	// session file nobody can reach is worse than a refusal.
	if err := e.forkRoom(); err != nil {
		e.toast.Show("Cannot fork: "+err.Error(), toast.ToastWarning, 4*time.Second)
		return
	}
	result, err := e.ctrl.Fork(entryID)
	if err != nil {
		e.toast.Show("Cannot fork: "+err.Error(), toast.ToastWarning, 4*time.Second)
		return
	}
	opened, err := e.openFork.open(result.File)
	if err != nil || opened == nil {
		e.toast.Show(forkUnopenedNotice(result, err), toast.ToastWarning, 6*time.Second)
		return
	}
	// A fork cut before a prompt hands its text over, exactly as a rewind
	// does, except that the composer it lands in belongs to the new tab.
	opened.adoptForkPrompt(result.Prompt)
	// The new tab is the one in front of the user now, so the notice belongs
	// on it.
	opened.toast.Show(forkNotice(e.forkTabName(opened)), toast.ToastSuccess, 5*time.Second)
}

// adoptForkPrompt puts the prompt the copy stopped before into the composer
// of the tab that just opened. The tab is new, so there is nothing to keep
// out of the way: the prompt is the whole draft.
func (e *View) adoptForkPrompt(prompt string) {
	if e == nil || prompt == "" || e.composer == nil {
		return
	}
	e.composer.Chat.ClearSelection()
	e.composer.Chat.Value = prompt
	e.composer.Chat.Cursor = len(prompt)
	e.composer.FocusChat()
}

// forkTabName finds what the shell called the tab it just opened, so the
// notice can name it. An empty answer means the shell keeps no name for it.
func (e *View) forkTabName(opened *View) string {
	if e == nil || opened == nil || e.navigation == nil || e.navigation.registry == nil {
		return ""
	}
	for _, entry := range e.navigation.registry.Entries() {
		if entry.View == opened {
			return entry.Name
		}
	}
	return ""
}

// forkNotice names the tab the copy opened in.
func forkNotice(tab string) string {
	if tab == "" {
		return "Forked into a new tab"
	}
	return "Forked into " + tab
}

// forkUnopenedNotice reports a copy that was written but could not be shown.
// The file is on disk, so the answer says how to reach it rather than leaving
// the user to think the fork failed.
func forkUnopenedNotice(result session.ForkResult, err error) string {
	id := session.ShortID(result.SessionID)
	why := "the shell opened none"
	if err != nil {
		why = err.Error()
	}
	return "Forked into session " + id + ", but no tab could be opened: " + why +
		". Open it with /resume " + id
}
