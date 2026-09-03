// The projection is the bounded, decision-rich model-facing view of the
// durable plan: one renderer behind the plan tool's get-active answer. It
// always carries the header contract, progress, the active and blocked steps
// in full, collapsed completed outcomes, and the nearest pending steps. A
// finished plan projects as one terminal line — result, close time, goal,
// progress — because its contract is discharged. The audit trail, raw tool
// output, and evidence history stay durable and are served only by get full.

package plangate

import (
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/alvnukov/cozyphi/internal/redact"
	"github.com/alvnukov/cozyphi/internal/session"
)

// maxProjectionBytes is the one budget every model-facing plan view obeys.
// The durable snapshot is allowed 96 KiB; the projection is the decision
// view injected on every inference, so it stays a fraction of that.
const maxProjectionBytes = 8 * 1024

// View windows applied while building, before any truncation. Under budget
// pressure the ladder below shrinks them further.
const (
	pendingWindow       = 4
	completedWindow     = 8
	blockedWindow       = 4
	activeAttemptWindow = 2
)

// fitFloorRunes is the smallest text a fit-to-budget rung may leave in place;
// below it the next rung takes over rather than emptying a field to nothing.
const fitFloorRunes = 64

// fitFloorBytes is the byte floor of the escape pass. Durable prose caps are
// runes, so wide-rune plans multiply them into bytes; the escape floors every
// remaining text to this many bytes, which keeps even the widest durable plan
// far inside the budget.
const fitFloorBytes = 64

// Projection is the compact plan the model sees. Revision and approval drive
// the gate contract; progress orients; the header keeps the authored
// contract; the step lists carry work in decreasing closeness. Elided counts
// whatever the budget shed, so nothing disappears silently.
type Projection struct {
	Revision        uint64          `json:"revision"`
	Approved        bool            `json:"approved"`
	Result          string          `json:"result,omitempty"`
	ClosedAt        *time.Time      `json:"closedAt,omitempty"`
	Progress        *planProgress   `json:"progress,omitempty"`
	Goal            string          `json:"goal,omitempty"`
	Approach        string          `json:"approach,omitempty"`
	SuccessCriteria []string        `json:"successCriteria,omitempty"`
	Constraints     []string        `json:"constraints,omitempty"`
	WorkingContext  string          `json:"workingContext,omitempty"`
	Active          *stepView       `json:"active,omitempty"`
	Blocked         []stepView      `json:"blocked,omitempty"`
	Completed       []collapsedStep `json:"completed,omitempty"`
	Next            []stepView      `json:"next,omitempty"`
	Elided          *elidedCounts   `json:"elided,omitempty"`
}

// planProgress counts the lifecycle states so the model can orient without
// reading step lists. Done merges completed and cancelled: neither is work.
type planProgress struct {
	Total   int `json:"total,omitempty"`
	Done    int `json:"done,omitempty"`
	Active  int `json:"active,omitempty"`
	Blocked int `json:"blocked,omitempty"`
	Pending int `json:"pending,omitempty"`
}

// stepView is one live step: identity, the work, and what ends it. Blocked
// steps add their blocker and resume condition; the active step adds its
// citable attempts. Timestamps and evidence refs are audit — get full.
type stepView struct {
	ID         string        `json:"id,omitempty"`
	Content    string        `json:"content"`
	Status     string        `json:"status"`
	Type       string        `json:"type,omitempty"`
	Why        string        `json:"why,omitempty"`
	DoneWhen   string        `json:"doneWhen,omitempty"`
	Risk       string        `json:"risk,omitempty"`
	Note       string        `json:"note,omitempty"`
	JIT        bool          `json:"jit,omitempty"`
	Blocker    string        `json:"blocker,omitempty"`
	ResumeWhen string        `json:"resumeWhen,omitempty"`
	Skills     []skillView   `json:"skills,omitempty"`
	Attempts   []attemptView `json:"attempts,omitempty"`
}

// skillView reports one recommended skill on a step and whether the user's
// toggle switched it off: the model authored the list, the user edits the
// marks, and the projection shows both so the model knows what will actually
// be injected.
type skillView struct {
	Name string `json:"name"`
	Off  bool   `json:"off,omitempty"`
}

// attemptView is the citable evidence of one accepted call: the call id the
// model names as call:<callId> in complete, the tool, the terminal status,
// and the bounded summary. The timestamp is audit and stays in get full.
type attemptView struct {
	CallID  string `json:"callId"`
	Tool    string `json:"tool"`
	Status  string `json:"status"`
	Summary string `json:"summary,omitempty"`
}

// collapsedStep is a finished step: identity and outcome only. Legacy steps
// carry content instead — they have no id or outcome to collapse to.
type collapsedStep struct {
	ID      string `json:"id,omitempty"`
	Content string `json:"content,omitempty"`
	Status  string `json:"status"`
	Outcome string `json:"outcome,omitempty"`
}

