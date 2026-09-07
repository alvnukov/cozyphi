package sessions

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tui/agentlist"
	"github.com/alvnukov/cozyphi/internal/tui/agentpanel"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/tui/pathutil"
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
	// asks is every unanswered ask of the family, oldest first. The head is
	// the one on screen; the rest wait their turn. See family_ask.go.
	asks []pendingAsk
	// host is the view whose overlay is drawing the head right now, and
	// hostKey the owner that ask was stamped with there.
	host    *View
	hostKey string
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
	// A question this child left open goes with it, denied: the call it
	// guards is gone, and a panel nobody can answer for would sit on the
	// screen the user is on until they pressed Escape.
	f.denyFrom(child)
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

// Show records which session the screen holds so the right row wears ●, and
// hands the family's open ask to that screen: a question follows the user
// instead of staying behind on a screen they have left. It does not move the
// screen itself — the shell does that, and calls this from both directions.
func (f *Family) Show(jobID string) {
	if f == nil {
		return
	}
	if _, ok := f.kids[jobID]; !ok {
		jobID = ""
	}
	f.current = jobID
	f.panel.SetCurrent(jobID)
	f.showHead()
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

// backFrom is the way out of a sub-agent's screen: the bottom rung of a child
// composer's Escape ladder leaves it the way Enter on the band's main row
// does, whether the child is still running or long finished — the run is left
// alone, stopping it is x in the band or Ctrl+C. It answers only for the child that is
// actually on screen, so the key never moves a session the user is not in —
// and never at all in the session that owns the family, which has nowhere to
// go back to and keeps Escape's old meaning.
func (f *Family) backFrom(jobID string) bool {
	if f == nil || jobID == "" || f.current != jobID {
		return false
	}
	f.open("")
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
// session means talking to it. A job this family no longer holds has no
// session left to show, so the user is told where its transcript went instead
// of watching the key do nothing.
func (f *Family) open(jobID string) {
	if f == nil {
		return
	}
	child, ok := f.kids[jobID]
	if jobID != "" && !ok {
		if v := f.Screen(); v != nil {
			v.Toast(f.releasedNotice(jobID), toast.ToastWarning, 6*time.Second)
		}
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

// releasedNotice says that a sub-agent's session is gone and names the
// directory its transcript was written to, so the answer is still one `ls`
// away. The path comes from the job manager's own root; a session without one
// says only that the sub-agent is closed rather than inventing a location.
func (f *Family) releasedNotice(jobID string) string {
	notice := "Sub-agent is no longer open"
	if f.parent == nil {
		return notice
	}
	dir := f.parent.ctrl.ChildJobDir(jobID)
	if dir == "" {
		return notice
	}
	return notice + "; transcript: " + pathutil.ShortPath(dir) + string(filepath.Separator)
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
// own keys while it holds the keyboard, the way out while a sub-agent's screen
// is open, and otherwise the panel's reminder that a child which finished is
// still reachable.
func (f *Family) footerHint() (string, bool) {
	if f == nil {
		return "", false
	}
	if f.panel.Focused() {
		return keys.Hints(keys.ScopeAgents), true
	}
	if f.current != "" {
		return keys.Hints(keys.ScopeChild), true
	}
	return f.panel.Hint()
}

// rows is the panel's seam onto the family: one row per retained child, named
// the way the parent's transcript names it, carrying the call the parent's
// sub-agent store saw last and the state the child's own view reports.
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

// row builds one child's row. Nothing here is invented: the action is the last
// tool call the store the transcript row reads has recorded, worded as that
// call's own row words it, and the state comes from the child's own status and
// the job's recorded outcome.
//
// The live session outranks the recorded outcome. A child the user typed into
// again runs under a linked follow-up assignment the parent's transcript run
// knows nothing about, so a terminal run there would leave the band claiming
// the child had finished while it works.
func (f *Family) row(jobID string, child *View) agentpanel.Row {
	row := agentpanel.Row{ID: jobID, Title: child.childTitle}
	var recorded agentpanel.State
	var haveRecorded bool
	if f.parent != nil && f.parent.transcript != nil {
		if run, ok := f.parent.transcript.SubagentRun(jobID); ok {
			row.Action = latestAction(run.Children)
			row.Ended = run.Finished
			haveRecorded = run.Status.Terminal()
			if haveRecorded {
				recorded = childState(run.Status)
			}
		}
	}
	status := child.Status()
	switch {
	case status.Running:
		// The recorded end belongs to the previous assignment; a row that
		// kept it would retire a child that is working.
		row.State, row.Ended = agentpanel.StateRunning, time.Time{}
	case status.Waiting != "":
		row.State, row.Waiting, row.Ended = agentpanel.StateWaiting, status.Waiting, time.Time{}
	case status.Stopped:
		// A follow-up runs under a linked assignment of its own, so the end
		// the parent recorded for this job id may be an older one; the live
		// session is the authority on how the child stands now.
		row.State = agentpanel.StateStopped
	case status.Error != "":
		row.State = agentpanel.StateFailed
	case haveRecorded:
		row.State = recorded
	default:
		row.State = agentpanel.StateRunning
	}
	return row
}

// latestAction names what a child is doing right now: the last tool call its
// run has recorded, in the words the transcript titles that call's row with. A
// child that has called nothing yet has no action, and none is invented for it.
func latestAction(children []block.ChildTool) string {
	if len(children) == 0 {
		return ""
	}
	return children[len(children)-1].Title()
}

// Agents is the /agents browser's seam onto this session's sub-agents: every
// child the job manager still has a record of, plus the ones this parent
// retains that the manager no longer lists. Nothing here is invented — the
// title, the counts, the clock and the summary all come from records somebody
// else wrote, and a job that never recorded a field leaves the row without it.
func (f *Family) Agents() []agentlist.Agent {
	if f == nil || f.parent == nil {
		return nil
	}
	var infos []job.Info
	if f.parent.ctrl != nil {
		// A session without a job manager simply has no children to list; the
		// browser then shows the ones this parent still holds, and no more.
		infos, _ = f.parent.ctrl.ChildJobs(context.Background())
	}
	listed := make(map[string]bool, len(infos))
	out := make([]agentlist.Agent, 0, len(infos)+len(f.order))
	for _, info := range infos {
		listed[info.ID] = true
		out = append(out, f.agent(info))
	}
	for _, id := range f.order {
		if v := f.kids[id]; v != nil && !listed[id] {
			out = append(out, f.retainedAgent(id, v))
		}
	}
	return out
}

// agent builds one row from the job's own record, adding only what this session
// knows on top of it: whether the child's session is still open, what it is
// blocked on, and how many tool rows the parent's transcript counted.
func (f *Family) agent(info job.Info) agentlist.Agent {
	a := agentlist.Agent{
		JobID:      info.ID,
		Title:      tools.SpawnTitle(string(info.Role), info.Description, info.Prompt),
		Status:     info.Status,
		Tools:      f.toolCount(info.ID),
		Created:    info.CreatedAt,
		Started:    info.StartedAt,
		Finished:   info.FinishedAt,
		Summary:    info.OutcomeSummary,
		Error:      info.Error,
		ResultPath: resultLocation(info.Meta),
	}
	if child, ok := f.Child(info.ID); ok {
		a.Retained = true
		a.Waiting = child.Status().Waiting
	}
	return a
}

// retainedAgent builds the row of a child this parent holds but the manager no
// longer lists — a job record pruned under a session that is still open. Its
// facts come from the parent's transcript and the child's own view, the same
// two sources the band reads.
func (f *Family) retainedAgent(jobID string, child *View) agentlist.Agent {
	a := agentlist.Agent{JobID: jobID, Title: child.childTitle, Retained: true}
	if f.parent.transcript != nil {
		if run, ok := f.parent.transcript.SubagentRun(jobID); ok {
			a.Tools = len(run.Children)
			a.Created, a.Started, a.Finished = run.Started, run.Started, run.Finished
			a.Status, a.Summary, a.Error = run.Status, run.Summary, run.Error
		}
	}
	if a.Status.Terminal() {
		return a
	}
	switch status := child.Status(); {
	case status.Stopped:
		a.Status = job.StatusCancelled
	case status.Error != "":
		a.Status, a.Error = job.StatusFailed, status.Error
	default:
		a.Waiting = status.Waiting
	}
	return a
}

// toolCount is how many tool rows the parent's transcript has seen this child
// make; 0 for a run this process never watched.
func (f *Family) toolCount(jobID string) int {
	if f.parent.transcript == nil {
		return 0
	}
	run, ok := f.parent.transcript.SubagentRun(jobID)
	if !ok {
		return 0
	}
	return len(run.Children)
}

// resultLocation is where a released child's answer can still be read: the
// result file when the job wrote one, else the directory that holds its record.
func resultLocation(m job.Meta) string {
	if m.ResultPath != "" {
		return m.ResultPath
	}
	return m.Dir
}

// OpenAgent shows one retained child as the current screen — the very path the
// band's Enter takes, so the browser and the band cannot mean different things.
func (f *Family) OpenAgent(jobID string) { f.open(jobID) }

// StopAgent stops a running child through the manager path agent_cancel uses.
func (f *Family) StopAgent(jobID string) error { return f.stop(jobID) }

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
