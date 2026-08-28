// Package planedit renders the durable plan viewer/editor modal. Pane owns
// only an editable draft and interaction state; the revision-guarded patch
// round trip stays behind Store.
package planedit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/session"
)

// Store is the persistence seam: Snapshot reads the durable plan, Apply
// atomically patches it against the expected revision. A stale revision
// returns an error that names the actual one.
type Store interface {
	Snapshot() session.Plan
	Apply(ctx context.Context, expectedRevision uint64, ops []session.PlanPatchOp) (session.Plan, error)
}

// State is a detached behavioral snapshot for shell integration and tests.
type State struct {
	Selected int
	Scroll   int
	Overflow bool
	Dirty    bool
	Error    string
	Editing  bool
	Readonly bool
}

// fieldKind names one editable slot in the draft. Status, outcome, evidence
// and ids belong to lifecycle transitions and never appear here.
type fieldKind uint8

const (
	fieldGoal fieldKind = iota
	fieldApproach
	fieldContext
	fieldCriterion
	fieldConstraint
	fieldContent
	fieldWhy
	fieldDoneWhen
	fieldNote
	fieldRisk
)

// fieldRef addresses one editable slot: a plan-level field, a directive by
// index, or a step field by draft step index.
type fieldRef struct {
	kind fieldKind
	step int // draft.Steps index; -1 for plan-level fields
	idx  int // criterion/constraint index
}

var noField = fieldRef{kind: fieldGoal, step: -1}

func (r fieldRef) label() string {
	switch r.kind {
	case fieldGoal:
		return "goal"
	case fieldApproach:
		return "approach"
	case fieldContext:
		return "context"
	case fieldCriterion:
		return fmt.Sprintf("criterion %d", r.idx+1)
	case fieldConstraint:
		return fmt.Sprintf("constraint %d", r.idx+1)
	case fieldContent:
		return "content"
	case fieldWhy:
		return "why"
	case fieldDoneWhen:
		return "done when"
	case fieldNote:
		return "note"
	case fieldRisk:
		return "risk"
	}
	return "?"
}

func (r fieldRef) get(d Draft) string {
	switch r.kind {
	case fieldGoal:
		return d.Goal
	case fieldApproach:
		return d.Approach
	case fieldContext:
		return d.WorkingContext
	case fieldCriterion:
		if r.idx >= 0 && r.idx < len(d.SuccessCriteria) {
			return d.SuccessCriteria[r.idx]
		}
	case fieldConstraint:
		if r.idx >= 0 && r.idx < len(d.Constraints) {
			return d.Constraints[r.idx]
		}
	case fieldContent:
		if s, ok := d.step(r.step); ok {
			return s.Content
		}
	case fieldWhy:
		if s, ok := d.step(r.step); ok {
			return s.Why
		}
	case fieldDoneWhen:
		if s, ok := d.step(r.step); ok {
			return s.DoneWhen
		}
	case fieldNote:
		if s, ok := d.step(r.step); ok {
			return s.Note
		}
	case fieldRisk:
		if s, ok := d.step(r.step); ok {
			return s.Risk
		}
	}
	return ""
}

