package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/tui/agentpanel"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

// familyCap is how many child views one parent keeps. It matches the
// controller's own retention budget: the runtime drops the record of a
// released child and this side retires the view built around it.
const familyCap = 12

// Family is one parent session's sub-agents. They are never tabs: the parent
// holds their views itself, shows one of them as the current screen when the
// user opens a row, and draws a single agent panel — this one — on the parent
// screen and on every child screen alike.
//
// The family owns no timers and no goroutines. The panel it holds is a dumb
// widget: it reads rows through the seam below on every frame, and hands
// opening, stopping and leaving back through actions.
type Family struct {
	parent *View
	// order is adoption order, which is the order the rows are drawn in.
	order []string
	kids  map[string]*View
	panel *agentpanel.Panel
	// current is the job id of the session on screen; "" is the parent.
	current string
	// onShow asks the shell to change the current screen. A nil argument
	// means the parent's own screen. Unwired, opening a row does nothing —
	// which is what a headless or test assembly wants.
	onShow func(child *View)
}

// newFamily builds a parent's empty family and the panel that draws it.
func newFamily(parent *View, now func() time.Time) *Family {
	f := &Family{parent: parent, kids: map[string]*View{}}
	f.panel = agentpanel.New(parent.theme, f.rows, now, agentpanel.Actions{
		Open:  f.open,
		Stop:  f.stop,
		Leave: f.leave,
	})
	return f
}

// Family is the family this view draws: its own for a parent session, and the
// parent's for a child, so both screens show the same band.
func (e *View) Family() *Family {
	if e == nil {
		return nil
	}
	return e.family
}

// SetOnShow installs the shell's screen seam: the family asks for a child's
// view to become the current screen, or for nil to go back to the parent.
func (f *Family) SetOnShow(show func(child *View)) {
	if f != nil {
		f.onShow = show
	}
}

// Adopt takes a newly built child view into the family. The child draws the
// family's panel from then on, and its own composer leaves down into it.
func (f *Family) Adopt(jobID, title string, child *View) error {
	if f == nil || child == nil {
		return errors.New("cannot retain sub-agent: no session to attach it to")
	}
	if jobID == "" {
		return errors.New("cannot retain sub-agent: the assignment has no id")
	}
	if _, ok := f.kids[jobID]; ok {
		return fmt.Errorf("sub-agent %s is already retained", jobID)
	}
	if len(f.order) >= familyCap {
		return fmt.Errorf("cannot retain sub-agent: this session already holds %d", familyCap)
	}
	child.family = f
	child.childJobID = jobID
	child.childTitle = title
	child.bindFamily()
	f.kids[jobID] = child
	f.order = append(f.order, jobID)
	f.parent.RequestRedraw()
	return nil
}

// Release removes a child the runtime no longer retains and hands its view
// back for closing. The screen falls back to the parent when the child being
// released is the one on it.
func (f *Family) Release(jobID string) (*View, bool) {
	if f == nil {
		return nil, false
	}
	child, ok := f.kids[jobID]
	if !ok {
		return nil, false
	}
	if f.current == jobID {
		f.Show("")
	}
	delete(f.kids, jobID)
	for i, id := range f.order {
		if id == jobID {
			f.order = append(f.order[:i], f.order[i+1:]...)
			break
		}
	}
	f.parent.RequestRedraw()
	return child, true
}

// Has reports whether this job is already retained.
func (f *Family) Has(jobID string) bool {
	if f == nil {
		return false
	}
	_, ok := f.kids[jobID]
	return ok
}

// Len counts the retained children.
func (f *Family) Len() int {
	if f == nil {
		return 0
	}
	return len(f.order)
}

// Child returns one retained child's view.
func (f *Family) Child(jobID string) (*View, bool) {
	if f == nil {
		return nil, false
	}
	v, ok := f.kids[jobID]
	return v, ok
}

// Views returns every retained child's view in adoption order.
func (f *Family) Views() []*View {
	if f == nil {
		return nil
	}
	out := make([]*View, 0, len(f.order))
	for _, id := range f.order {
		if v := f.kids[id]; v != nil {
			out = append(out, v)
		}
	}
	return out
}

// RunningNames names the children still working, the way the shell wants them
// for the notice that refuses an exit.
func (f *Family) RunningNames() []string {
	var out []string
	for _, id := range f.order {
		v := f.kids[id]
		if v != nil && v.Status().Running {
			out = append(out, v.childTitle)
		}
	}
	return out
}

// Screen is the view the family is currently showing: a child when one is
// open, the parent otherwise.
func (f *Family) Screen() *View {
	if f == nil {
		return nil
	}
	if v, ok := f.kids[f.current]; ok && v != nil {
		return v
	}
	return f.parent
}

// Current is the job id on screen; "" means the parent's own screen.
func (f *Family) Current() string {
	if f == nil {
		return ""
	}
	return f.current
}

// Show records which session the screen holds so the right row wears ●. It
// does not move the screen itself: the shell does that, and calls this.
func (f *Family) Show(jobID string) {
	if f == nil {
		return
	}
	if _, ok := f.kids[jobID]; !ok {
		jobID = ""
	}
	f.current = jobID
	f.panel.SetCurrent(jobID)
}

// SetTheme repaints the shared panel when the parent's theme changes.
func (f *Family) SetTheme(t components.Theme) {
	if f != nil {
		f.panel.SetTheme(t)
	}
}

// Focused reports whether the band holds the keyboard.
func (f *Family) Focused() bool { return f != nil && f.panel.Focused() }

