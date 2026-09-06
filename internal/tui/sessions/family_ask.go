package sessions

import (
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/overlays"
)

// A family answers its questions where the user is standing. Every permission,
// continue and question ask any member raises — the session that owns the
// family included — is held here, and the screen the family is currently
// showing renders the one at the head. Change the screen and the ask goes with
// the user: it leaves the overlay it was in without being answered and opens on
// the new one, so it is never duplicated and never lost. A second ask waits its
// turn behind the first and opens the moment the first is answered.
//
// Nothing is copied and nothing is re-addressed: the reply always travels down
// the channel the asking session put in the message, and the family only
// decides which overlay draws it.

// askKind tells the three kinds of ask apart, so a withdrawal lands on the
// ask it was sent for rather than on another one the same session raised.
type askKind int

const (
	askNone askKind = iota
	askPermission
	askContinue
	askQuestion
)

// msgAskKind classifies an ask or its withdrawal; askNone is anything else.
func msgAskKind(m controller.Msg) askKind {
	switch m.(type) {
	case controller.PermissionAskMsg, controller.PermissionDismissMsg:
		return askPermission
	case controller.ContinueAskMsg, controller.ContinueDismissMsg:
		return askContinue
	case controller.QuestionAskMsg, controller.QuestionDismissMsg:
		return askQuestion
	}
	return askNone
}

// pendingAsk is one unanswered ask of the family: the session that raised it,
// the key its overlay state is stamped with, and the message the reply channel
// travels in.
type pendingAsk struct {
	origin *View
	owner  string
	kind   askKind
	msg    controller.Msg
}

// askOwner is the key an ask carries inside one family. It is stable for the
// session that raised it and different for every member, so a withdrawal or a
// dismissal lands on the ask it was meant for; the prefix keeps a job id from
// ever colliding with the key of the session that owns the family.
func askOwner(v *View) string {
	if v == nil {
		return ""
	}
	if v.childJobID == "" {
		return "parent"
	}
	return "child:" + v.childJobID
}

// askOrigin is the stamp one pending ask wears on a given screen: no label on
// the screen of the session that raised it, the child's row title on any other
// screen, and the name the registry gave the parent — "main" until the shell
// says otherwise — when it is the parent asking from behind a child's screen.
func (a pendingAsk) askOrigin(host *View) overlays.AskOrigin {
	origin := overlays.AskOrigin{Owner: a.owner, Child: a.origin.isChild()}
	if a.origin == host {
		return origin
	}
	switch {
	case origin.Child:
		origin.Label = a.origin.childTitle
	case a.origin.lifetime.slot != "":
		origin.Label = a.origin.lifetime.slot
	default:
		origin.Label = "main"
	}
	return origin
}

// raiseAsk takes one member's ask into the family and shows it if the screen
// is free. A ping goes out either way: the ask is why the session stopped, and
// the user has to be able to find it even while it waits its turn.
func (f *Family) raiseAsk(origin *View, m controller.Msg, detail string) {
	if f == nil || origin == nil {
		return
	}
	kind := msgAskKind(m)
	if kind == askNone {
		return
	}
	f.asks = append(f.asks, pendingAsk{origin: origin, owner: askOwner(origin), kind: kind, msg: m})
	f.showHead()
	// The ping has to name a session the user can reach: a sub-agent has no
	// tab, so the screen its family is on is the one that can be found.
	target := origin
	if origin.isChild() {
		if screen := f.Screen(); screen != nil {
			target = screen
		}
	}
	if target.notifier != nil {
		target.notifier.NeedsAttention(detail)
	}
}

// withdrawAsk drops the ask its own session gave up on — the call was
// cancelled, or answered somewhere else. Head or still queued, it leaves
// without an answer, and the next one takes the screen.
func (f *Family) withdrawAsk(origin *View, m controller.Msg) {
	if f == nil {
		return
	}
	kind := msgAskKind(m)
	for i, a := range f.asks {
		if a.origin != origin || a.kind != kind {
			continue
		}
		if i == 0 && f.host != nil {
			f.host.overlays.ApplyFrom(m, a.askOrigin(f.host))
			f.host.RequestRedraw()
			f.host, f.hostKey = nil, ""
		}
		f.drop(i)
		break
	}
	f.showHead()
}