// set stores a trimmed value after local validation. Required slots refuse an
// empty value here, before the store round trip; optional slots accept a
// clear. Server-side length limits still apply and surface on Apply.
func (r fieldRef) set(d *Draft, value string) error {
	value = strings.TrimSpace(value)
	required := map[fieldKind]bool{
		fieldGoal: true, fieldCriterion: true, fieldConstraint: true,
		fieldContent: true, fieldWhy: true, fieldDoneWhen: true,
	}
	if required[r.kind] && value == "" {
		return fmt.Errorf("planedit: %s cannot be empty", r.label())
	}
	switch r.kind {
	case fieldGoal:
		d.Goal = value
	case fieldApproach:
		d.Approach = value
	case fieldContext:
		d.WorkingContext = value
	case fieldCriterion:
		if r.idx < 0 || r.idx >= len(d.SuccessCriteria) {
			return errors.New("planedit: criterion out of range")
		}
		d.SuccessCriteria[r.idx] = value
	case fieldConstraint:
		if r.idx < 0 || r.idx >= len(d.Constraints) {
			return errors.New("planedit: constraint out of range")
		}
		d.Constraints[r.idx] = value
	case fieldContent:
		if _, ok := d.step(r.step); !ok {
			return errors.New("planedit: step out of range")
		}
		d.Steps[r.step].Content = value
	case fieldWhy:
		if _, ok := d.step(r.step); !ok {
			return errors.New("planedit: step out of range")
		}
		d.Steps[r.step].Why = value
	case fieldDoneWhen:
		if _, ok := d.step(r.step); !ok {
			return errors.New("planedit: step out of range")
		}
		d.Steps[r.step].DoneWhen = value
	case fieldNote:
		if _, ok := d.step(r.step); !ok {
			return errors.New("planedit: step out of range")
		}
		d.Steps[r.step].Note = value
	case fieldRisk:
		if _, ok := d.step(r.step); !ok {
			return errors.New("planedit: step out of range")
		}
		d.Steps[r.step].Risk = value
	}
	return nil
}

// Draft is the editable copy of the plan contract.
type Draft struct {
	Goal            string
	Approach        string
	WorkingContext  string
	SuccessCriteria []string
	Constraints     []string
	Steps           []DraftStep
}

// DraftStep holds the patchable fields of one plan step. Steps without ids
// (legacy v1 items) cannot be patched and never become draft steps.
type DraftStep struct {
	ID        string
	Content   string
	Why       string
	DoneWhen  string
	Note      string
	Risk      string
	baseIndex int
}

func (d Draft) step(i int) (DraftStep, bool) {
	if i < 0 || i >= len(d.Steps) {
		return DraftStep{}, false
	}
	return d.Steps[i], true
}

// newDraft copies the patchable surface of the snapshot. Items without ids
// stay rendered from the base but are not editable.
func newDraft(plan session.Plan) Draft {
	d := Draft{
		Goal:            plan.Goal,
		Approach:        plan.Approach,
		WorkingContext:  plan.WorkingContext,
		SuccessCriteria: slices.Clone(plan.SuccessCriteria),
		Constraints:     slices.Clone(plan.Constraints),
	}
	for i, item := range plan.Items {
		if item.ID == "" {
			continue
		}
		d.Steps = append(d.Steps, DraftStep{
			ID:        item.ID,
			Content:   item.Content,
			Why:       item.Why,
			DoneWhen:  item.DoneWhen,
			Note:      item.Note,
			Risk:      item.Risk,
			baseIndex: i,
		})
	}
	return d
}

// patchValue wraps a scalar for a patch operation slot.
func patchValue(v string) session.PatchValue[string] {
	return session.PatchValue[string]{Set: true, Value: v}
}

