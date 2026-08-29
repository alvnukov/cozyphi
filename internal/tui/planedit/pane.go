// Package planedit renders the durable plan viewer/editor modal. Pane owns
// only an editable draft and interaction state; persistence stays behind Store.
package planedit

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/input"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/session"
)

// Store is the plan editor's complete external interface. It supplies one
// durable snapshot, the configured type choices, and an atomic patch apply.
type Store interface {
	Snapshot() session.Plan
	StepTypes() []session.StepType
	// Models lists what the model pickers offer: configured models merged
	// with the provider catalog.
	Models() []string
	Apply(ctx context.Context, expectedRevision uint64, ops []session.PlanPatchOp) (session.Plan, error)
}

// State is a detached behavioral snapshot for shell integration and tests.
type State struct {
	Selected   int
	Scroll     int
	Overflow   bool
	Dirty      bool
	Error      string
	Editing    bool
	Detail     bool
	Confirming bool
	Readonly   bool
}

type fieldKind uint8

const (
	fieldGoal fieldKind = iota
	fieldApproach
	fieldContext
	fieldCriterion
	fieldConstraint
	fieldID
	fieldContent
	fieldWhy
	fieldDoneWhen
	fieldNote
	fieldRisk
	fieldSkills
)

const (
	maxGoalRunes      = 512
	maxApproachRunes  = 1024
	maxContextRunes   = 2048
	maxDirectiveRunes = 512
	maxDirectiveCount = 8
	maxStepIDRunes    = 64
	maxStepFieldRunes = 512
	maxPatchOps       = 32

	// Session caps mirrored locally, so the editor refuses before the patch
	// path has to.
	maxStepActions  = 4
	maxActionSkills = 4
)

var stepIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type fieldRef struct {
	kind fieldKind
	step int
	idx  int
}

func (r fieldRef) label() string {
	switch r.kind {
	case fieldGoal:
		return "goal"
	case fieldApproach:
		return "approach"
	case fieldContext:
		return "working context"
	case fieldCriterion:
		return "success criterion"
	case fieldConstraint:
		return "constraint"
	case fieldID:
		return "step id"
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
	case fieldSkills:
		return "action skills"
	default:
		return "field"
	}
}

func (r fieldRef) limit() int {
	switch r.kind {
	case fieldGoal:
		return maxGoalRunes
	case fieldApproach:
		return maxApproachRunes
	case fieldContext:
		return maxContextRunes
	case fieldID:
		return maxStepIDRunes
	default:
		return maxStepFieldRunes
	}
}

func (r fieldRef) required() bool {
	switch r.kind {
	case fieldGoal, fieldApproach, fieldCriterion, fieldConstraint, fieldID, fieldContent, fieldWhy, fieldDoneWhen:
		return true
	default:
		return false
	}
}

type directiveDraft struct {
	Value    string
	Original string
	New      bool
}

// Draft is the complete editable contract. Original identity is retained in
// private fields so compilation can use update operations instead of risky
// remove-then-add rewrites.
type Draft struct {
	Goal            string
	Approach        string
	WorkingContext  string
	SuccessCriteria []directiveDraft
	Constraints     []directiveDraft
	ModelsByType    map[session.StepType]string
	Steps           []DraftStep
}

// DraftStep carries both editable prose and immutable lifecycle identity.
type DraftStep struct {
	ID       string
	Content  string
	Type     session.StepType
	Status   session.PlanStatus
	Why      string
	DoneWhen string
	Note     string
	Risk     string
	JIT      bool
	Model    string
	Actions  []session.PlanAction

	baseIndex int
	baseID    string
	isNew     bool
}

func newDraft(plan session.Plan) Draft {
	d := Draft{
		Goal: plan.Goal, Approach: plan.Approach, WorkingContext: plan.WorkingContext,
		ModelsByType: maps.Clone(plan.ModelsByType),
	}
	for _, value := range plan.SuccessCriteria {
		d.SuccessCriteria = append(d.SuccessCriteria, directiveDraft{Value: value, Original: value})
	}
	for _, value := range plan.Constraints {
		d.Constraints = append(d.Constraints, directiveDraft{Value: value, Original: value})
	}
	for i, item := range plan.Items {
		d.Steps = append(d.Steps, DraftStep{
			ID: item.ID, Content: item.Content, Type: item.Type, Status: item.Status,
			Why: item.Why, DoneWhen: item.DoneWhen, Note: item.Note, Risk: item.Risk, JIT: item.JIT,
			Model: item.Model, Actions: append([]session.PlanAction(nil), item.Actions...),
			baseIndex: i, baseID: item.ID,
		})
	}
	return d
}

func patchValue(value string) session.PatchValue[string] {
	return session.PatchValue[string]{Set: true, Value: value}
}

// authoredActions strips run history and compact-irrelevant skills: the
// durable patch path rejects authored lists that carry runs, so the editor
// never re-authors history it only displays.
func authoredActions(actions []session.PlanAction) []session.PlanAction {
	out := make([]session.PlanAction, 0, len(actions))
	for _, action := range actions {
		clean := session.PlanAction{Event: action.Event, Type: action.Type}
		if action.Type == session.PlanActionInjectSkill {
			clean.Skills = action.Skills
		}
		out = append(out, clean)
	}
	return out
}

// actionsEqual compares authored action lists field by field; the slice
// inside PlanAction keeps them out of slices.Equal.
func actionsEqual(a, b []session.PlanAction) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Event != b[i].Event || a[i].Type != b[i].Type || !slices.Equal(a[i].Skills, b[i].Skills) {
			return false
		}
	}
	return true
}

// authoredModelsByType drops cleared pins so a type without a model holds no
// key at all — exactly what the durable map should store.
func authoredModelsByType(models map[session.StepType]string) map[session.StepType]string {
	var out map[session.StepType]string
	for typ, name := range models {
		if name == "" {
			continue
		}
		if out == nil {
			out = make(map[session.StepType]string)
		}
		out[typ] = name
	}
	return out
}