// elidedCounts names what the budget shed. Every rung increments its slot,
// so the projection never hides plan size by silent omission. Escaped reports
// the byte-floor escape pass truncated remaining prose — wide-rune plans only.
type elidedCounts struct {
	Pending         int  `json:"pending,omitempty"`
	Completed       int  `json:"completed,omitempty"`
	Blocked         int  `json:"blocked,omitempty"`
	BlockedProse    int  `json:"blockedProse,omitempty"`
	Attempts        int  `json:"attempts,omitempty"`
	ActiveDetail    bool `json:"activeDetail,omitempty"`
	SuccessCriteria int  `json:"successCriteria,omitempty"`
	Constraints     int  `json:"constraints,omitempty"`
	WorkingContext  bool `json:"workingContext,omitempty"`
	Approach        bool `json:"approach,omitempty"`
	Goal            bool `json:"goal,omitempty"`
	Escaped         bool `json:"escaped,omitempty"`
}

// Project renders the projection of a durable plan: full windows first, then
// the truncation ladder until the serialized body fits the budget. It is pure
// and deterministic — the same plan always projects to the same bytes.
func Project(plan session.Plan) Projection {
	p := buildProjection(plan)
	p.trimToBudget()
	return p
}

func buildProjection(plan session.Plan) Projection {
	closed := plan.Result != ""
	p := Projection{
		Revision: plan.Revision,
		Approved: plan.Approved,
	}
	if closed {
		// The finished plan leaves one bounded terminal view: how it ended,
		// when, and the work count. Step lists, directives and context are
		// working views of a contract that is discharged; get full still
		// serves the whole record.
		p.Goal = plan.Goal
		p.Result = string(plan.Result)
		p.ClosedAt = plan.ClosedAt
	} else {
		p.Goal = plan.Goal
		p.Approach = plan.Approach
		p.SuccessCriteria = plan.SuccessCriteria
		p.Constraints = plan.Constraints
		p.WorkingContext = plan.WorkingContext
	}
	progress := planProgress{Total: len(plan.Items)}
	var completed, upcoming []session.PlanItem
	for _, item := range plan.Items {
		switch item.Status {
		case session.PlanInProgress:
			progress.Active++
			if p.Active == nil && !closed {
				view := fullStepView(item)
				p.Active = &view
				continue
			}
			upcoming = append(upcoming, item) // craftable only via legacy update
		case session.PlanBlocked:
			progress.Blocked++
			if !closed {
				p.Blocked = append(p.Blocked, fullStepView(item))
			}
		default:
			// Terminal statuses are done; anything else still owes work.
			if item.Status.Terminal() {
				progress.Done++
				completed = append(completed, item)
				continue
			}
			progress.Pending++
			upcoming = append(upcoming, item)
		}
	}
	if progress.Total > 0 {
		p.Progress = &progress
	}
	if closed {
		return p
	}

	p.Next = briefStepViews(upcoming, pendingWindow)
	if dropped := len(upcoming) - min(pendingWindow, len(upcoming)); dropped > 0 {
		p.elided().Pending = dropped
	}
	p.Completed = collapseSteps(completed, completedWindow)
	if dropped := len(completed) - min(completedWindow, len(completed)); dropped > 0 {
		p.elided().Completed = dropped
	}
	if len(p.Blocked) > blockedWindow {
		p.elided().Blocked = len(p.Blocked) - blockedWindow
		p.Blocked = p.Blocked[:blockedWindow]
	}
	return p
}

// fullStepView carries everything a live step needs to be executed and
// judged: action, why, done_when, risk, blocker, and citable attempts.
func fullStepView(item session.PlanItem) stepView {
	view := stepView{
		ID: item.ID, Content: item.Content, Status: string(item.Status), Type: string(item.Type),
		Why: item.Why, DoneWhen: item.DoneWhen, Risk: item.Risk, Note: item.Note, JIT: item.JIT,
		Blocker: item.Blocker, ResumeWhen: item.ResumeWhen,
		Skills: stepSkillViews(item.Actions),
	}
	for _, attempt := range item.Attempts {
		view.Attempts = append(view.Attempts, attemptView{
			CallID: attempt.CallID, Tool: attempt.Tool, Status: attempt.Status,
			// Defense in depth: the session masks summaries at write time; the
			// renderer masks again so a legacy snapshot cannot leak through.
			Summary: redact.Redact(attempt.Summary),
		})
	}
	return view
}