// ops diffs the draft against its base snapshot and returns the atomic patch
// batch that turns base into draft; nil means nothing changed. Directives are
// identity-keyed by exact text, so a reword is a remove plus an add.
func (d Draft) ops(base session.Plan) ([]session.PlanPatchOp, error) {
	var ops []session.PlanPatchOp

	if d.Goal != base.Goal || d.Approach != base.Approach || d.WorkingContext != base.WorkingContext {
		if strings.TrimSpace(d.Goal) == "" {
			return nil, errors.New("planedit: goal cannot be empty")
		}
		op := session.PlanPatchOp{Op: session.PlanPatchSetPlanFields}
		if d.Goal != base.Goal {
			op.Goal = patchValue(d.Goal)
		}
		if d.Approach != base.Approach {
			op.Approach = patchValue(d.Approach)
		}
		if d.WorkingContext != base.WorkingContext {
			op.WorkingContext = patchValue(d.WorkingContext)
		}
		ops = append(ops, op)
	}

	for _, old := range base.Constraints {
		if !slices.Contains(d.Constraints, old) {
			ops = append(ops, session.PlanPatchOp{Op: session.PlanPatchRemoveConstraint, Value: old})
		}
	}
	for _, added := range d.Constraints {
		if !slices.Contains(base.Constraints, added) {
			if strings.TrimSpace(added) == "" {
				return nil, errors.New("planedit: constraint cannot be empty")
			}
			ops = append(ops, session.PlanPatchOp{Op: session.PlanPatchAddConstraint, Value: added})
		}
	}

	for _, old := range base.SuccessCriteria {
		if !slices.Contains(d.SuccessCriteria, old) {
			ops = append(ops, session.PlanPatchOp{Op: session.PlanPatchRemoveCriterion, Value: old})
		}
	}
	for _, added := range d.SuccessCriteria {
		if !slices.Contains(base.SuccessCriteria, added) {
			if strings.TrimSpace(added) == "" {
				return nil, errors.New("planedit: criterion cannot be empty")
			}
			ops = append(ops, session.PlanPatchOp{Op: session.PlanPatchAddCriterion, Value: added})
		}
	}

	for _, step := range d.Steps {
		if step.baseIndex < 0 || step.baseIndex >= len(base.Items) {
			continue
		}
		item := base.Items[step.baseIndex]
		op := session.PlanPatchOp{Op: session.PlanPatchUpdateStep, ID: step.ID}
		changed := false
		if step.Content != item.Content {
			if strings.TrimSpace(step.Content) == "" {
				return nil, fmt.Errorf("planedit: step %s content cannot be empty", step.ID)
			}
			op.Content = patchValue(step.Content)
			changed = true
		}
		if step.Why != item.Why {
			op.Why = patchValue(step.Why)
			changed = true
		}
		if step.DoneWhen != item.DoneWhen {
			op.DoneWhen = patchValue(step.DoneWhen)
			changed = true
		}
		if step.Note != item.Note {
			op.Note = patchValue(step.Note)
			changed = true
		}
		if step.Risk != item.Risk {
			op.Risk = patchValue(step.Risk)
			changed = true
		}
		if changed {
			ops = append(ops, op)
		}
	}

	return ops, nil
}

type rowKind uint8

const (
	rowHeading rowKind = iota
	rowField
	rowStep
	rowStepField
	rowInfo
)

type paneRow struct {
	text string
	kind rowKind
	ref  fieldRef
}

// Pane is a full-screen modal with a centered plan panel. It is mutated and
// rendered only by the UI goroutine.
type Pane struct {
	theme components.Theme
	store Store

	onClose   func()
	onApplied func(session.Plan)
	visible   bool

	base     session.Plan // snapshot the draft came from; carries the revision
	draft    Draft
	dirty    bool
	err      string
	readonly bool

	editActive bool
	editRef    fieldRef
	editDraft  string

	selected int
	scroll   int
	viewport int
	overflow bool

	bodyTop   int
	bodyLeft  int
	bodyWidth int
}

// New returns a hidden modal.
func New(theme components.Theme, store Store, onClose func()) *Pane {
	return &Pane{theme: theme, store: store, onClose: onClose}
}

// SetTheme updates modal chrome styling.
func (p *Pane) SetTheme(theme components.Theme) {
	if p != nil {
		p.theme = theme
	}
}

// SetOnApplied receives the committed plan before the modal closes.
func (p *Pane) SetOnApplied(onApplied func(session.Plan)) {
	if p != nil {
		p.onApplied = onApplied
	}
}

// Show opens a fresh draft of the latest durable plan. Legacy v1 plans render
// read-only: the patch vocabulary requires a v2 contract.
func (p *Pane) Show() {
	if p == nil {
		return
	}
	p.visible = true
	p.selected, p.scroll = 0, 0
	p.dirty = false
	p.err = ""
	p.cancelEdit()
	if p.store != nil {
		p.base = p.store.Snapshot()
	} else {
		p.base = session.Plan{}
	}
	p.draft = newDraft(p.base)
	p.readonly = !p.base.Schema.IsV2()
}