func (d Draft) validate(types []session.StepType) error {
	if err := validateText("goal", d.Goal, maxGoalRunes, true); err != nil {
		return err
	}
	if err := validateText("approach", d.Approach, maxApproachRunes, true); err != nil {
		return err
	}
	if err := validateText("working context", d.WorkingContext, maxContextRunes, false); err != nil {
		return err
	}
	if len(d.SuccessCriteria) == 0 {
		return errors.New("planedit: at least one success criterion is required")
	}
	if len(d.SuccessCriteria) > maxDirectiveCount || len(d.Constraints) > maxDirectiveCount {
		return fmt.Errorf("planedit: criteria and constraints allow at most %d items each", maxDirectiveCount)
	}
	if err := validateDirectives(d.SuccessCriteria, "success criterion"); err != nil {
		return err
	}
	if err := validateDirectives(d.Constraints, "constraint"); err != nil {
		return err
	}
	configured := make(map[session.StepType]struct{}, len(types))
	for _, typ := range types {
		configured[typ] = struct{}{}
	}
	ids := make(map[string]struct{}, len(d.Steps))
	for i, step := range d.Steps {
		if step.ID == "" && !step.isNew { // Loaded legacy step; structural operations are separately blocked.
			continue
		}
		if err := validateText(fmt.Sprintf("step %d id", i+1), step.ID, maxStepIDRunes, true); err != nil {
			return err
		}
		if !stepIDPattern.MatchString(step.ID) {
			return fmt.Errorf(
				"planedit: step %d id must be a lowercase slug using letters, digits, '.', '_' or '-'",
				i+1,
			)
		}
		if _, duplicate := ids[step.ID]; duplicate {
			return fmt.Errorf("planedit: step id %q is duplicated", step.ID)
		}
		ids[step.ID] = struct{}{}
		if err := validateText(
			fmt.Sprintf("step %q content", step.ID),
			step.Content,
			maxStepFieldRunes,
			true,
		); err != nil {
			return err
		}
		if err := validateText(fmt.Sprintf("step %q why", step.ID), step.Why, maxStepFieldRunes, true); err != nil {
			return err
		}
		if err := validateText(
			fmt.Sprintf("step %q done when", step.ID),
			step.DoneWhen,
			maxStepFieldRunes,
			true,
		); err != nil {
			return err
		}
		if err := validateText(fmt.Sprintf("step %q note", step.ID), step.Note, maxStepFieldRunes, false); err != nil {
			return err
		}
		if err := validateText(fmt.Sprintf("step %q risk", step.ID), step.Risk, maxStepFieldRunes, false); err != nil {
			return err
		}
		if step.isNew {
			if _, ok := configured[step.Type]; !ok {
				return fmt.Errorf("planedit: step %q type %q is not configured", step.ID, step.Type)
			}
		}
	}
	return nil
}

func validateText(label, value string, limit int, required bool) error {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return fmt.Errorf("planedit: %s is required", label)
	}
	if utf8.RuneCountInString(value) > limit {
		return fmt.Errorf("planedit: %s exceeds %d characters", label, limit)
	}
	return nil
}