// stepSkillViews projects the step's injected skills with the user's off
// marks. The inject action is the single source of truth: the model authors
// the list, DisabledSkills is the toggle, and EffectiveSkills (what the
// engine injects) is exactly the names without an off mark.
func stepSkillViews(actions []session.PlanAction) []skillView {
	for _, action := range actions {
		if action.Type != session.PlanActionInjectSkill {
			continue
		}
		if len(action.Skills) == 0 {
			return nil
		}
		off := make(map[string]struct{}, len(action.DisabledSkills))
		for _, name := range action.DisabledSkills {
			off[name] = struct{}{}
		}
		views := make([]skillView, 0, len(action.Skills))
		for _, name := range action.Skills {
			_, disabled := off[name]
			views = append(views, skillView{Name: name, Off: disabled})
		}
		return views
	}
	return nil
}

// briefStepViews renders the nearest upcoming steps minimally — id, work,
// status, type. Why and done_when arrive the moment a step becomes active.
func briefStepViews(items []session.PlanItem, window int) []stepView {
	if len(items) > window {
		items = items[:window]
	}
	views := make([]stepView, 0, len(items))
	for _, item := range items {
		views = append(views, stepView{
			ID: item.ID, Content: item.Content, Status: string(item.Status), Type: string(item.Type),
		})
	}
	return views
}

// collapseSteps keeps the newest finished steps as id+outcome (content for
// legacy steps), newest first; the older ones were counted by the caller.
func collapseSteps(items []session.PlanItem, window int) []collapsedStep {
	keep := min(window, len(items))
	views := make([]collapsedStep, 0, keep)
	for i := len(items) - 1; i >= len(items)-keep; i-- {
		item := items[i]
		view := collapsedStep{ID: item.ID, Status: string(item.Status), Outcome: item.Outcome}
		if item.ID == "" {
			view = collapsedStep{Content: item.Content, Status: string(item.Status)}
		}
		views = append(views, view)
	}
	return views
}

// trimRung is one step of the truncation ladder. The ladder is the documented
// priority order: rung 0 is cut first, and the rungs a plan never reaches
// never touch it. The ordinary ladder never touches the active step's why,
// done_when, or content, nor any id, status, or call id — the byte-floor
// escape pass below is the only last resort for those.
type trimRung struct {
	apply func(p *Projection)
}

var trimLadder = []trimRung{
	{func(p *Projection) { p.elided().Pending += p.cutNext(1) }},
	{func(p *Projection) {
		for i := range p.Blocked {
			p.elided().Attempts += len(p.Blocked[i].Attempts)
			p.Blocked[i].Attempts = nil
		}
	}},
	{func(p *Projection) { p.elided().Completed += p.cutCompleted(2) }},
	{func(p *Projection) { p.elided().Attempts += p.cutActiveAttempts(activeAttemptWindow) }},
	{func(p *Projection) { p.elided().Pending += p.cutNext(0) }},
	{func(p *Projection) { p.elided().Completed += p.cutCompleted(0) }},
	{func(p *Projection) { p.elided().SuccessCriteria += cutStringTail(&p.SuccessCriteria) }},
	{func(p *Projection) { p.elided().Constraints += cutStringTail(&p.Constraints) }},
	{func(p *Projection) {
		if p.Active != nil && (p.Active.Note != "" || p.Active.Risk != "") {
			p.Active.Note, p.Active.Risk = "", ""
			p.elided().ActiveDetail = true
		}
	}},
	{func(p *Projection) { p.elided().BlockedProse += p.demoteBlockedToBrief() }},
	{func(p *Projection) { p.elided().Blocked += p.cutBlockedWindow(1) }},
	{func(p *Projection) { e := p.elided(); p.fitText(&p.WorkingContext, &e.WorkingContext) }},
	{func(p *Projection) { e := p.elided(); p.fitText(&p.Approach, &e.Approach) }},
	{func(p *Projection) { e := p.elided(); p.fitText(&p.Goal, &e.Goal) }},
}

// trimToBudget walks the ladder until the serialized body fits. Every rung
// either sheds bytes or leaves the projection untouched, so the walk always
// terminates; the escape pass backstops the ladder so the budget is a hard
// invariant even for wide-rune plans whose rune caps multiply into bytes.
func (p *Projection) trimToBudget() {
	if !p.overBudget() {
		return
	}
	for _, rung := range trimLadder {
		rung.apply(p)
		if !p.overBudget() {
			return
		}
	}
	p.escapeToBudget()
}