// Hide discards the in-memory draft and closes the modal.
func (p *Pane) Hide() {
	if p == nil || !p.visible {
		return
	}
	p.visible = false
	p.draft = Draft{}
	p.err = ""
	p.cancelEdit()
	if p.onClose != nil {
		p.onClose()
	}
}

// Visible reports whether the modal owns the screen and input.
func (p *Pane) Visible() bool { return p != nil && p.visible }

// State returns interaction state for tests and shell integration.
func (p *Pane) State() State {
	if p == nil {
		return State{}
	}
	return State{
		Selected: p.selected,
		Scroll:   p.scroll,
		Overflow: p.overflow,
		Dirty:    p.dirty,
		Error:    p.err,
		Editing:  p.editActive,
		Readonly: p.readonly,
	}
}

// Handle implements components.Widget; the editor dispatches through
// HandleEvent so hidden panes cannot accidentally consume input.
func (*Pane) Handle(*components.EventContext, xui.Event) {}

// HandleEvent consumes every key and mouse event while the modal is visible.
func (p *Pane) HandleEvent(ctx *components.EventContext, ev xui.Event) bool {
	if p == nil || !p.visible {
		return false
	}
	switch event := ev.(type) {
	case xui.KeyEvent:
		if event.Press {
			p.handleKey(event)
		}
	case xui.MouseEvent:
		p.handleMouse(event)
	default:
		return false
	}
	ctx.ConsumeAndRedraw()
	return true
}

func (p *Pane) handleKey(event xui.KeyEvent) {
	if event.Code == xui.KeyRune && event.Mods == xui.ModCtrl && event.HotkeyRune() == 's' {
		if p.editActive {
			if !p.commitEdit() {
				return
			}
		}
		p.apply()
		return
	}
	if p.editActive {
		p.handleEditKey(event)
		return
	}
	switch event.Code {
	case xui.KeyEscape:
		p.Hide()
	case xui.KeyUp:
		p.moveSelection(-1)
	case xui.KeyDown:
		p.moveSelection(1)
	case xui.KeyPageUp:
		p.moveSelection(-max(p.viewport-1, 1))
	case xui.KeyPageDown:
		p.moveSelection(max(p.viewport-1, 1))
	case xui.KeyHome:
		p.selected = 0
		p.followSelection()
	case xui.KeyEnd:
		p.selected = max(len(p.rows())-1, 0)
		p.followSelection()
	case xui.KeyEnter:
		p.activateSelected()
	case xui.KeyRune:
		if event.Rune == ' ' && event.Mods == 0 {
			p.activateSelected()
		}
	}
}

func (p *Pane) handleEditKey(event xui.KeyEvent) {
	switch event.Code {
	case xui.KeyEscape:
		p.cancelEdit()
		p.err = ""
	case xui.KeyEnter:
		p.commitEdit()
	case xui.KeyBackspace:
		runes := []rune(p.editDraft)
		if len(runes) > 0 {
			p.editDraft = string(runes[:len(runes)-1])
		}
	case xui.KeyRune:
		if !event.Mods.Has(xui.ModCtrl) && !event.Mods.Has(xui.ModAlt) && len([]rune(p.editDraft)) < maxEditRunes {
			p.editDraft += string(event.Rune)
		}
	}
}

func (p *Pane) handleMouse(event xui.MouseEvent) {
	if p.editActive {
		return
	}
	if event.Action == xui.MousePress && event.Button == xui.MouseLeft {
		if event.Y >= p.bodyTop && event.Y < p.bodyTop+p.viewport &&
			event.X >= p.bodyLeft && event.X < p.bodyLeft+p.bodyWidth {
			idx := p.scroll + event.Y - p.bodyTop
			rows := p.rows()
			if idx < len(rows) {
				p.selected = idx
				p.activate(rows[idx])
			}
		}
		return
	}
	step := max(event.Wheel, 1) * 3
	switch event.Button {
	case xui.MouseWheelUp:
		p.scroll -= step
	case xui.MouseWheelDown:
		p.scroll += step
	}
	p.clampScroll()
}

