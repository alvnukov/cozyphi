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

// rewindBackArg is the word that undoes the last cursor move instead of
// naming a boundary to cut at.
const rewindBackArg = "back"

// rewindDraft is what the composer held around a hand-back: before is the
// text that was there, after is what the hand-back made of it. Keeping both
// is what lets the undo give back exactly the prompt it took and nothing of
// the user's own.
type rewindDraft struct {
	before string
	after  string
}

// configureRewind registers /rewind. It lives here rather than among the
// builtins because its completer has to ask this session where its turns
// begin and end, and the builtins know no session.
func (e *View) configureRewind() {
	e.commands.Register(commands.Command{
		Name:        "rewind",
		Description: "Move the context back to a turn boundary - /rewind <id>|back",
		Slash:       true,
		Insert:      "/rewind ",
		ArgCompleter: func(args []string, partial string) []mention.Item {
			if len(args) > 0 {
				return nil
			}
			return e.rewindItems(partial)
		},
		Run: func(ctx commands.CommandContext) error {
			switch len(ctx.Args) {
			case 0:
				return errors.New("usage: /rewind <entry id>, or /rewind back to undo the last one")
			case 1:
				if strings.EqualFold(ctx.Args[0], rewindBackArg) {
					e.rewindBack()
					return nil
				}
				e.RewindTo(ctx.Args[0])
				return nil
			default:
				return errors.New("usage: /rewind <entry id>, or /rewind back to undo the last one")
			}
		},
	})
}

// rewindItems offers the turn boundaries of this session, newest first,
// because the place a rewind is usually aimed at is a turn or two back. The
// undo comes first when nothing has been typed yet, so the way out of a
// rewind is the first thing the list shows.
func (e *View) rewindItems(partial string) []mention.Item {
	items := make([]mention.Item, 0, 8)
	if strings.HasPrefix(rewindBackArg, strings.ToLower(partial)) {
		items = append(items, mention.Item{
			Path:        rewindBackArg,
			Description: "undo the last rewind",
		})
	}
	if e.ctrl == nil {
		return items
	}
	for _, boundary := range slices.Backward(e.ctrl.TurnBoundaries()) {
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

// RewindTo cuts the context back to the turn boundary the entry anchors: the
// cursor moves, the rows past it leave the feed, and a prompt anchor gives
// its text back to the composer so the turn can be sent again, reworded.
func (e *View) RewindTo(entryID string) {
	if e == nil || entryID == "" {
		return
	}
	if e.ctrl == nil {
		return
	}
	result, err := e.ctrl.Rewind(entryID)
	if err != nil {
		e.toast.Show("Cannot rewind: "+err.Error(), toast.ToastWarning, 4*time.Second)
		return
	}
	e.applyRewind(result)
	e.toast.Show(rewindNotice(result), toast.ToastSuccess, 5*time.Second)
}

// rewindBack undoes the last cursor move (/rewind back). It is recorded like
// any other move, so pressing it twice lands where it started.
func (e *View) rewindBack() {
	if e == nil || e.ctrl == nil {
		return
	}
	result, err := e.ctrl.UndoRewind()
	if err != nil {
		e.toast.Show("Cannot rewind: "+err.Error(), toast.ToastWarning, 4*time.Second)
		return
	}
	e.applyRewind(result)
	e.toast.Show("Rewind undone: the context is back where it was", toast.ToastSuccess, 4*time.Second)
}

// applyRewind redraws the feed on the branch the cursor now stands on and
// settles the composer around the prompt the cut handed back.
func (e *View) applyRewind(result session.RewindResult) {
	e.restoreRewindPrompt(result.Prompt)
	e.transcript.LoadReplay(e.ctrl.ReplaySnapshot())
	e.transcript.SetHistoricalShellTasks(e.ctrl.ReplayShellTasks())
	e.applyShellTasks(e.shellTasks)
	e.transcript.Sync()
	e.transcript.StickToBottom()
}

// restoreRewindPrompt puts the anchor prompt back in the composer, and takes
// it away again when the move that follows makes the prompt part of the
// context once more.
//
// The undo restores the composer to what it held before the hand-back, not to
// nothing: whatever the user had typed before the rewind is theirs, and so is
// the prompt an earlier rewind handed back. It restores only while the
// composer still holds exactly what the hand-back produced, so a draft the
// user has edited since is left alone.
func (e *View) restoreRewindPrompt(prompt string) {
	if prompt == "" {
		draft := e.rewindDraft
		e.rewindDraft = rewindDraft{}
		if draft.after == "" || e.composer.Chat.Value != draft.after {
			return
		}
		e.composer.Chat.ClearSelection()
		e.composer.Chat.Value = draft.before
		e.composer.Chat.Cursor = len(draft.before)
		return
	}
	e.composer.Chat.ClearSelection()
	before := e.composer.Chat.Value
	// A draft typed before the rewind survives; the prompt joins it on a new
	// line rather than overwriting it, the way a recalled queued prompt does.
	after := prompt
	if strings.TrimSpace(before) != "" {
		after = before + "\n" + prompt
	}
	e.composer.Chat.Value = after
	e.composer.Chat.Cursor = len(after)
	e.rewindDraft = rewindDraft{before: before, after: after}
	e.composer.FocusChat()
}

// rewindNotice names where the context now ends and how to get back.
func rewindNotice(result session.RewindResult) string {
	where := "the start of the session"
	if result.Prompt != "" {
		where = "before " + displayLine(result.Prompt, 40)
	} else if result.Target != "" {
		where = "after " + session.ShortID(result.Target)
	}
	return "Rewound to " + where + "; /rewind back undoes it"
}

// displayLine shortens text to one readable line for a notice.
func displayLine(text string, limit int) string {
	runes := []rune(strings.Join(strings.Fields(text), " "))
	if len(runes) > limit {
		return string(runes[:limit]) + "..."
	}
	return string(runes)
}
