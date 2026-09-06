package sessions

import (
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// A sub-agent drains its bus whether or not anyone is looking at it, so its
// permission, continue and question asks arrive while it has no screen — and
// the session that owns it keeps asking after the user has opened a child. So
// no session answers for itself here: every ask goes to the family, which
// holds one slot plus a queue behind it and draws the head on whichever screen
// the user is on. See family_ask.go for what happens next.

// showAsk hands one ask to the family that decides where it is answered.
func (e *View) showAsk(m controller.Msg, detail string) {
	if e == nil {
		return
	}
	if e.family == nil {
		// A view built without a family answers for itself, which is what a
		// headless or test assembly wants.
		e.overlays.Apply(m)
		if e.notifier != nil {
			e.notifier.NeedsAttention(detail)
		}
		return
	}
	e.family.raiseAsk(e, m, detail)
}

// withdrawAsk takes back an ask its session gave up on, wherever the family
// put it: on screen, or still waiting its turn.
func (e *View) withdrawAsk(m controller.Msg) {
	if e == nil {
		return
	}
	if e.family == nil {
		e.overlays.Apply(m)
		return
	}
	e.family.withdrawAsk(e, m)
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