func (p *Pane) moveSelection(delta int) {
	p.selected += delta
	p.clampSelection()
	p.followSelection()
}

func (p *Pane) clampSelection() {
	p.selected = clamp(p.selected, 0, max(len(p.rows())-1, 0))
}

func (p *Pane) followSelection() {
	if p.viewport <= 0 {
		p.scroll = 0
		return
	}
	p.scroll = min(p.scroll, p.selected)
	if p.selected >= p.scroll+p.viewport {
		p.scroll = p.selected - p.viewport + 1
	}
	p.clampScroll()
}

func (p *Pane) clampScroll() {
	if p.viewport <= 0 {
		p.scroll = 0
		return
	}
	p.scroll = clamp(p.scroll, 0, max(len(p.rows())-p.viewport, 0))
}

func (p *Pane) activateSelected() {
	rows := p.rows()
	if p.selected >= 0 && p.selected < len(rows) {
		p.activate(rows[p.selected])
	}
}

// activate starts inline editing on an editable row; read-only rows explain
// why they refuse instead of silently doing nothing.
func (p *Pane) activate(row paneRow) {
	switch row.kind {
	case rowField, rowStepField:
		if p.readonly {
			p.err = "legacy plan: only v2 plans can be edited"
			return
		}
		p.editActive = true
		p.editRef = row.ref
		p.editDraft = row.ref.get(p.draft)
		p.err = ""
	case rowStep:
		p.err = "step status moves only through plan lifecycle actions"
	case rowHeading, rowInfo:
		p.err = "read-only row"
	}
}

// commitEdit stores the edited text. It reports whether the edit landed.
func (p *Pane) commitEdit() bool {
	if !p.editActive {
		return false
	}
	if err := p.editRef.set(&p.draft, p.editDraft); err != nil {
		p.err = err.Error()
		return false
	}
	p.cancelEdit()
	p.dirty = true
	p.err = ""
	return true
}

func (p *Pane) cancelEdit() {
	p.editActive = false
	p.editRef = noField
	p.editDraft = ""
}

func (p *Pane) apply() {
	if p.store == nil {
		p.err = "planedit: plan store unavailable"
		return
	}
	if p.readonly {
		p.err = "legacy plan: only v2 plans can be edited"
		return
	}
	ops, err := p.draft.ops(p.base)
	if err != nil {
		p.err = err.Error()
		return
	}
	if len(ops) == 0 {
		p.Hide()
		return
	}
	plan, err := p.store.Apply(context.Background(), p.base.Revision, ops)
	if err != nil {
		// A stale revision names the actual one; the next Show() re-reads it.
		p.err = err.Error()
		return
	}
	if p.onApplied != nil {
		p.onApplied(plan)
	}
	p.Hide()
}