// escapeToBudget is the hard backstop: with every ordinary rung spent and the
// body still over budget, it byte-truncates the remaining prose in documented
// last-resort order — header prose, directives, goal, blocked prose, then the
// active step's summaries and prose — each down to fitFloorBytes, re-checking
// after every cut. Ids, statuses, types, and attempt call ids are never cut:
// the citation keys must survive. The floors sum to a fraction of the budget,
// so the pass always lands inside it.
func (p *Projection) escapeToBudget() {
	if !p.overBudget() {
		return
	}
	var targets []*string
	targets = append(targets, &p.WorkingContext, &p.Approach)
	for i := range p.SuccessCriteria {
		targets = append(targets, &p.SuccessCriteria[i])
	}
	for i := range p.Constraints {
		targets = append(targets, &p.Constraints[i])
	}
	targets = append(targets, &p.Goal)
	for i := range p.Blocked {
		b := &p.Blocked[i]
		targets = append(targets, &b.Why, &b.DoneWhen, &b.Risk, &b.Note, &b.Content, &b.ResumeWhen, &b.Blocker)
	}
	if p.Active != nil {
		for i := range p.Active.Attempts {
			targets = append(targets, &p.Active.Attempts[i].Summary)
		}
		targets = append(targets,
			&p.Active.Note, &p.Active.Risk, &p.Active.Why, &p.Active.DoneWhen, &p.Active.Content)
	}
	for _, target := range targets {
		if !p.overBudget() {
			return
		}
		if truncateBytes(target, fitFloorBytes) {
			p.elided().Escaped = true
		}
	}
}

// demoteBlockedToBrief reduces every blocked step to identity, work, status,
// type, blocker, and resume condition — dropping why, done_when, risk, and
// note — and returns how many steps it changed.
func (p *Projection) demoteBlockedToBrief() int {
	demoted := 0
	for i := range p.Blocked {
		b := &p.Blocked[i]
		if b.Why == "" && b.DoneWhen == "" && b.Risk == "" && b.Note == "" {
			continue
		}
		b.Why, b.DoneWhen, b.Risk, b.Note = "", "", "", ""
		demoted++
	}
	return demoted
}

// cutBlockedWindow shrinks the blocked-steps window and returns how many
// dropped. The window keeps the first blocked steps in plan order.
func (p *Projection) cutBlockedWindow(window int) int {
	keep := min(window, len(p.Blocked))
	dropped := len(p.Blocked) - keep
	p.Blocked = p.Blocked[:keep]
	return dropped
}

func (p *Projection) overBudget() bool {
	body, err := json.Marshal(p)
	return err != nil || len(body) > maxProjectionBytes
}

func (p *Projection) elided() *elidedCounts {
	if p.Elided == nil {
		p.Elided = &elidedCounts{}
	}
	return p.Elided
}

// cutNext shrinks the nearest-steps window and returns how many dropped.
func (p *Projection) cutNext(window int) int {
	dropped := len(p.Next) - min(window, len(p.Next))
	p.Next = p.Next[:len(p.Next)-dropped]
	return dropped
}

// cutCompleted shrinks the finished-steps window and returns how many
// dropped. The window keeps the newest outcomes.
func (p *Projection) cutCompleted(window int) int {
	keep := min(window, len(p.Completed))
	dropped := len(p.Completed) - keep
	p.Completed = p.Completed[:keep]
	return dropped
}

// cutActiveAttempts keeps the newest attempts on the active step — the ones
// the model can still cite — and counts the rest.
func (p *Projection) cutActiveAttempts(window int) int {
	if p.Active == nil {
		return 0
	}
	keep := min(window, len(p.Active.Attempts))
	dropped := len(p.Active.Attempts) - keep
	p.Active.Attempts = p.Active.Attempts[keep:]
	return dropped
}

// cutStringTail drops a directive list to its first two entries and returns
// how many went.
func cutStringTail(list *[]string) int {
	const keep = 2
	if len(*list) <= keep {
		return 0
	}
	dropped := len(*list) - keep
	*list = (*list)[:keep]
	return dropped
}

// fitText shrinks a header field until the body fits: twenty percent at a
// time down to the floor, each cut marked with an ellipsis. The elided flag
// is set only when the field actually shrank.
func (p *Projection) fitText(field *string, marked *bool) {
	if *field == "" {
		return
	}
	before := *field
	runes := utf8.RuneCountInString(*field)
	for p.overBudget() && runes > fitFloorRunes {
		runes = max(runes*4/5, fitFloorRunes)
		*field = truncateRunes(*field, runes) + "…"
	}
	if *field != before {
		*marked = true
	}
}

func truncateRunes(s string, keep int) string {
	runes := []rune(s)
	if len(runes) <= keep {
		return s
	}
	return string(runes[:keep])
}

// truncateBytes byte-truncates s in place to at most floor bytes on a rune
// boundary plus an ellipsis, reporting whether it cut anything.
func truncateBytes(s *string, floor int) bool {
	if len(*s) <= floor {
		return false
	}
	cut := []byte((*s)[:floor])
	for len(cut) > 0 && !utf8.RuneStart(cut[len(cut)-1]) {
		cut = cut[:len(cut)-1]
	}
	*s = string(cut) + "…"
	return true
}