// askResolved is the overlays seam: the ask at the head was answered on the
// screen that was showing it, and its reply is already on its way. The slot is
// free, so whatever was waiting behind it opens now.
func (f *Family) askResolved(owner string) {
	if f == nil || len(f.asks) == 0 || f.asks[0].owner != owner {
		return
	}
	f.host, f.hostKey = nil, ""
	f.drop(0)
	f.showHead()
}

// denyFrom answers no to every ask one member left open, on screen or in the
// queue. It is what a released sub-agent and a closing session leave behind:
// the call the ask guarded is gone, so it is denied rather than granted, and
// no panel outlives the session it belongs to.
func (f *Family) denyFrom(origin *View) {
	if f == nil {
		return
	}
	f.denyIf(func(a pendingAsk) bool { return a.origin == origin })
}

// denyAll answers no to every ask the family is holding, whoever raised it.
// The session that owns the family is closing: nothing here can still be
// answered, and a blocked call must not be left waiting for a screen.
func (f *Family) denyAll() {
	if f == nil {
		return
	}
	f.denyIf(func(pendingAsk) bool { return true })
}

// denyIf takes the matching asks off the screen first — a withdrawal answers
// nothing — and then denies them all through the channels they arrived on, so
// one path ends an ask whether or not it was the one being drawn.
func (f *Family) denyIf(match func(pendingAsk) bool) {
	if len(f.asks) > 0 && f.host != nil && match(f.asks[0]) {
		f.retireHost()
	}
	kept := f.asks[:0]
	for _, a := range f.asks {
		if !match(a) {
			kept = append(kept, a)
			continue
		}
		denyAsk(a.msg)
		f.clearWaiting(a.origin)
	}
	f.asks = kept
	f.showHead()
}

// showHead puts the ask at the head of the queue on the family's current
// screen, taking it off the screen it was on. The move answers nothing: the
// same ask, the same reply channel, a different overlay drawing it.
func (f *Family) showHead() {
	if f == nil {
		return
	}
	screen := f.Screen()
	if len(f.asks) == 0 {
		f.retireHost()
		return
	}
	head := f.asks[0]
	if f.host == screen && f.hostKey == head.owner {
		return
	}
	f.retireHost()
	if screen == nil || screen.Closed() {
		return
	}
	screen.overlays.ApplyFrom(head.msg, head.askOrigin(screen))
	f.host, f.hostKey = screen, head.owner
	screen.RequestRedraw()
}

// retireHost takes the ask on screen back out of the overlay that was drawing
// it, without answering it.
func (f *Family) retireHost() {
	if f.host == nil {
		return
	}
	f.host.overlays.Withdraw(f.hostKey)
	f.host.RequestRedraw()
	f.host, f.hostKey = nil, ""
}

// drop removes one pending ask and stops its session reporting that it waits,
// unless it left another ask behind in the queue.
func (f *Family) drop(i int) {
	origin := f.asks[i].origin
	f.asks = append(f.asks[:i], f.asks[i+1:]...)
	f.clearWaiting(origin)
}

// clearWaiting takes the ⏸ off a session's panel row once nothing of its is
// left in the queue. Answering an ask raises no message of its own, so this is
// the only place that can tell the row the wait is over.
func (f *Family) clearWaiting(origin *View) {
	if origin == nil {
		return
	}
	for _, a := range f.asks {
		if a.origin == origin {
			return
		}
	}
	origin.lifetime.status.Waiting = ""
}

// denyAsk answers one ask no without ever drawing it. The empty reply is what
// Escape sends; a full or absent channel means the runtime already stopped
// waiting for one.
func denyAsk(m controller.Msg) {
	switch msg := m.(type) {
	case controller.PermissionAskMsg:
		select {
		case msg.Reply <- controller.AskReply{}:
		default:
		}
	case controller.ContinueAskMsg:
		select {
		case msg.Reply <- controller.ContinueReply{}:
		default:
		}
	case controller.QuestionAskMsg:
		select {
		case msg.Reply <- controller.QuestionReply{}:
		default:
		}
	}
}