// Draw renders an opaque screen with a centered modal panel.
func (p *Pane) Draw(ctx components.DrawContext) components.Surface {
	w, h := max(ctx.Max.Width, 1), max(ctx.Max.Height, 1)
	th := p.theme
	if th.Foreground.Fg.Kind == 0 && th.Muted.Fg.Kind == 0 {
		th = components.DefaultTheme()
	}
	root := components.NewSurface(w, h, p)
	fillSurface(&root, xui.Style{Fg: th.Foreground.Fg})

	pw := min(min(max(w-4, 20), 100), w)
	ph := min(min(max(h-2, 6), 36), h)
	x0, y0 := (w-pw)/2, (h-ph)/2
	panel := components.NewSurface(pw, ph, p)
	fillSurface(&panel, xui.Style{Fg: th.Foreground.Fg, Bg: th.BackgroundElement.Bg})

	title := " Plan "
	if p.readonly {
		title = " Plan · read-only "
	}
	layout.DrawRoundedBorder(
		&panel,
		layout.BorderRounded,
		xui.Style{Fg: th.Muted.Fg, Bg: th.BackgroundElement.Bg},
		&layout.BorderLabel{Text: title, Style: th.Foreground},
		nil,
		nil,
		&layout.BorderLabel{Text: " Ctrl+S apply · Esc discard · Enter edit ", Style: th.Muted},
		ctx.Method,
	)

	meta := fmt.Sprintf("rev %d · %s", p.base.Revision, planState(p.base.Approved))
	if p.dirty {
		meta += " · unsaved"
	}
	panel.Print(2, 1, layout.TruncateToWidth(meta, pw-4, ctx.Method), th.Muted, ctx.Method)

	bodyTop := 3
	view := max(ph-5, 0)
	p.viewport = view
	rows := p.rows()
	p.overflow = len(rows) > view
	p.clampSelection()
	p.clampScroll()
	p.bodyTop, p.bodyLeft, p.bodyWidth = y0+bodyTop, x0+min(2, pw), max(pw-4, 0)

	for i := range view {
		idx := p.scroll + i
		if idx >= len(rows) || bodyTop+i >= ph-1 {
			break
		}
		style, marker := p.rowStyle(rows[idx], idx)
		line := marker + rows[idx].text
		panel.Print(2, bodyTop+i, layout.TruncateToWidth(line, pw-5, ctx.Method), style, ctx.Method)
	}
	if p.overflow {
		drawScrollbar(&panel, pw-2, bodyTop, view, len(rows), p.scroll, th.Muted)
	}
	if p.err != "" && ph >= 3 {
		panel.Print(2, ph-2, layout.TruncateToWidth("Error: "+p.err, pw-4, ctx.Method), th.Warning, ctx.Method)
	}
	blit(&root, panel, x0, y0)
	return root
}

func planState(approved bool) string {
	if approved {
		return "approved"
	}
	return "draft"
}

func (p *Pane) rowStyle(row paneRow, idx int) (xui.Style, string) {
	th := p.theme
	marker := "  "
	if idx == p.selected {
		return xui.Style{Reverse: true}, "› "
	}
	switch row.kind {
	case rowHeading:
		return xui.Style{Bold: true, Fg: th.Foreground.Fg}, marker
	case rowStep:
		return th.Foreground, marker
	case rowInfo:
		return th.Muted, marker
	default:
		return th.Foreground, marker
	}
}