func validateDirectives(values []directiveDraft, label string) error {
	seen := make(map[string]struct{}, len(values))
	for i, entry := range values {
		value := strings.TrimSpace(entry.Value)
		if err := validateText(fmt.Sprintf("%s %d", label, i+1), value, maxDirectiveRunes, true); err != nil {
			return err
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("planedit: %s %q is duplicated", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

// ops is the deep draft-to-patch compiler. It validates the complete draft,
// preserves operation identity, and emits one atomic bounded reconciliation.
func (d Draft) ops(base session.Plan, types []session.StepType) ([]session.PlanPatchOp, error) {
	if err := d.validate(types); err != nil {
		return nil, err
	}
	var ops []session.PlanPatchOp
	if modelsByType := authoredModelsByType(d.ModelsByType); !maps.Equal(modelsByType, base.ModelsByType) {
		ops = append(ops, session.PlanPatchOp{
			Op:           session.PlanPatchSetPlanFields,
			ModelsByType: session.PatchValue[map[session.StepType]string]{Set: true, Value: modelsByType},
		})
	}
	if d.Goal != base.Goal || d.Approach != base.Approach {
		op := session.PlanPatchOp{Op: session.PlanPatchSetPlanFields}
		if d.Goal != base.Goal {
			op.Goal = patchValue(strings.TrimSpace(d.Goal))
		}
		if d.Approach != base.Approach {
			op.Approach = patchValue(strings.TrimSpace(d.Approach))
		}
		ops = append(ops, op)
	}
	if d.WorkingContext != base.WorkingContext {
		ops = append(ops, session.PlanPatchOp{
			Op: session.PlanPatchReplaceContext, WorkingContext: patchValue(strings.TrimSpace(d.WorkingContext)),
		})
	}
	ops = append(ops, directiveOps(d.SuccessCriteria, base.SuccessCriteria, true)...)
	ops = append(ops, directiveOps(d.Constraints, base.Constraints, false)...)

	basePresent := make(map[string]bool, len(base.Items))
	for _, step := range d.Steps {
		if !step.isNew && step.baseID != "" {
			basePresent[step.baseID] = true
		}
	}
	for _, step := range d.Steps {
		if step.isNew || step.baseID == "" || step.baseIndex < 0 || step.baseIndex >= len(base.Items) {
			continue
		}
		item := base.Items[step.baseIndex]
		op := session.PlanPatchOp{Op: session.PlanPatchUpdateStep, ID: step.baseID}
		if step.Content != item.Content {
			op.Content = patchValue(strings.TrimSpace(step.Content))
		}
		if step.Why != item.Why {
			op.Why = patchValue(strings.TrimSpace(step.Why))
		}
		if step.DoneWhen != item.DoneWhen {
			op.DoneWhen = patchValue(strings.TrimSpace(step.DoneWhen))
		}
		if step.Note != item.Note {
			op.Note = patchValue(strings.TrimSpace(step.Note))
		}
		if step.Risk != item.Risk {
			op.Risk = patchValue(strings.TrimSpace(step.Risk))
		}
		if step.Model != item.Model {
			op.Model = patchValue(step.Model)
		}
		if !actionsEqual(authoredActions(step.Actions), authoredActions(item.Actions)) {
			op.Actions = session.PatchValue[[]session.PlanAction]{Set: true, Value: authoredActions(step.Actions)}
		}
		if op.Content.Set || op.Why.Set || op.DoneWhen.Set || op.Note.Set || op.Risk.Set || op.Model.Set ||
			op.Actions.Set {
			ops = append(ops, op)
		}
	}

	var deletions []session.PlanItem
	for _, item := range base.Items {
		if item.ID == "" || basePresent[item.ID] {
			continue
		}
		if item.Status != session.PlanPending {
			return nil, fmt.Errorf(
				"planedit: step %q is %s; only pending steps can be removed",
				item.ID,
				item.Status,
			)
		}
		deletions = append(deletions, item)
	}
	newSteps := make([]DraftStep, 0, len(d.Steps))
	for _, step := range d.Steps {
		if step.isNew {
			newSteps = append(newSteps, step)
		}
	}

	anchor, heldAnchor := "", ""
	if len(newSteps) > 0 {
		for _, item := range base.Items {
			if item.ID != "" && basePresent[item.ID] {
				anchor = item.ID
				break
			}
		}
		if anchor == "" && len(deletions) > 0 {
			anchor, heldAnchor = deletions[0].ID, deletions[0].ID
		}
		if anchor == "" {
			return nil, errors.New(
				"planedit: cannot add steps to an empty plan because insert_step requires an existing step anchor; create the plan with at least one step first",
			)
		}
	}

	// Remove obsolete pending steps first to free the 32-step budget. If every
	// old step is being replaced, one old step stays only long enough to anchor
	// all insertions; it is removed immediately afterward.
	for _, item := range deletions {
		if item.ID != heldAnchor {
			ops = append(ops, session.PlanPatchOp{Op: session.PlanPatchRemoveStep, ID: item.ID})
		}
	}
	for _, step := range newSteps {
		item := &session.PlanItem{
			ID:       step.ID,
			Content:  strings.TrimSpace(step.Content),
			Type:     step.Type,
			Status:   session.PlanPending,
			Why:      strings.TrimSpace(step.Why),
			DoneWhen: strings.TrimSpace(step.DoneWhen),
			Risk:     strings.TrimSpace(step.Risk),
			JIT:      step.JIT,
			Model:    step.Model,
			Actions:  authoredActions(step.Actions),
		}
		ops = append(ops, session.PlanPatchOp{Op: session.PlanPatchInsertStep, Before: anchor, Step: item})
		if strings.TrimSpace(step.Note) != "" {
			ops = append(
				ops,
				session.PlanPatchOp{Op: session.PlanPatchUpdateStep, ID: step.ID, Note: patchValue(step.Note)},
			)
		}
	}
	if heldAnchor != "" {
		ops = append(ops, session.PlanPatchOp{Op: session.PlanPatchRemoveStep, ID: heldAnchor})
	}

	finalIDs := make([]string, 0, len(d.Steps))
	for _, step := range d.Steps {
		if step.ID != "" {
			finalIDs = append(finalIDs, step.ID)
		}
	}
	baseRemaining := make([]string, 0, len(base.Items))
	for _, item := range base.Items {
		if item.ID != "" && basePresent[item.ID] {
			baseRemaining = append(baseRemaining, item.ID)
		}
	}
	structural := len(deletions) > 0 || len(newSteps) > 0
	moved := !slices.Equal(finalIDs, baseRemaining)
	if (structural || moved) && len(finalIDs) > 0 {
		ops = append(ops, session.PlanPatchOp{Op: session.PlanPatchReorderSteps, IDs: finalIDs})
	}
	if len(ops) > maxPatchOps {
		return nil, fmt.Errorf(
			"planedit: draft needs %d patch operations; maximum is %d; apply fewer changes at once",
			len(ops), maxPatchOps,
		)
	}
	return ops, nil
}

func directiveOps(draft []directiveDraft, base []string, criterion bool) []session.PlanPatchOp {
	update, add, remove := session.PlanPatchUpdateConstraint,
		session.PlanPatchAddConstraint,
		session.PlanPatchRemoveConstraint
	if criterion {
		update, add, remove = session.PlanPatchUpdateCriterion,
			session.PlanPatchAddCriterion,
			session.PlanPatchRemoveCriterion
	}

	type transform struct{ from, to string }
	kept := make(map[string]bool, len(draft))
	var transforms []transform
	var additions []string
	for _, entry := range draft {
		value := strings.TrimSpace(entry.Value)
		if entry.New {
			additions = append(additions, value)
			continue
		}
		kept[entry.Original] = true
		if value != entry.Original {
			transforms = append(transforms, transform{from: entry.Original, to: value})
		}
	}
	var removals []string
	for _, original := range base {
		if !kept[original] {
			removals = append(removals, original)
		}
	}

	// Pair removed slots with additions. This keeps a required criterion list
	// non-empty and avoids transient overflow when a list already has 8 entries.
	paired := min(len(removals), len(additions))
	for i := range paired {
		if removals[i] != additions[i] {
			transforms = append(transforms, transform{from: removals[i], to: additions[i]})
		}
	}
	removals, additions = removals[paired:], additions[paired:]

	ops := make([]session.PlanPatchOp, 0, len(removals)+len(additions)+len(transforms)*2)
	current := make(map[string]bool, len(base))
	for _, value := range base {
		current[value] = true
	}
	for _, value := range removals {
		ops = append(ops, session.PlanPatchOp{Op: remove, Value: value})
		delete(current, value)
	}

	targets := make(map[string]bool, len(draft))
	for _, entry := range draft {
		targets[strings.TrimSpace(entry.Value)] = true
	}
	for len(transforms) > 0 {
		progress := false
		for i := 0; i < len(transforms); i++ {
			change := transforms[i]
			if current[change.to] {
				continue
			}
			ops = append(ops, session.PlanPatchOp{Op: update, From: change.from, To: change.to})
			delete(current, change.from)
			current[change.to] = true
			transforms = slices.Delete(transforms, i, i+1)
			progress = true
			break
		}
		if progress {
			continue
		}

		// Every remaining target is occupied by another remaining source: break
		// the cycle with a short value that cannot collide with this bounded list.
		temporary := temporaryDirectiveValue(current, targets)
		change := &transforms[0]
		ops = append(ops, session.PlanPatchOp{Op: update, From: change.from, To: temporary})
		delete(current, change.from)
		current[temporary] = true
		change.from = temporary
	}
	for _, value := range additions {
		ops = append(ops, session.PlanPatchOp{Op: add, Value: value})
	}
	return ops
}

func temporaryDirectiveValue(current, targets map[string]bool) string {
	for _, candidate := range "!#$%&()*+,-./:;<=>?@[]^_{|}~" {
		value := string(candidate)
		if !current[value] && !targets[value] {
			return value
		}
	}
	// Lists are capped at 8, so the single-rune pool above always has a free
	// value. Keep a deterministic fallback in case that bound ever changes.
	for i := 0; ; i++ {
		value := fmt.Sprintf("~%d", i)
		if !current[value] && !targets[value] {
			return value
		}
	}
}

type viewMode uint8

const (
	viewBrowse viewMode = iota
	viewDetail
	viewTypes
	viewModels
)

type rowKind uint8

const (
	rowHeading rowKind = iota
	rowField
	rowStep
	rowAddCriterion
	rowAddConstraint
	rowAddStep
	rowTypeChoice
	rowActionMoveUp
	rowActionMoveDown
	rowActionToggleJIT
	rowActionDelete
	rowModelType
	rowModelChoice
	rowStepModel
	rowAddAction
	rowActionEvent
	rowActionType
	rowActionSkills
	rowActionRemove
	rowActionBack
	rowInfo
)

type paneRow struct {
	text       string
	kind       rowKind
	ref        fieldRef
	step       int
	selectable bool
}

type confirmKind uint8

const (
	confirmNone confirmKind = iota
	confirmDiscard
	confirmCriterion
	confirmConstraint
	confirmStep
)

type confirmation struct {
	kind  confirmKind
	index int
	label string
}

// Pane is a full-screen modal, mutated and rendered only by the UI goroutine.
type Pane struct {
	theme components.Theme
	store Store

	onClose   func()
	onApplied func(session.Plan)
	visible   bool

	base           session.Plan
	draft          Draft
	types          []session.StepType
	models         []string
	modelType      session.StepType // the type a model picker is editing
	modelStep      int              // the step a model picker edits; -1 means a type target
	dirty          bool
	err            string
	readonly       bool
	readonlyReason string
	mode           viewMode
	detailStep     int

	textRef   fieldRef
	textField *input.TextField
	confirm   confirmation

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
	return &Pane{theme: theme, store: store, onClose: onClose, detailStep: -1}
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

// Show opens a fresh draft of the latest durable plan.
func (p *Pane) Show() {
	if p == nil {
		return
	}
	p.visible = true
	p.dirty, p.err = false, ""
	p.mode, p.detailStep = viewBrowse, -1
	p.textField, p.confirm = nil, confirmation{}
	if p.store != nil {
		p.base = p.store.Snapshot()
		p.types = slices.Clone(p.store.StepTypes())
		p.models = append([]string(nil), p.store.Models()...)
	} else {
		p.base = session.Plan{}
		p.types = nil
	}
	p.draft = newDraft(p.base)
	p.readonlyReason = ""
	switch {
	case !p.base.Schema.IsV2():
		p.readonlyReason = "legacy plan: only v2 plans can be edited"
	case p.hasLegacyStep():
		p.readonlyReason = "plan contains legacy id-less steps; migration is required before editing"
	}
	p.readonly = p.readonlyReason != ""
	p.resetSelection()
}

// Hide discards the draft and closes the modal.
func (p *Pane) Hide() {
	if p == nil || !p.visible {
		return
	}
	p.visible = false
	p.draft = Draft{}
	p.textField = nil
	p.confirm = confirmation{}
	p.err = ""
	if p.onClose != nil {
		p.onClose()
	}
}

func (p *Pane) Visible() bool { return p != nil && p.visible }

// State returns interaction state for tests and shell integration.
func (p *Pane) State() State {
	if p == nil {
		return State{}
	}
	return State{
		Selected: p.selected, Scroll: p.scroll, Overflow: p.overflow, Dirty: p.dirty,
		Error: p.err, Editing: p.textField != nil, Detail: p.mode == viewDetail,
		Confirming: p.confirm.kind != confirmNone, Readonly: p.readonly,
	}
}

// Handle implements components.Widget; shell input uses HandleEvent.
func (*Pane) Handle(*components.EventContext, xui.Event) {}

// HandleEvent consumes all input while visible, including paste outside the
// popup, so modal text can never leak into the composer underneath.
func (p *Pane) HandleEvent(ctx *components.EventContext, ev xui.Event) bool {
	if p == nil || !p.visible {
		return false
	}
	if ctx == nil {
		ctx = &components.EventContext{}
	}
	if p.textField != nil {
		p.handleTextEvent(ctx, ev)
		ctx.ConsumeAndRedraw()
		return true
	}
	if p.confirm.kind != confirmNone {
		p.handleConfirmation(ev)
		ctx.ConsumeAndRedraw()
		return true
	}
	switch event := ev.(type) {
	case xui.KeyEvent:
		if event.Press {
			p.handleKey(event)
		}
	case xui.MouseEvent:
		p.handleMouse(event)
	case xui.PasteEvent:
		// Deliberately swallowed while the modal is visible.
	}
	ctx.ConsumeAndRedraw()
	return true
}

func (p *Pane) handleTextEvent(ctx *components.EventContext, ev xui.Event) {
	if key, ok := ev.(xui.KeyEvent); ok && key.Press {
		if key.Code == xui.KeyEscape {
			p.textField = nil
			p.err = ""
			return
		}
		if key.Code == xui.KeyRune && key.Mods == xui.ModCtrl && key.HotkeyRune() == 's' {
			if p.commitText() {
				p.apply()
			}
			return
		}
	}
	before, cursor := p.textField.Value, p.textField.Cursor
	p.textField.Handle(ctx, ev)
	if p.textField != nil && utf8.RuneCountInString(p.textField.Value) > p.textRef.limit() {
		p.textField.Value, p.textField.Cursor = before, cursor
		p.err = fmt.Sprintf("planedit: %s allows at most %d characters", p.textRef.label(), p.textRef.limit())
	}
}

func (p *Pane) handleConfirmation(ev xui.Event) {
	key, ok := ev.(xui.KeyEvent)
	if !ok || !key.Press {
		return
	}
	if key.Code == xui.KeyEscape || (key.Code == xui.KeyRune && strings.EqualFold(string(key.Rune), "n")) {
		p.confirm = confirmation{}
		p.err = ""
		return
	}
	if key.Code != xui.KeyRune || !strings.EqualFold(string(key.Rune), "y") {
		return
	}
	confirm := p.confirm
	p.confirm = confirmation{}
	switch confirm.kind {
	case confirmDiscard:
		p.Hide()
	case confirmCriterion:
		if confirm.index >= 0 && confirm.index < len(p.draft.SuccessCriteria) {
			p.draft.SuccessCriteria = slices.Delete(p.draft.SuccessCriteria, confirm.index, confirm.index+1)
			p.changed()
		}
	case confirmConstraint:
		if confirm.index >= 0 && confirm.index < len(p.draft.Constraints) {
			p.draft.Constraints = slices.Delete(p.draft.Constraints, confirm.index, confirm.index+1)
			p.changed()
		}
	case confirmStep:
		if confirm.index >= 0 && confirm.index < len(p.draft.Steps) {
			p.draft.Steps = slices.Delete(p.draft.Steps, confirm.index, confirm.index+1)
			p.mode, p.detailStep = viewBrowse, -1
			p.changed()
		}
	}
	p.clampSelection()
}

func (p *Pane) handleKey(event xui.KeyEvent) {
	if event.Code == xui.KeyRune && event.Mods == xui.ModCtrl && event.HotkeyRune() == 's' {
		p.apply()
		return
	}
	switch event.Code {
	case xui.KeyEscape:
		if p.mode == viewModels {
			if p.modelStep >= 0 {
				p.mode = viewDetail
			} else {
				p.mode = viewBrowse
			}
			p.resetSelection()
			return
		}
		if p.mode == viewTypes {
			p.mode = viewDetail
			p.resetSelection()
			return
		}
		if p.mode == viewDetail {
			p.mode, p.detailStep = viewBrowse, -1
			p.resetSelection()
			return
		}
		if p.dirty {
			p.confirm = confirmation{kind: confirmDiscard, label: "Discard all unsaved plan changes?"}
		} else {
			p.Hide()
		}
	case xui.KeyUp:
		p.moveSelection(-1)
	case xui.KeyDown:
		p.moveSelection(1)
	case xui.KeyPageUp:
		p.moveSelection(-max(p.viewport-1, 1))
	case xui.KeyPageDown:
		p.moveSelection(max(p.viewport-1, 1))
	case xui.KeyHome:
		p.selectEdge(false)
	case xui.KeyEnd:
		p.selectEdge(true)
	case xui.KeyEnter:
		p.activateSelected()
	case xui.KeyDelete, xui.KeyBackspace:
		p.requestDeleteSelected()
	case xui.KeyRune:
		if event.Rune == ' ' && event.Mods == 0 {
			p.activateSelected()
		}
	}
}

func (p *Pane) handleMouse(event xui.MouseEvent) {
	if event.Action == xui.MousePress && event.Button == xui.MouseLeft {
		if event.Y >= p.bodyTop && event.Y < p.bodyTop+p.viewport &&
			event.X >= p.bodyLeft && event.X < p.bodyLeft+p.bodyWidth {
			idx := p.scroll + event.Y - p.bodyTop
			rows := p.rows()
			if idx >= 0 && idx < len(rows) && rows[idx].selectable {
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

func (p *Pane) resetSelection() {
	p.selected, p.scroll = 0, 0
	p.selectEdge(false)
}

func (p *Pane) moveSelection(delta int) {
	rows := p.rows()
	if len(rows) == 0 || delta == 0 {
		return
	}
	direction := 1
	if delta < 0 {
		direction = -1
	}
	remaining := max(abs(delta), 1)
	idx := p.selected
	for remaining > 0 {
		next := idx + direction
		for next >= 0 && next < len(rows) && !rows[next].selectable {
			next += direction
		}
		if next < 0 || next >= len(rows) {
			break
		}
		idx = next
		remaining--
	}
	p.selected = idx
	p.followSelection()
}

func (p *Pane) selectEdge(end bool) {
	rows := p.rows()
	if end {
		for i := range slices.Backward(rows) {
			if rows[i].selectable {
				p.selected = i
				p.followSelection()
				return
			}
		}
		return
	}
	for i, row := range rows {
		if row.selectable {
			p.selected = i
			p.followSelection()
			return
		}
	}
	p.selected = 0
}

func (p *Pane) clampSelection() {
	rows := p.rows()
	if len(rows) == 0 {
		p.selected = 0
		return
	}
	p.selected = clamp(p.selected, 0, len(rows)-1)
	if !rows[p.selected].selectable {
		p.selectEdge(false)
	}
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

func (p *Pane) activate(row paneRow) {
	if p.readonly && row.kind != rowActionBack {
		p.err = p.readonlyReason
		return
	}
	switch row.kind {
	case rowField:
		if row.ref.kind == fieldID && !p.draft.Steps[row.ref.step].isNew {
			p.err = "step identity is read-only"
			return
		}
		p.openText(row.ref)
	case rowStep:
		p.mode, p.detailStep = viewDetail, row.step
		p.resetSelection()
		p.err = ""
	case rowAddCriterion:
		if len(p.draft.SuccessCriteria) >= maxDirectiveCount {
			p.err = fmt.Sprintf("only %d success criteria are allowed", maxDirectiveCount)
			return
		}
		p.openText(fieldRef{kind: fieldCriterion, idx: len(p.draft.SuccessCriteria), step: -1})
	case rowAddConstraint:
		if len(p.draft.Constraints) >= maxDirectiveCount {
			p.err = fmt.Sprintf("only %d constraints are allowed", maxDirectiveCount)
			return
		}
		p.openText(fieldRef{kind: fieldConstraint, idx: len(p.draft.Constraints), step: -1})
	case rowAddStep:
		p.addStep()
	case rowTypeChoice:
		if p.mode == viewTypes && p.detailStep >= 0 && p.detailStep < len(p.draft.Steps) {
			p.draft.Steps[p.detailStep].Type = p.types[row.step]
			p.mode = viewDetail
			p.changed()
			p.resetSelection()
		} else if p.detailStep >= 0 && p.draft.Steps[p.detailStep].isNew {
			p.mode = viewTypes
			p.resetSelection()
		}
	case rowActionMoveUp:
		p.moveStep(-1)
	case rowActionMoveDown:
		p.moveStep(1)
	case rowActionToggleJIT:
		if row.step >= 0 && row.step < len(p.draft.Steps) && p.draft.Steps[row.step].isNew {
			p.draft.Steps[row.step].JIT = !p.draft.Steps[row.step].JIT
			p.changed()
		}
	case rowActionDelete:
		p.requestDeleteStep(row.step)
	case rowModelType:
		if row.step >= 0 && row.step < len(p.types) {
			p.modelType, p.modelStep = p.types[row.step], -1
			p.mode = viewModels
			p.resetSelection()
		}
	case rowStepModel:
		p.modelStep = p.detailStep
		p.mode = viewModels
		p.resetSelection()
	case rowModelChoice:
		name := ""
		if row.step >= 0 && row.step < len(p.models) {
			name = p.models[row.step]
		}
		if p.modelStep >= 0 && p.modelStep < len(p.draft.Steps) {
			p.draft.Steps[p.modelStep].Model = name
			p.mode = viewDetail
		} else {
			if name == "" {
				delete(p.draft.ModelsByType, p.modelType)
			} else {
				if p.draft.ModelsByType == nil {
					p.draft.ModelsByType = make(map[session.StepType]string)
				}
				p.draft.ModelsByType[p.modelType] = name
			}
			p.mode = viewBrowse
		}
		p.changed()
		p.resetSelection()
	case rowAddAction:
		if p.detailStep < 0 || p.detailStep >= len(p.draft.Steps) {
			return
		}
		step := &p.draft.Steps[p.detailStep]
		if len(step.Actions) >= maxStepActions {
			p.err = fmt.Sprintf("planedit: at most %d actions are allowed per step", maxStepActions)
			return
		}
		step.Actions = append(step.Actions, session.PlanAction{
			Event: session.PlanActionOnStepStart, Type: session.PlanActionCompact,
		})
		p.changed()
	case rowActionEvent, rowActionType, rowActionSkills, rowActionRemove:
		if p.detailStep < 0 || p.detailStep >= len(p.draft.Steps) {
			return
		}
		step := &p.draft.Steps[p.detailStep]
		if row.ref.idx < 0 || row.ref.idx >= len(step.Actions) {
			return
		}
		action := &step.Actions[row.ref.idx]
		switch row.kind {
		case rowActionEvent:
			// Step actions fire on step moments; the cycle stays inside them.
			if action.Event == session.PlanActionOnStepStart {
				action.Event = session.PlanActionOnStepEnd
			} else {
				action.Event = session.PlanActionOnStepStart
			}
		case rowActionType:
			if action.Type == session.PlanActionCompact {
				action.Type = session.PlanActionInjectSkill
			} else {
				action.Type, action.Skills = session.PlanActionCompact, nil
			}
		case rowActionSkills:
			p.openText(fieldRef{kind: fieldSkills, step: p.detailStep, idx: row.ref.idx})
			return
		case rowActionRemove:
			step.Actions = slices.Delete(step.Actions, row.ref.idx, row.ref.idx+1)
		}
		p.changed()
	case rowActionBack:
		p.mode, p.detailStep = viewBrowse, -1
		p.resetSelection()
	}
}

func (p *Pane) addStep() {
	if p.hasLegacyStep() {
		p.err = "legacy id-less steps block adding, deleting, or reordering steps"
		return
	}
	if len(p.types) == 0 {
		p.err = "no step types are configured"
		return
	}
	p.draft.Steps = append(p.draft.Steps, DraftStep{
		Type: p.types[0], Status: session.PlanPending, baseIndex: -1, isNew: true,
	})
	p.detailStep = len(p.draft.Steps) - 1
	p.mode = viewDetail
	p.changed()
	p.resetSelection()
}

func (p *Pane) moveStep(delta int) {
	if p.hasLegacyStep() {
		p.err = "legacy id-less steps block adding, deleting, or reordering steps"
		return
	}
	from, to := p.detailStep, p.detailStep+delta
	if from < 0 || from >= len(p.draft.Steps) || to < 0 || to >= len(p.draft.Steps) {
		p.err = "step is already at that edge"
		return
	}
	p.draft.Steps[from], p.draft.Steps[to] = p.draft.Steps[to], p.draft.Steps[from]
	p.detailStep = to
	p.changed()
}

func (p *Pane) requestDeleteSelected() {
	if p.readonly {
		p.err = p.readonlyReason
		return
	}
	rows := p.rows()
	if p.selected < 0 || p.selected >= len(rows) {
		return
	}
	row := rows[p.selected]
	switch {
	case row.kind == rowField && row.ref.kind == fieldCriterion:
		if len(p.draft.SuccessCriteria) == 1 {
			p.err = "at least one success criterion is required"
			return
		}
		p.confirm = confirmation{kind: confirmCriterion, index: row.ref.idx, label: "Delete this success criterion?"}
	case row.kind == rowField && row.ref.kind == fieldConstraint:
		p.confirm = confirmation{kind: confirmConstraint, index: row.ref.idx, label: "Delete this constraint?"}
	case row.kind == rowStep:
		p.requestDeleteStep(row.step)
	case p.mode == viewDetail:
		p.requestDeleteStep(p.detailStep)
	}
}

func (p *Pane) requestDeleteStep(index int) {
	if index < 0 || index >= len(p.draft.Steps) {
		return
	}
	step := p.draft.Steps[index]
	if step.ID == "" && !step.isNew {
		p.err = "legacy id-less steps are read-only"
		return
	}
	if p.hasLegacyStep() {
		p.err = "legacy id-less steps block adding, deleting, or reordering steps"
		return
	}
	if step.Status != session.PlanPending {
		p.err = fmt.Sprintf("step %q is %s; only pending steps can be deleted", step.ID, step.Status)
		return
	}
	p.confirm = confirmation{kind: confirmStep, index: index, label: "Delete this pending step?"}
}

func (p *Pane) hasLegacyStep() bool {
	return slices.ContainsFunc(p.draft.Steps, func(step DraftStep) bool { return step.ID == "" && !step.isNew })
}

func (p *Pane) openText(ref fieldRef) {
	value := p.fieldValue(ref)
	p.textRef = ref
	p.textField = &input.TextField{
		Value: value, Cursor: len(value), MaxLines: 10, Style: p.theme.Foreground,
		PlaceholderStyle: p.theme.Muted, Placeholder: "Enter " + ref.label(),
	}
	p.textField.OnSubmit = func(string) { p.commitText() }
	p.err = ""
}

func (p *Pane) commitText() bool {
	if p.textField == nil {
		return false
	}
	value := strings.TrimSpace(p.textField.Value)
	ref := p.textRef
	adding := (ref.kind == fieldCriterion && ref.idx == len(p.draft.SuccessCriteria)) ||
		(ref.kind == fieldConstraint && ref.idx == len(p.draft.Constraints))
	previous := p.fieldValue(ref)
	if err := validateText(ref.label(), value, ref.limit(), ref.required()); err != nil {
		p.err = err.Error()
		return false
	}
	if ref.kind == fieldID && !stepIDPattern.MatchString(value) {
		p.err = "planedit: step id must be a lowercase slug using letters, digits, '.', '_' or '-'"
		return false
	}
	if err := p.setField(ref, value); err != nil {
		p.err = err.Error()
		return false
	}
	p.textField = nil
	if adding || previous != value {
		p.changed()
	} else {
		p.err = ""
	}
	return true
}

func (p *Pane) fieldValue(ref fieldRef) string {
	switch ref.kind {
	case fieldGoal:
		return p.draft.Goal
	case fieldApproach:
		return p.draft.Approach
	case fieldContext:
		return p.draft.WorkingContext
	case fieldCriterion:
		if ref.idx >= 0 && ref.idx < len(p.draft.SuccessCriteria) {
			return p.draft.SuccessCriteria[ref.idx].Value
		}
	case fieldConstraint:
		if ref.idx >= 0 && ref.idx < len(p.draft.Constraints) {
			return p.draft.Constraints[ref.idx].Value
		}
	default:
		if ref.step >= 0 && ref.step < len(p.draft.Steps) {
			step := p.draft.Steps[ref.step]
			switch ref.kind {
			case fieldID:
				return step.ID
			case fieldContent:
				return step.Content
			case fieldWhy:
				return step.Why
			case fieldDoneWhen:
				return step.DoneWhen
			case fieldNote:
				return step.Note
			case fieldRisk:
				return step.Risk
			case fieldSkills:
				if ref.idx >= 0 && ref.idx < len(step.Actions) {
					return strings.Join(step.Actions[ref.idx].Skills, ", ")
				}
			}
		}
	}
	return ""
}

func (p *Pane) setField(ref fieldRef, value string) error {
	switch ref.kind {
	case fieldGoal:
		p.draft.Goal = value
	case fieldApproach:
		p.draft.Approach = value
	case fieldContext:
		p.draft.WorkingContext = value
	case fieldCriterion:
		if ref.idx == len(p.draft.SuccessCriteria) {
			p.draft.SuccessCriteria = append(p.draft.SuccessCriteria, directiveDraft{Value: value, New: true})
		} else if ref.idx >= 0 && ref.idx < len(p.draft.SuccessCriteria) {
			p.draft.SuccessCriteria[ref.idx].Value = value
		} else {
			return errors.New("planedit: success criterion is no longer available")
		}
	case fieldConstraint:
		if ref.idx == len(p.draft.Constraints) {
			p.draft.Constraints = append(p.draft.Constraints, directiveDraft{Value: value, New: true})
		} else if ref.idx >= 0 && ref.idx < len(p.draft.Constraints) {
			p.draft.Constraints[ref.idx].Value = value
		} else {
			return errors.New("planedit: constraint is no longer available")
		}
	default:
		if ref.step < 0 || ref.step >= len(p.draft.Steps) {
			return errors.New("planedit: step is no longer available")
		}
		step := &p.draft.Steps[ref.step]
		switch ref.kind {
		case fieldID:
			for i, existing := range p.draft.Steps {
				if i != ref.step && existing.ID == value {
					return fmt.Errorf("planedit: step id %q already exists", value)
				}
			}
			step.ID = value
		case fieldContent:
			step.Content = value
		case fieldWhy:
			step.Why = value
		case fieldDoneWhen:
			step.DoneWhen = value
		case fieldNote:
			step.Note = value
		case fieldRisk:
			step.Risk = value
		case fieldSkills:
			if ref.idx < 0 || ref.idx >= len(step.Actions) {
				return errors.New("planedit: action is no longer available")
			}
			tokens := strings.FieldsFunc(value, func(r rune) bool {
				return r == ',' || r == ' ' || r == '\n'
			})
			if len(tokens) == 0 {
				return errors.New("planedit: at least one skill is required")
			}
			if len(tokens) > maxActionSkills {
				return fmt.Errorf("planedit: at most %d skills are allowed", maxActionSkills)
			}
			seen := make(map[string]bool, len(tokens))
			skills := make([]string, 0, len(tokens))
			for _, token := range tokens {
				if seen[token] {
					continue
				}
				seen[token] = true
				skills = append(skills, token)
			}
			step.Actions[ref.idx].Skills = skills
		}
	}
	return nil
}

func (p *Pane) changed() {
	p.dirty = true
	p.err = ""
}

func (p *Pane) apply() {
	if p.store == nil {
		p.err = "planedit: plan store unavailable"
		return
	}
	if p.readonly {
		p.err = p.readonlyReason
		return
	}
	ops, err := p.draft.ops(p.base, p.types)
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
		p.err = err.Error()
		return
	}
	if p.onApplied != nil {
		p.onApplied(plan)
	}
	p.Hide()
}

// Draw renders an opaque screen with a centered settings-style browser.
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
	switch p.mode {
	case viewDetail:
		title = " Step details "
	case viewTypes:
		title = " Choose step type "
	case viewModels:
		title = " Choose model "
	}
	if p.readonly {
		title = " Plan · read-only "
	}
	hint := " ↑↓ select · Enter open · Ctrl+S apply · Esc close "
	switch p.mode {
	case viewDetail:
		hint = " Enter edit/action · Del delete · Esc back "
	case viewTypes:
		hint = " Enter choose · Esc back "
	case viewModels:
		hint = " Enter pick · Esc back "
	}
	if p.confirm.kind != confirmNone {
		hint = " y confirm · n/Esc cancel "
	}
	layout.DrawRoundedBorder(
		&panel, layout.BorderRounded,
		xui.Style{Fg: th.Muted.Fg, Bg: th.BackgroundElement.Bg},
		&layout.BorderLabel{Text: title, Style: th.Foreground}, nil, nil,
		&layout.BorderLabel{Text: hint, Style: th.Muted}, ctx.Method,
	)

	meta := fmt.Sprintf("rev %d · %s", p.base.Revision, planState(p.base.Approved))
	if p.dirty {
		meta += " · unsaved · material edits may require approval again"
	}
	if pw > 4 && ph > 2 {
		panel.Print(2, 1, layout.TruncateToWidth(meta, pw-4, ctx.Method), th.Muted, ctx.Method)
	}

	bodyTop := 3
	view := max(ph-5, 0)
	previousViewport := p.viewport
	p.viewport = view
	rows := p.rows()
	p.overflow = len(rows) > view
	p.clampSelection()
	if previousViewport != view {
		p.followSelection()
	} else {
		p.clampScroll()
	}
	p.bodyTop, p.bodyLeft, p.bodyWidth = y0+bodyTop, x0+min(2, pw), max(pw-4, 0)
	for i := range view {
		idx := p.scroll + i
		if idx >= len(rows) || bodyTop+i >= ph-1 {
			break
		}
		style, marker := p.rowStyle(rows[idx], idx)
		panel.Print(
			2,
			bodyTop+i,
			layout.TruncateToWidth(marker+rows[idx].text, max(pw-5, 0), ctx.Method),
			style,
			ctx.Method,
		)
	}
	if p.overflow {
		drawScrollbar(&panel, max(pw-2, 0), bodyTop, view, len(rows), p.scroll, th.Muted)
	}
	message := p.err
	if p.confirm.kind != confirmNone {
		message = p.confirm.label + " (y/n)"
	}
	if message != "" && ph >= 3 && pw > 4 {
		panel.Print(2, ph-2, layout.TruncateToWidth(message, pw-4, ctx.Method), th.Warning, ctx.Method)
	}
	blit(&root, panel, x0, y0)
	if p.textField != nil {
		p.drawTextPopup(&root, ctx, th)
	}
	return root
}

func (p *Pane) drawTextPopup(root *components.Surface, ctx components.DrawContext, th components.Theme) {
	w, h := root.Size.Width, root.Size.Height
	pw := min(max(w-8, 18), 76)
	pw = min(pw, w)
	ph := min(max(h/2, 7), 16)
	ph = min(ph, h)
	x0, y0 := (w-pw)/2, (h-ph)/2
	popup := components.NewSurface(pw, ph, p)
	fillSurface(&popup, xui.Style{Fg: th.Foreground.Fg, Bg: th.BackgroundElement.Bg})
	label := fmt.Sprintf(
		" Edit %s · %d/%d ",
		p.textRef.label(),
		utf8.RuneCountInString(p.textField.Value),
		p.textRef.limit(),
	)
	layout.DrawRoundedBorder(
		&popup, layout.BorderRounded,
		xui.Style{Fg: th.ToolName.Fg, Bg: th.BackgroundElement.Bg},
		&layout.BorderLabel{Text: label, Style: th.Foreground}, nil, nil,
		&layout.BorderLabel{Text: " Enter save · Shift/Ctrl+Enter newline · Esc cancel ", Style: th.Muted}, ctx.Method,
	)
	innerW, innerH := max(pw-4, 1), max(ph-3, 1)
	field := p.textField.Draw(components.DrawContext{
		Max:    components.Size{Width: innerW, Height: innerH},
		Method: ctx.Method,
	})
	blit(&popup, field, min(2, pw-1), min(2, ph-1))
	blit(root, popup, x0, y0)
	if field.Cursor != nil {
		root.Cursor = &components.Point{X: x0 + min(2, pw-1) + field.Cursor.X, Y: y0 + min(2, ph-1) + field.Cursor.Y}
	}
}

func (p *Pane) rowStyle(row paneRow, idx int) (xui.Style, string) {
	if idx == p.selected && row.selectable {
		return xui.Style{Reverse: true}, "› "
	}
	switch row.kind {
	case rowHeading:
		return xui.Style{Bold: true, Fg: p.theme.Foreground.Fg}, "  "
	case rowInfo:
		return p.theme.Muted, "  "
	case rowAddCriterion,
		rowAddConstraint,
		rowAddStep,
		rowActionMoveUp,
		rowActionMoveDown,
		rowActionToggleJIT,
		rowActionDelete,
		rowModelType,
		rowModelChoice,
		rowStepModel,
		rowAddAction,
		rowActionEvent,
		rowActionType,
		rowActionSkills,
		rowActionRemove,
		rowActionBack:
		return p.theme.ToolName, "  "
	default:
		return p.theme.Foreground, "  "
	}
}

func (p *Pane) rows() []paneRow {
	switch p.mode {
	case viewDetail:
		return p.detailRows()
	case viewTypes:
		rows := make([]paneRow, 0, len(p.types))
		for i, typ := range p.types {
			rows = append(rows, paneRow{text: "  " + string(typ), kind: rowTypeChoice, step: i, selectable: true})
		}
		return rows
	case viewModels:
		rows := []paneRow{{text: "  (type default)", kind: rowModelChoice, step: -1, selectable: true}}
		for i, name := range p.models {
			rows = append(rows, paneRow{text: "  " + name, kind: rowModelChoice, step: i, selectable: true})
		}
		return rows
	default:
		return p.browseRows()
	}
}

func (p *Pane) browseRows() []paneRow {
	rows := []paneRow{{text: "Plan", kind: rowHeading}}
	addField := func(label string, ref fieldRef, value string) {
		rows = append(rows, paneRow{
			text: "  " + label + ": " + compactValue(value),
			kind: rowField, ref: ref, selectable: true,
		})
	}
	addField("Goal", fieldRef{kind: fieldGoal, step: -1}, p.draft.Goal)
	addField("Approach", fieldRef{kind: fieldApproach, step: -1}, p.draft.Approach)
	addField("Context", fieldRef{kind: fieldContext, step: -1}, p.draft.WorkingContext)
	rows = append(rows, paneRow{text: "Success criteria", kind: rowHeading})
	for i, entry := range p.draft.SuccessCriteria {
		addField(strconv.Itoa(i+1), fieldRef{kind: fieldCriterion, idx: i, step: -1}, entry.Value)
	}
	rows = append(rows,
		paneRow{text: "  + Add success criterion", kind: rowAddCriterion, selectable: true},
		paneRow{text: "Constraints", kind: rowHeading},
	)
	for i, entry := range p.draft.Constraints {
		addField(strconv.Itoa(i+1), fieldRef{kind: fieldConstraint, idx: i, step: -1}, entry.Value)
	}
	rows = append(rows,
		paneRow{text: "  + Add constraint", kind: rowAddConstraint, selectable: true},
		paneRow{text: "Steps", kind: rowHeading},
	)
	for i, step := range p.draft.Steps {
		label := fmt.Sprintf(
			"  %d %s %s — %s",
			i+1,
			statusIcon(step.Status),
			stepTypeLabel(step.Type),
			compactValue(step.Content),
		)
		if step.ID == "" && !step.isNew {
			label += " (legacy id-less; read-only)"
		}
		rows = append(rows, paneRow{text: label, kind: rowStep, step: i, selectable: true})
	}
	if len(p.draft.Steps) == 0 {
		rows = append(rows, paneRow{text: "  (no steps)", kind: rowInfo})
	}
	rows = append(rows, paneRow{text: "  + Add step", kind: rowAddStep, selectable: true})

	// The settings section: one pin per configured step type. Clearing a
	// pin hands that type back to the session default.
	rows = append(rows, paneRow{text: "Step models", kind: rowHeading})
	if len(p.types) == 0 {
		rows = append(rows, paneRow{text: "  (no step types configured)", kind: rowInfo})
	} else {
		for i, typ := range p.types {
			label := "(type default)"
			if name := p.draft.ModelsByType[typ]; name != "" {
				label = name
			}
			rows = append(rows, paneRow{
				text: "  " + string(typ) + ": " + label, kind: rowModelType, step: i, selectable: true,
			})
		}
	}
	return rows
}

func (p *Pane) detailRows() []paneRow {
	if p.detailStep < 0 || p.detailStep >= len(p.draft.Steps) {
		return []paneRow{{text: "Step is no longer available", kind: rowInfo}}
	}
	step := p.draft.Steps[p.detailStep]
	rows := []paneRow{{text: "Identity (read-only after creation)", kind: rowHeading}}
	if step.isNew {
		rows = append(rows,
			paneRow{
				text: "  ID: " + compactValue(step.ID), kind: rowField,
				ref: fieldRef{kind: fieldID, step: p.detailStep}, selectable: true,
			},
			paneRow{
				text: "  Type: " + stepTypeLabel(step.Type) + " — choose…",
				kind: rowTypeChoice, step: p.detailStep, selectable: true,
			},
		)
	} else {
		rows = append(rows,
			paneRow{text: "  ID: " + compactValue(step.ID), kind: rowInfo},
			paneRow{text: "  Type: " + stepTypeLabel(step.Type), kind: rowInfo},
		)
	}
	jitPosture := "disabled"
	if step.JIT {
		jitPosture = "enabled"
	}
	rows = append(rows,
		paneRow{text: "  Status: " + string(step.Status), kind: rowInfo},
		paneRow{text: "  Just-in-time: " + jitPosture + " (read-only after creation)", kind: rowInfo},
		paneRow{text: "Contract", kind: rowHeading},
	)
	if step.ID == "" && !step.isNew {
		rows = append(rows,
			paneRow{text: "  Content: " + compactValue(step.Content), kind: rowInfo},
			paneRow{text: "  Why: " + compactValue(step.Why), kind: rowInfo},
			paneRow{text: "  Done when: " + compactValue(step.DoneWhen), kind: rowInfo},
			paneRow{text: "  Note: " + compactValue(step.Note), kind: rowInfo},
			paneRow{text: "  Risk: " + compactValue(step.Risk), kind: rowInfo},
		)
	} else {
		for _, spec := range []struct {
			label string
			kind  fieldKind
			value string
		}{
			{"Content", fieldContent, step.Content},
			{"Why", fieldWhy, step.Why},
			{"Done when", fieldDoneWhen, step.DoneWhen},
			{"Note", fieldNote, step.Note},
			{"Risk", fieldRisk, step.Risk},
		} {
			rows = append(rows, paneRow{
				text: "  " + spec.label + ": " + compactValue(spec.value), kind: rowField,
				ref: fieldRef{kind: spec.kind, step: p.detailStep}, selectable: true,
			})
		}
	}
	if step.isNew {
		rows = append(rows, paneRow{
			text: "  Toggle just-in-time posture (currently " + jitPosture + ")",
			kind: rowActionToggleJIT, step: p.detailStep, selectable: true,
		})
	}
	rows = append(rows, paneRow{text: "Actions", kind: rowHeading})
	if step.ID != "" || step.isNew {
		rows = append(rows,
			paneRow{text: "  ↑ Move step up", kind: rowActionMoveUp, step: p.detailStep, selectable: true},
			paneRow{text: "  ↓ Move step down", kind: rowActionMoveDown, step: p.detailStep, selectable: true},
			paneRow{text: "  Delete pending step…", kind: rowActionDelete, step: p.detailStep, selectable: true},
		)
	}
	if step.ID != "" || step.isNew {
		// Automation lives with the step: actions first, then the model pin —
		// both compile into one update_step patch.
		rows = append(rows, paneRow{text: "Automation", kind: rowHeading})
		for i, action := range step.Actions {
			ref := fieldRef{kind: fieldSkills, step: p.detailStep, idx: i}
			rows = append(rows, paneRow{
				text: fmt.Sprintf("  ⚙ %d %s@%s — Enter: next event", i+1, action.Type, action.Event),
				kind: rowActionEvent, ref: ref, step: p.detailStep, selectable: true,
			})
			nextType := session.PlanActionInjectSkill
			if action.Type == session.PlanActionInjectSkill {
				nextType = session.PlanActionCompact
			}
			rows = append(rows, paneRow{
				text: fmt.Sprintf("  ⚙ %d %s · type → %s", i+1, action.Type, nextType),
				kind: rowActionType, ref: ref, step: p.detailStep, selectable: true,
			})
			if action.Type == session.PlanActionInjectSkill {
				skills := strings.Join(action.Skills, ", ")
				if skills == "" {
					skills = "(none)"
				}
				rows = append(rows, paneRow{
					text: fmt.Sprintf("  ⚙ %d inject_skill · skills: %s", i+1, skills),
					kind: rowActionSkills, ref: ref, step: p.detailStep, selectable: true,
				})
			}
			rows = append(rows, paneRow{
				text: fmt.Sprintf("  ⚙ %d %s · remove", i+1, action.Type),
				kind: rowActionRemove, ref: ref, step: p.detailStep, selectable: true,
			})
		}
		rows = append(rows, paneRow{text: "  + Add action", kind: rowAddAction, step: p.detailStep, selectable: true})
		model := step.Model
		if model == "" {
			model = "(type default)"
		}
		rows = append(rows, paneRow{
			text: "  Model: " + model, kind: rowStepModel, step: p.detailStep, selectable: true,
		})
	}
	rows = append(rows, paneRow{text: "  ← Back to plan", kind: rowActionBack, selectable: true})
	return rows
}

func compactValue(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "(none)"
	}
	return value
}

func planState(approved bool) string {
	if approved {
		return "approved"
	}
	return "draft"
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

func stepTypeLabel(typ session.StepType) string {
	if typ == "" {
		return "step"
	}
	return string(typ)
}

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
	if height <= 0 || total <= height || x < 0 || x >= surface.Size.Width {
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

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
