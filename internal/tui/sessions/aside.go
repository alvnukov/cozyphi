package sessions

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/components/mention"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
)

// asideAnchorPrefix marks the first argument of /btw as the message the
// question is about rather than the first word of the question.
const asideAnchorPrefix = "@"

const asideUsage = "usage: /btw <question>, or /btw @<entry id> <question> to ask about an earlier message"

// configureAside registers /btw. Like /rewind and /fork it lives here rather
// than among the builtins, because its completer has to ask this session
// which messages it holds.
func (e *View) configureAside() {
	e.commands.Register(commands.Command{
		Name:        "btw",
		Description: "Ask a side question that stays out of the context - /btw [@<id>] <question>",
		Slash:       true,
		Insert:      "/btw ",
		ArgCompleter: func(args []string, partial string) []mention.Item {
			if len(args) > 0 || !strings.HasPrefix(partial, asideAnchorPrefix) {
				return nil
			}
			return e.asideItems(strings.TrimPrefix(partial, asideAnchorPrefix))
		},
		Run: func(ctx commands.CommandContext) error {
			anchor, question, err := parseAsideArgs(ctx.Args)
			if err != nil {
				return err
			}
			e.AskAside(anchor, question)
			return nil
		},
	})
}

// parseAsideArgs splits /btw into the anchor, empty for the current context,
// and the question.
func parseAsideArgs(args []string) (anchor, question string, err error) {
	if len(args) > 0 && strings.HasPrefix(args[0], asideAnchorPrefix) {
		anchor = strings.TrimPrefix(args[0], asideAnchorPrefix)
		if anchor == "" {
			return "", "", errors.New(asideUsage)
		}
		args = args[1:]
	}
	question = strings.Join(args, " ")
	if strings.TrimSpace(question) == "" {
		return "", "", errors.New(asideUsage)
	}
	return anchor, question, nil
}

// asideItems offers the messages of the context, newest first, because a
// side question is usually about something said a moment ago.
func (e *View) asideItems(partial string) []mention.Item {
	if e.ctrl == nil {
		return []mention.Item{}
	}
	anchors := e.ctrl.AsideAnchors()
	items := make([]mention.Item, 0, len(anchors))
	for _, anchor := range slices.Backward(anchors) {
		if !strings.HasPrefix(anchor.EntryID, partial) {
			continue
		}
		items = append(items, mention.Item{
			Path:        asideAnchorPrefix + anchor.EntryID,
			Description: anchor.Preview,
		})
	}
	return items
}

// AskAside asks a side question about the context up to the entry, or about
// all of it when the entry is empty. The answer streams into the feed and is
// kept in the session file, and the model never sees either again.
func (e *View) AskAside(entryID, question string) {
	if e == nil || e.ctrl == nil {
		return
	}
	if err := e.ctrl.Aside(question, entryID); err != nil {
		e.toast.Show("Cannot ask on the side: "+err.Error(), toast.ToastWarning, 4*time.Second)
	}
}
