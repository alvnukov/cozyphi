package sessions

import (
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/overlays"
)

// A sub-agent drains its bus whether or not anyone is looking at it, so its
// permission, continue and question asks arrive while it has no screen. They
// are shown on the screen its family is on — the parent's, or a sibling's —
// labelled with the child's name, and answered into the child's own reply
// channel. Nothing is copied: one ask lives in exactly one overlay, and the
// session that raised it is the only one that can withdraw it.

// askHost is the view that shows this session's asks, and the origin stamped
// on them. A session that has its own screen keeps its asks; a hidden child
// hands them to the screen its family is on, unless that screen is already
// answering something else — an arriving ask must never cancel the question
// the user is looking at, so it stays with the child, whose row says it is
// waiting.
func (e *View) askHost() (*View, overlays.AskOrigin) {
	if e == nil || e.childJobID == "" {
		return e, overlays.AskOrigin{}
	}
	// The owner travels with the ask even on the child's own screen: the
	// grants offered are the child's either way, and a dismissal has to find
	// the ask wherever it landed.
	origin := overlays.AskOrigin{Owner: e.childJobID}
	host := e.family.Screen()
	if host == nil || host == e || host.Closed() || host.overlays.Active() {
		return e, origin
	}
	origin.Label = e.childTitle
	return host, origin
}

// showAsk puts one ask on screen and pings the user through the session they
// would answer it on: a child has no tab to switch to, so the parent's
// notifier is the one that names something the user can find.
func (e *View) showAsk(m controller.Msg, detail string) {
	host, origin := e.askHost()
	host.overlays.ApplyFrom(m, origin)
	if host.notifier != nil {
		host.notifier.NeedsAttention(detail)
	}
	if host != e {
		host.RequestRedraw()
	}
}

// withdrawAsk takes back an ask its session gave up on. The screen may have
// changed since it was shown, so every view of the family is offered the
// withdrawal and only the one holding that session's ask acts on it.
func (e *View) withdrawAsk(m controller.Msg) {
	if e == nil {
		return
	}
	if e.childJobID == "" {
		e.overlays.Apply(m)
		return
	}
	origin := overlays.AskOrigin{Owner: e.childJobID}
	for _, v := range e.family.hosts() {
		v.overlays.ApplyFrom(m, origin)
	}
}

// recordAskAttention marks this session as waiting and, for a sub-agent, the
// session that owns it: a child has no tab, so the notice that names a
// session to switch to can only name its parent.
func (e *View) recordAskAttention(text string) {
	e.recordAttention(text)
	if e.childJobID == "" || e.family == nil || e.family.parent == nil || e.family.parent == e {
		return
	}
	e.family.parent.recordAttention("[" + e.childTitle + "] " + text)
}

// isChild reports whether this view was built for a sub-agent assignment
// rather than opened by the user.
func (e *View) isChild() bool { return e != nil && e.childJobID != "" }