// rows flattens the two zones — the plan header and the step list — into one
// navigable list with a uniform "label: value" markup.
func (p *Pane) rows() []paneRow {
	rows := []paneRow{
		{text: "Plan", kind: rowHeading},
		{text: "  goal: " + valueOrNone(p.draft.Goal), kind: rowField, ref: fieldRef{kind: fieldGoal, step: -1}},
		{
			text: "  approach: " + valueOrNone(p.draft.Approach),
			kind: rowField,
			ref:  fieldRef{kind: fieldApproach, step: -1},
		},
		{
			text: "  context: " + valueOrNone(p.draft.WorkingContext),
			kind: rowField,
			ref:  fieldRef{kind: fieldContext, step: -1},
		},
	}
	for i, criterion := range p.draft.SuccessCriteria {
		rows = append(rows, paneRow{
			text: "  criterion " + strconv.Itoa(i+1) + ": " + valueOrNone(criterion),
			kind: rowField, ref: fieldRef{kind: fieldCriterion, step: -1, idx: i},
		})
	}
	for i, constraint := range p.draft.Constraints {
		rows = append(rows, paneRow{
			text: "  constraint " + strconv.Itoa(i+1) + ": " + valueOrNone(constraint),
			kind: rowField, ref: fieldRef{kind: fieldConstraint, step: -1, idx: i},
		})
	}

	rows = append(rows, paneRow{text: "Steps", kind: rowHeading})
	stepDraft := 0
	for i, item := range p.base.Items {
		rows = append(rows, paneRow{
			text: fmt.Sprintf("%d %s %s — %s", i+1, statusIcon(item.Status), stepTypeLabel(item.Type), item.Content),
			kind: rowStep,
		})
		if item.ID != "" && !p.readonly && stepDraft < len(p.draft.Steps) && p.draft.Steps[stepDraft].baseIndex == i {
			s := p.draft.Steps[stepDraft]
			base := "    "
			rows = append(
				rows,
				paneRow{
					text: base + "content: " + valueOrNone(s.Content),
					kind: rowStepField,
					ref:  fieldRef{kind: fieldContent, step: stepDraft},
				},
				paneRow{
					text: base + "why: " + valueOrNone(s.Why),
					kind: rowStepField,
					ref:  fieldRef{kind: fieldWhy, step: stepDraft},
				},
				paneRow{
					text: base + "done when: " + valueOrNone(s.DoneWhen),
					kind: rowStepField,
					ref:  fieldRef{kind: fieldDoneWhen, step: stepDraft},
				},
				paneRow{
					text: base + "note: " + valueOrNone(s.Note),
					kind: rowStepField,
					ref:  fieldRef{kind: fieldNote, step: stepDraft},
				},
				paneRow{
					text: base + "risk: " + valueOrNone(s.Risk),
					kind: rowStepField,
					ref:  fieldRef{kind: fieldRisk, step: stepDraft},
				},
			)
			stepDraft++
		} else if item.ID == "" {
			rows = append(rows, paneRow{text: "    (legacy step without id — read-only)", kind: rowInfo})
		}
		if item.Outcome != "" {
			rows = append(rows, paneRow{text: "    outcome: " + item.Outcome, kind: rowInfo})
		}
		if item.Evidence != "" {
			rows = append(rows, paneRow{text: "    evidence: " + item.Evidence, kind: rowInfo})
		}
		if item.Blocker != "" {
			rows = append(rows, paneRow{text: "    blocked by: " + item.Blocker, kind: rowInfo})
		}
	}
	if len(p.base.Items) == 0 {
		rows = append(rows, paneRow{text: "  (no steps)", kind: rowInfo})
	}
	return rows
}

// valueOrNone keeps every field row the same shape; an empty optional slot
// reads as (none) instead of a bare colon.
func valueOrNone(v string) string {
	if strings.TrimSpace(v) == "" {
		return "(none)"
	}
	return v
}

func statusIcon(status session.PlanStatus) string {
	switch status {
	case session.PlanInProgress:
		return "▸"
	case session.PlanCompleted:
		return "✓"
	case session.PlanBlocked:
		return "!"
	case session.PlanCancelled:
		return "×"
	default:
		return "○"
	}
}

func stepTypeLabel(t session.StepType) string {
	if t == "" {
		return "step"
	}
	return string(t)
}

const maxEditRunes = 2048

func fillSurface(surface *components.Surface, style xui.Style) {
	for y := 0; y < surface.Size.Height; y++ {
		for x := 0; x < surface.Size.Width; x++ {
			surface.SetCell(x, y, xui.Cell{Char: " ", Width: 1, Style: style})
		}
	}
}

func blit(dst *components.Surface, src components.Surface, x0, y0 int) {
	for y := 0; y < src.Size.Height; y++ {
		for x := 0; x < src.Size.Width; x++ {
			dst.SetCell(x0+x, y0+y, src.Buffer[y*src.Size.Width+x])
		}
	}
}

func drawScrollbar(surface *components.Surface, x, y, height, total, scroll int, style xui.Style) {
	if height <= 0 || total <= height {
		return
	}
	thumb := max(height*height/total, 1)
	start := scroll * max(height-thumb, 0) / max(total-height, 1)
	for row := range height {
		char := "│"
		if row >= start && row < start+thumb {
			char = "█"
		}
		surface.SetCell(x, y+row, xui.Cell{Char: char, Width: 1, Style: style})
	}
}

func clamp(value, low, high int) int { return min(max(value, low), high) }