// Draw paints the band. It is called on every frame, visible or not: the
// panel schedules the wakes its own windows need from inside Draw, so a frame
// it never gets is a deadline it never meets.
func (f *Family) Draw(ctx components.DrawContext, width int) components.Surface {
	if f == nil {
		return components.NewSurface(max(width, 0), 0, nil)
	}
	return f.panel.Draw(ctx, width)
}

// HandleEvent gives the band one event: a key while it is focused, or a mouse
// event the view has already made panel-relative.
func (f *Family) HandleEvent(ctx *components.EventContext, ev xui.Event) bool {
	if f == nil {
		return false
	}
	return f.panel.HandleEvent(ctx, ev)
}

// enter moves the keyboard into the band. It refuses when the band has
// nothing to show, so the composer keeps a Down key that would land nowhere.
func (f *Family) enter() bool {
	if f == nil || !f.panel.Visible() {
		return false
	}
	f.panel.Focus()
	if v := f.Screen(); v != nil {
		// Keys reach the band through the view's own ladder, so the view root
		// takes focus while the band holds it.
		v.FocusEditor()
	}
	return true
}

// leave hands the keyboard back to the composer of the session on screen.
func (f *Family) leave() {
	if f == nil {
		return
	}
	f.panel.Blur()
	if v := f.Screen(); v != nil && v.composer != nil {
		v.composer.FocusChat()
	}
}

// open shows a row's session: a child's own screen, or the parent's for the
// main row. The band gives the keyboard back on the way, because opening a
// session means talking to it.
func (f *Family) open(jobID string) {
	if f == nil {
		return
	}
	child, ok := f.kids[jobID]
	if jobID != "" && !ok {
		return
	}
	f.panel.Blur()
	f.Show(jobID)
	if f.onShow != nil {
		if jobID == "" {
			f.onShow(nil)
		} else {
			f.onShow(child)
		}
	}
	if v := f.Screen(); v != nil && v.composer != nil {
		v.composer.FocusChat()
	}
}

// stop cancels a running child through the manager path agent_cancel uses, so
// the panel's x and the model's tool cannot mean different things.
func (f *Family) stop(jobID string) error {
	if f == nil || f.parent == nil || f.parent.ctrl == nil {
		return errors.New("sub-agents are not available in this session")
	}
	return f.parent.ctrl.CancelChild(context.Background(), jobID)
}

// footerHint is what the family puts on the footer's right edge: the band's
// own keys while it holds the keyboard, and afterwards the panel's reminder
// that a child which finished is still reachable.
func (f *Family) footerHint() (string, bool) {
	if f == nil {
		return "", false
	}
	if f.panel.Focused() {
		return keys.Hints(keys.ScopeAgents), true
	}
	return f.panel.Hint()
}

// rows is the panel's seam onto the family: one row per retained child, named
// the way the parent's transcript names it, with the counts the parent's
// sub-agent store already keeps and the state the child's own view reports.
func (f *Family) rows() []agentpanel.Row {
	if f == nil {
		return nil
	}
	out := make([]agentpanel.Row, 0, len(f.order))
	for _, id := range f.order {
		v := f.kids[id]
		if v == nil {
			continue
		}
		out = append(out, f.row(id, v))
	}
	return out
}

// row builds one child's row. Nothing here is invented: the counts and the
// clock come from the store the transcript row reads, and the state comes
// from the child's own status and the job's recorded outcome.
func (f *Family) row(jobID string, child *View) agentpanel.Row {
	row := agentpanel.Row{ID: jobID, Title: child.childTitle}
	if f.parent != nil && f.parent.transcript != nil {
		if run, ok := f.parent.transcript.SubagentRun(jobID); ok {
			row.Tools = len(run.Children)
			row.Started = run.Started
			row.Ended = run.Finished
			if run.Status.Terminal() {
				row.State = childState(run.Status)
				return row
			}
		}
	}
	status := child.Status()
	switch {
	case status.Stopped:
		row.State = agentpanel.StateStopped
	case status.Error != "":
		row.State = agentpanel.StateFailed
	case status.Waiting != "":
		row.State, row.Waiting = agentpanel.StateWaiting, status.Waiting
	default:
		row.State = agentpanel.StateRunning
	}
	return row
}

// childState maps a terminal job status onto the row's end state. A timeout
// is a failure to the user: the child did not come back with an answer.
func childState(s job.Status) agentpanel.State {
	switch s {
	case job.StatusCompleted:
		return agentpanel.StateDone
	case job.StatusCancelled:
		return agentpanel.StateStopped
	default:
		return agentpanel.StateFailed
	}
}

// SyncIdentities labels every child with the parent's opening number and its
// own name, so a child screen says which session it belongs to.
func (f *Family) SyncIdentities(number int) {
	if f == nil {
		return
	}
	for _, id := range f.order {
		if v := f.kids[id]; v != nil {
			v.SetIdentity(number, v.childTitle)
		}
	}
}

// ChildTitle is the name this view carries in its parent's band; "" for a
// session that is not a child.
func (e *View) ChildTitle() string {
	if e == nil {
		return ""
	}
	return e.childTitle
}

// ChildJobID is the assignment this view was built for; "" for a session the
// user opened.
func (e *View) ChildJobID() string {
	if e == nil {
		return ""
	}
	return e.childJobID
}

// SessionID is the conversation this view drives. A child names its parent by
// this id, which is how the shell finds the family a new sub-agent belongs to.
func (e *View) SessionID() string {
	if e == nil || e.ctrl == nil {
		return ""
	}
	return e.ctrl.SessionID()
}
