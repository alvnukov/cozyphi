// Package planedit renders the durable plan viewer/editor modal. Pane owns
// only an editable draft and interaction state; persistence stays behind Store.
package planedit

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/input"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/browse"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
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
	Jumping    bool
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
		d.Steps = append(d.Steps, draftStep(item, i))
	}
	return d
}

// draftStep binds one durable step to the index the ops compiler diffs it
// against.
func draftStep(item session.PlanItem, index int) DraftStep {
	return DraftStep{
		ID: item.ID, Content: item.Content, Type: item.Type, Status: item.Status,
		Why: item.Why, DoneWhen: item.DoneWhen, Note: item.Note, Risk: item.Risk, JIT: item.JIT,
		Model: item.Model, Actions: append([]session.PlanAction(nil), item.Actions...),
		baseIndex: index, baseID: item.ID,
	}
}

func patchValue(value string) session.PatchValue[string] {
	return session.PatchValue[string]{Set: true, Value: value}
}

// authoredActions strips run history and compact-irrelevant skills: the
// durable patch path rejects authored lists that carry runs, so the editor
// never re-authors history it only displays. Off marks ride along for the
// names the authored list still carries.
func authoredActions(actions []session.PlanAction) []session.PlanAction {
	out := make([]session.PlanAction, 0, len(actions))
	for _, action := range actions {
		clean := session.PlanAction{Event: action.Event, Type: action.Type}
		if action.Type == session.PlanActionInjectSkill {
			clean.Skills = action.Skills
			for _, name := range action.DisabledSkills {
				if slices.Contains(action.Skills, name) {
					clean.DisabledSkills = append(clean.DisabledSkills, name)
				}
			}
		}
		out = append(out, clean)
	}
	return out
}

// skillListSummary renders the authored skill list for the detail row,
// marking each user-disabled name so the row reads the effective set: the
// skills a toggle has switched off stay visible with their off mark instead
// of silently disappearing from the editor.
func skillListSummary(action session.PlanAction) string {
	if len(action.Skills) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(action.Skills))
	for _, name := range action.Skills {
		if slices.Contains(action.DisabledSkills, name) {
			parts = append(parts, name+" (off)")
		} else {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, ", ")
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
		if !slices.EqualFunc(authoredActions(step.Actions), authoredActions(item.Actions), session.PlanActionEqual) {
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

	// Insertions anchor on a surviving base step where there is one. A plan
	// with no steps left to keep needs no anchor: the first insert has one
	// place to land and the rest chain onto it, in draft order.
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
	}

	// Remove obsolete pending steps first to free the 32-step budget. If every
	// old step is being replaced, one old step stays only long enough to anchor
	// all insertions; it is removed immediately afterward.
	for _, item := range deletions {
		if item.ID != heldAnchor {
			ops = append(ops, session.PlanPatchOp{Op: session.PlanPatchRemoveStep, ID: item.ID})
		}
	}
	chained := "" // id the next insert follows while the plan grows from empty
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
		insert := session.PlanPatchOp{Op: session.PlanPatchInsertStep, Step: item}
		switch {
		case anchor != "":
			insert.Before = anchor
		case chained != "":
			insert.After = chained
		}
		chained = step.ID
		ops = append(ops, insert)
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
	// A plan built from nothing is already in draft order, so the reorder is
	// dead weight — and dropping it keeps a plan authored to the step cap in
	// one apply inside the patch-op budget.
	structural := len(deletions) > 0 || len(newSteps) > 0
	moved := !slices.Equal(finalIDs, baseRemaining)
	if (structural || moved) && len(finalIDs) > 0 && len(base.Items) > 0 {
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

// directiveRename records that a durable directive keeps its slot under a new
// value. Directives are addressed by value, so the order renames are applied in
// matters: a rename cannot land on a value the list still holds.
type directiveRename struct{ from, to string }

func directiveOps(draft []directiveDraft, base []string, criterion bool) []session.PlanPatchOp {
	update, add, remove := session.PlanPatchUpdateConstraint,
		session.PlanPatchAddConstraint,
		session.PlanPatchRemoveConstraint
	if criterion {
		update, add, remove = session.PlanPatchUpdateCriterion,
			session.PlanPatchAddCriterion,
			session.PlanPatchRemoveCriterion
	}

	kept := make(map[string]bool, len(draft))
	var renames []directiveRename
	var additions []string
	for _, entry := range draft {
		value := strings.TrimSpace(entry.Value)
		if entry.New {
			additions = append(additions, value)
			continue
		}
		kept[entry.Original] = true
		if value != entry.Original {
			renames = append(renames, directiveRename{from: entry.Original, to: value})
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
			renames = append(renames, directiveRename{from: removals[i], to: additions[i]})
		}
	}
	removals, additions = removals[paired:], additions[paired:]

	ops := make([]session.PlanPatchOp, 0, len(removals)+len(additions)+len(renames)*2)
	current := make(map[string]bool, len(base))
	for _, value := range base {
		current[value] = true
	}
	for _, value := range removals {
		ops = append(ops, session.PlanPatchOp{Op: remove, Value: value})
		delete(current, value)
	}

	// A value put back by a broken cycle is appended, so it is added after every
	// rename has run and before the entries the user typed as new.
	var readded []string
	for len(renames) > 0 {
		index, cycle := nextRenameIndex(renames, current, base)
		change := renames[index]
		renames = slices.Delete(renames, index, index+1)
		delete(current, change.from)
		if cycle {
			// Every remaining target is held by another remaining rename, so no
			// rename can run. Free one name by removing its holder and putting
			// its value back at the end: the batch stays free of values the user
			// never authored.
			ops = append(ops, session.PlanPatchOp{Op: remove, Value: change.from})
			readded = append(readded, change.to)
			continue
		}
		ops = append(ops, session.PlanPatchOp{Op: update, From: change.from, To: change.to})
		current[change.to] = true
	}
	for _, value := range slices.Concat(readded, additions) {
		ops = append(ops, session.PlanPatchOp{Op: add, Value: value})
	}
	return ops
}

// nextRenameIndex picks the rename to compile next: one whose target the list no
// longer holds, or — when the renames form a cycle and none can run, reported by
// the second result — the one to break the cycle on. Breaking costs a remove and
// an append, so it falls on the entry the base plan holds last and the surviving
// entries keep their relative order.
func nextRenameIndex(renames []directiveRename, current map[string]bool, base []string) (int, bool) {
	for i, change := range renames {
		if !current[change.to] {
			return i, false
		}
	}
	last, lastPos := 0, -1
	for i, change := range renames {
		if pos := slices.Index(base, change.from); pos > lastPos {
			last, lastPos = i, pos
		}
	}
	return last, true
}

type viewMode uint8

const (
	viewBrowse viewMode = iota
	viewDetail
	viewTypes
	viewModels
	viewActionEvent
	viewActionType
	viewMenu
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
	rowEventChoice
	rowActionKindChoice
	rowActionBack
	rowInfo
	// rowText is a plain foreground line: the preview pane's wrapped values.
	rowText
	// rowMenuItem is one entry of the `.` action menu; its step field
	// indexes Pane.menu.
	rowMenuItem
)

type paneRow struct {
	text       string
	kind       rowKind
	ref        fieldRef
	step       int
	selectable bool
}

// Pane is a full-screen modal, mutated and rendered only by the UI goroutine.
type Pane struct {
	theme components.Theme
	store Store

	onClose func()
	visible bool

	base           session.Plan
	draft          Draft
	types          []session.StepType
	models         []string
	modelType      session.StepType // the type a model picker is editing
	modelStep      int              // the step a model picker edits; -1 means a type target
	actionStep     int              // the step whose action a choice screen edits
	actionIdx      int              // the action a choice screen edits
	dirty          bool
	err            string
	readonly       bool
	readonlyReason string
	mode           viewMode
	detailStep     int

	// baseline is the draft exactly as Show (or a rebase) built it; dirtiness
	// and the ● row markers compare against it. mark is a private clone of the
	// draft after the last recorded change — the state an undo returns to —
	// and undo/redo hold uniquely-owned drafts, oldest first.
	baseline Draft
	mark     Draft
	undo     []Draft
	redo     []Draft

	textRef   fieldRef
	textField *input.TextField
	confirm   browse.Confirm

	// Fuzzy jump (`/`): the query strip at the bottom moves the active
	// cursor to the best match live; Esc returns to jumpOrigin.
	jump        *input.TextField
	jumpOrigin  int
	jumpMatches []int
	jumpPos     int

	// The `.` action menu: the items the open viewMenu offers, and the
	// mode to return to when it closes.
	menu       []menuItem
	menuReturn viewMode

	// One cursor per list role, so a round trip — into a step's details, into
	// a choice, and back — returns to the row it left, not to the top.
	motions   browse.Motions
	browseCur browse.Cursor
	detailCur browse.Cursor
	choiceCur browse.Cursor
	viewport  int
	overflow  bool

	// Two-pane state: whether the last Draw split the panel, which step the
	// right pane previews, and how far the preview is wheeled.
	twoPaneOn     bool
	previewStep   int
	previewSel    int
	previewScroll int

	bodyTop   int
	masterHit hitRegion
	detailHit hitRegion
}

// menuItem is one row of the `.` action menu: a label naming the command
// (and its chord, when it has one) and the command itself, run after the
// menu closes back into the mode it came from.
type menuItem struct {
	label string
	run   func()
}

// hitRegion is a horizontal mouse target from the last Draw; zero width
// means the pane was not drawn.
type hitRegion struct {
	left, width int
}

func (r hitRegion) contains(x int) bool { return x >= r.left && x < r.left+r.width }

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

// Show opens a fresh draft of the latest durable plan.
func (p *Pane) Show() {
	if p == nil {
		return
	}
	p.visible = true
	p.dirty, p.err = false, ""
	p.mode, p.detailStep = viewBrowse, -1
	p.textField = nil
	p.jump, p.jumpMatches = nil, nil
	p.menu = nil
	p.confirm.Disarm()
	p.motions.Reset()
	if p.store != nil {
		p.base = p.store.Snapshot()
		p.types = slices.Clone(p.store.StepTypes())
		p.models = append([]string(nil), p.store.Models()...)
	} else {
		p.base = session.Plan{}
		p.types = nil
	}
	p.draft = newDraft(p.base)
	p.baseline = cloneDraft(p.draft)
	p.mark = cloneDraft(p.draft)
	p.undo, p.redo = nil, nil
	p.readonlyReason = ""
	switch {
	case !p.base.Schema.IsV2():
		p.readonlyReason = "legacy plan: only v2 plans can be edited"
	case p.hasLegacyStep():
		p.readonlyReason = "plan contains legacy id-less steps; migration is required before editing"
	}
	p.readonly = p.readonlyReason != ""
	p.browseCur, p.detailCur, p.choiceCur = browse.Cursor{}, browse.Cursor{}, browse.Cursor{}
	p.previewStep, p.previewSel, p.previewScroll = -1, -1, 0
	p.syncRows()
}

// Hide discards the draft and closes the modal.
func (p *Pane) Hide() {
	if p == nil || !p.visible {
		return
	}
	p.visible = false
	p.draft, p.baseline, p.mark = Draft{}, Draft{}, Draft{}
	p.undo, p.redo = nil, nil
	p.textField = nil
	p.jump, p.jumpMatches = nil, nil
	p.menu = nil
	p.confirm.Disarm()
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
	cur := p.activeCursor()
	return State{
		Selected: cur.Selected(), Scroll: cur.Scroll(), Overflow: p.overflow, Dirty: p.dirty,
		Error: p.err, Editing: p.textField != nil, Jumping: p.jump != nil,
		Detail:     p.mode == viewDetail,
		Confirming: p.confirm.Armed(), Readonly: p.readonly,
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
	if p.jump != nil {
		p.handleJumpEvent(ctx, ev)
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
		// Ctrl+S saves the field, like Enter: the durable plan write stays on
		// the step list, the level that owns the plan and advertises the key.
		if key.Code == xui.KeyRune && key.Mods == xui.ModCtrl && key.HotkeyRune() == 's' {
			p.commitText()
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

// openJump starts the `/` fuzzy jump on the active list: the strip at the
// bottom takes the keyboard, and every keystroke moves the selection to
// the best-matching row live.
func (p *Pane) openJump() {
	p.motions.Reset()
	p.syncRows()
	p.jumpOrigin = p.activeCursor().Selected()
	p.jump = &input.TextField{
		MaxLines: 1, Style: p.theme.Foreground,
		PlaceholderStyle: p.theme.Muted, Placeholder: "type to jump",
	}
	p.jumpMatches, p.jumpPos = nil, 0
	p.err = ""
}

func (p *Pane) handleJumpEvent(ctx *components.EventContext, ev xui.Event) {
	if mouse, ok := ev.(xui.MouseEvent); ok {
		// A click is already a jump of its own: keep what the query found
		// and let the click act normally.
		p.jump = nil
		p.handleMouse(mouse)
		return
	}
	if key, ok := ev.(xui.KeyEvent); ok && key.Press {
		switch key.Code {
		case xui.KeyEscape:
			p.activeCursor().Select(p.jumpOrigin)
			p.jump = nil
			return
		case xui.KeyEnter:
			p.jump = nil
			return
		case xui.KeyDown:
			p.cycleJump(1)
			return
		case xui.KeyUp:
			p.cycleJump(-1)
			return
		}
	}
	before := p.jump.Value
	p.jump.Handle(ctx, ev)
	if p.jump != nil && p.jump.Value != before {
		p.refreshJump()
	}
}

// refreshJump recomputes the match list for the current query and parks
// the selection on the best match. No match leaves the selection where
// the last one put it; the strip label says so.
func (p *Pane) refreshJump() {
	p.jumpMatches, p.jumpPos = nil, 0
	query := strings.TrimSpace(p.jump.Value)
	if query == "" {
		return
	}
	rows := p.rows()
	type match struct{ idx, score int }
	var found []match
	for i, row := range rows {
		if !row.selectable {
			continue
		}
		if score, ok := fuzzyScore(row.text, query); ok {
			found = append(found, match{i, score})
		}
	}
	slices.SortStableFunc(found, func(a, b match) int { return a.score - b.score })
	for _, m := range found {
		p.jumpMatches = append(p.jumpMatches, m.idx)
	}
	if len(p.jumpMatches) > 0 {
		p.activeCursor().Select(p.jumpMatches[0])
	}
}

func (p *Pane) cycleJump(delta int) {
	if len(p.jumpMatches) == 0 {
		return
	}
	p.jumpPos = (p.jumpPos + delta + len(p.jumpMatches)) % len(p.jumpMatches)
	p.activeCursor().Select(p.jumpMatches[p.jumpPos])
}

// fuzzyScore reports whether query is a case-folded subsequence of text,
// and how tight the leftmost such match is: the span it covers first,
// its start second — lower is better.
func fuzzyScore(text, query string) (int, bool) {
	q := []rune(strings.ToLower(query))
	if len(q) == 0 {
		return 0, true
	}
	start, last, qi := -1, 0, 0
	for i, r := range strings.ToLower(text) {
		if qi < len(q) && r == q[qi] {
			if qi == 0 {
				start = i
			}
			last = i
			qi++
			if qi == len(q) {
				break
			}
		}
	}
	if qi < len(q) {
		return 0, false
	}
	return (last-start)*1000 + start, true
}

func (p *Pane) handleKey(event xui.KeyEvent) {
	if p.confirm.Key(event) {
		p.err = ""
		return
	}
	if event.Code == xui.KeyRune && event.Mods == xui.ModCtrl {
		switch event.HotkeyRune() {
		case 's':
			p.motions.Reset()
			p.apply()
			return
		case 'z':
			p.motions.Reset()
			p.undoEdit()
			return
		case 'y':
			p.motions.Reset()
			p.redoEdit()
			return
		}
		// Ctrl+U and Ctrl+D fall through to the motion dialect.
	}
	if event.Code == xui.KeyUp || event.Code == xui.KeyDown {
		delta := 1
		if event.Code == xui.KeyUp {
			delta = -1
		}
		if event.Mods.Has(xui.ModAlt) {
			p.motions.Reset()
			p.moveStepBy(delta)
			return
		}
		if event.Mods.Has(xui.ModShift) {
			// Shift+arrows extend a selection everywhere else in the TUI, so
			// they must never mutate the plan here.
			p.motions.Reset()
			p.err = "shift+↑↓ does nothing here — press alt+↑↓ to move a step"
			return
		}
	}
	if m, ok := p.motions.Key(event); ok {
		p.syncRows()
		p.activeCursor().Apply(m)
		return
	}
	switch event.Code {
	case xui.KeyEscape:
		if p.mode == viewMenu {
			p.mode = p.menuReturn
			p.menu = nil
			p.restoreSelection()
			return
		}
		if p.mode == viewModels {
			if p.modelStep >= 0 {
				p.mode = viewDetail
			} else {
				p.mode = viewBrowse
			}
			p.restoreSelection()
			return
		}
		if p.mode == viewActionEvent || p.mode == viewActionType {
			p.mode = viewDetail
			p.restoreSelection()
			return
		}
		if p.mode == viewTypes {
			p.mode = viewDetail
			p.restoreSelection()
			return
		}
		if p.mode == viewDetail {
			// Back on the step list, the cursor parks on the step it visited
			// — which may have moved while its details were open.
			step := p.detailStep
			p.mode, p.detailStep = viewBrowse, -1
			p.restoreSelection()
			p.selectStepRow(step)
			return
		}
		if p.dirty {
			p.confirm.Arm("Discard all unsaved plan changes?", p.Hide)
		} else {
			p.Hide()
		}
	case xui.KeyEnter:
		p.activateSelected()
	case xui.KeyDelete:
		if p.inChoice() {
			p.err = choiceKeyMessage
			return
		}
		p.requestDeleteSelected()
	case xui.KeyBackspace:
		if p.inChoice() {
			p.err = choiceKeyMessage
			return
		}
		// Backspace also deleted here once, which no footer ever promised and
		// which reads as "go back" in a list. Say what works instead of
		// swallowing the key — or worse, destroying a row.
		p.err = "backspace does nothing here — press Del to delete"
	case xui.KeyRune:
		// The motion dialect took everything else; what remains of the rune
		// space is the Enter synonym, the jump and the menu.
		switch {
		case event.Mods == 0 && event.Rune == ' ':
			p.activateSelected()
		case event.Mods == 0 && event.Rune == '/' && !p.inChoice():
			p.openJump()
		case event.Mods == 0 && event.Rune == '.' && !p.inChoice():
			p.openMenu()
		}
	}
}

// choiceKeyMessage answers a delete key in a choice list, where nothing can
// be deleted: the list only picks a value.
const choiceKeyMessage = "this list only picks — Enter chooses, Esc goes back"

func (p *Pane) inChoice() bool {
	switch p.mode {
	case viewTypes, viewModels, viewActionEvent, viewActionType, viewMenu:
		return true
	}
	return false
}

func (p *Pane) handleMouse(event xui.MouseEvent) {
	if event.Action == xui.MousePress && event.Button == xui.MouseLeft {
		// A click is acting elsewhere: it withdraws an armed question the
		// same way a foreign key does.
		p.confirm.Disarm()
		if event.Y < p.bodyTop || event.Y >= p.bodyTop+p.viewport {
			return
		}
		local := event.Y - p.bodyTop
		activeHit := p.masterHit
		if p.twoPaneOn && p.mode == viewDetail {
			activeHit = p.detailHit
		}
		switch {
		case activeHit.contains(event.X):
			p.clickActiveRow(local)
		case p.twoPaneOn && p.mode == viewDetail && p.masterHit.contains(event.X):
			// Focus follows the click: back on the step list, acting on the
			// clicked row directly.
			step := p.detailStep
			p.mode, p.detailStep = viewBrowse, -1
			p.restoreSelection()
			p.selectStepRow(step)
			p.clickActiveRow(local)
		case p.twoPaneOn && p.mode == viewBrowse && p.detailHit.contains(event.X) && p.previewStep >= 0:
			// A click in the preview focuses the step it shows and lands on
			// the clicked row; the preview's scroll carries over.
			scroll := p.previewScroll
			p.mode, p.detailStep = viewDetail, p.previewStep
			p.resetSelection()
			rows := p.rows()
			if idx := scroll + local; idx >= 0 && idx < len(rows) && rows[idx].selectable {
				p.detailCur.Select(idx)
			}
		}
		return
	}
	// The wheel scrolls the pane under the pointer — the preview and the
	// unfocused browse list included.
	if p.twoPaneOn && p.mode == viewBrowse && p.detailHit.contains(event.X) {
		if m, ok := browse.Wheel(event); ok {
			// Clamped against the preview's content on the next Draw.
			p.previewScroll += m.N
		}
		return
	}
	if p.twoPaneOn && p.mode == viewDetail && p.masterHit.contains(event.X) {
		rows := p.browseRows()
		p.browseCur.SetRows(len(rows), func(i int) bool { return rows[i].selectable })
		p.browseCur.Wheel(event)
		return
	}
	p.syncRows()
	p.activeCursor().Wheel(event)
}

// clickActiveRow selects and activates the active list's row under the
// pointer.
func (p *Pane) clickActiveRow(local int) {
	p.syncRows()
	cur := p.activeCursor()
	idx := cur.Scroll() + local
	rows := p.rows()
	if idx >= 0 && idx < len(rows) && rows[idx].selectable {
		cur.Select(idx)
		p.activate(rows[idx])
	}
}

// activeCursor is the cursor of the list the keyboard drives: the browse
// list, the step details, or a choice list.
func (p *Pane) activeCursor() *browse.Cursor {
	switch p.mode {
	case viewDetail:
		return &p.detailCur
	case viewTypes, viewModels, viewActionEvent, viewActionType, viewMenu:
		return &p.choiceCur
	default:
		return &p.browseCur
	}
}

// resetSelection starts the active list from the top — for a list being
// entered fresh, never for one being returned to.
func (p *Pane) resetSelection() {
	*p.activeCursor() = browse.Cursor{}
	p.syncRows()
}

// restoreSelection re-clamps the active cursor after a mode switch back to a
// list whose selection survives the round trip.
func (p *Pane) restoreSelection() {
	p.syncRows()
	cur := p.activeCursor()
	cur.Select(cur.Selected())
}

// preselect parks the cursor on one choice row and keeps it in view, so a
// choice list opens on the current value instead of the top.
func (p *Pane) preselect(idx int) {
	*p.activeCursor() = browse.Cursor{}
	p.syncRows()
	p.activeCursor().Select(idx)
}

// stepTypeIndex is the choice row of a step's current type; an unknown type
// falls back to the top of the list.
func stepTypeIndex(types []session.StepType, current session.StepType) int {
	if i := slices.Index(types, current); i >= 0 {
		return i
	}
	return 0
}

// modelChoiceIndex is the choice row of a pinned model, in a list whose row
// zero is "(type default)"; no pin, or a pin the catalog no longer carries,
// lands there.
func (p *Pane) modelChoiceIndex(name string) int {
	if i := slices.Index(p.models, name); name != "" && i >= 0 {
		return i + 1
	}
	return 0
}

// syncRows tells the cursor about the current row list. The list changes
// with every mode switch and draft edit, so anything that moves or reads
// the cursor refreshes it first.
func (p *Pane) syncRows() {
	rows := p.rows()
	p.activeCursor().SetRows(len(rows), func(i int) bool { return rows[i].selectable })
}

func (p *Pane) activateSelected() {
	rows := p.rows()
	if sel := p.activeCursor().Selected(); sel >= 0 && sel < len(rows) {
		p.activate(rows[sel])
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
			p.restoreSelection()
		} else if p.detailStep >= 0 && p.draft.Steps[p.detailStep].isNew {
			p.mode = viewTypes
			p.preselect(stepTypeIndex(p.types, p.draft.Steps[p.detailStep].Type))
		}
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
			p.preselect(p.modelChoiceIndex(p.draft.ModelsByType[p.modelType]))
		}
	case rowStepModel:
		p.modelStep = p.detailStep
		p.mode = viewModels
		current := ""
		if p.modelStep >= 0 && p.modelStep < len(p.draft.Steps) {
			current = p.draft.Steps[p.modelStep].Model
		}
		p.preselect(p.modelChoiceIndex(current))
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
		p.restoreSelection()
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
			// A choice screen, not a blind cycle: Enter on this row opens
			// the event list with the current value preselected.
			p.actionStep, p.actionIdx = p.detailStep, row.ref.idx
			p.mode = viewActionEvent
			p.preselect(actionEventIndex(action.Event))
			return
		case rowActionType:
			p.actionStep, p.actionIdx = p.detailStep, row.ref.idx
			p.mode = viewActionType
			p.preselect(actionTypeIndex(action.Type))
			return
		case rowActionSkills:
			p.openText(fieldRef{kind: fieldSkills, step: p.detailStep, idx: row.ref.idx})
			return
		case rowActionRemove:
			step.Actions = slices.Delete(step.Actions, row.ref.idx, row.ref.idx+1)
		}
		p.changed()
	case rowEventChoice, rowActionKindChoice:
		if p.actionStep < 0 || p.actionStep >= len(p.draft.Steps) {
			return
		}
		step := &p.draft.Steps[p.actionStep]
		if p.actionIdx < 0 || p.actionIdx >= len(step.Actions) {
			return
		}
		action := &step.Actions[p.actionIdx]
		if row.kind == rowEventChoice {
			action.Event = actionEventOptions()[row.step]
		} else if action.Type != actionTypeOptions()[row.step] {
			// Switching away from inject_skill drops its skill list: a
			// compact action carrying stale skills would fail validation.
			action.Type = actionTypeOptions()[row.step]
			action.Skills = nil
		}
		p.mode = viewDetail
		p.changed()
		p.restoreSelection()
	case rowActionBack:
		step := p.detailStep
		p.mode, p.detailStep = viewBrowse, -1
		p.restoreSelection()
		p.selectStepRow(step)
	case rowMenuItem:
		if row.step < 0 || row.step >= len(p.menu) {
			return
		}
		item := p.menu[row.step]
		// Leave the menu first: the command runs in the mode it was called
		// from, exactly as its chord would.
		p.mode = p.menuReturn
		p.menu = nil
		p.restoreSelection()
		item.run()
	}
}

// openMenu builds the `.` action menu: the commands that apply to the
// selected row — the chords a user may not know yet, as rows — plus the
// plan-wide commands that are always worth reaching.
func (p *Pane) openMenu() {
	if p.readonly {
		p.err = p.readonlyReason
		return
	}
	p.motions.Reset()
	p.syncRows()
	rows := p.rows()
	var row paneRow
	if sel := p.activeCursor().Selected(); sel >= 0 && sel < len(rows) {
		row = rows[sel]
	}
	var items []menuItem
	switch p.mode {
	case viewBrowse:
		switch row.kind {
		case rowStep:
			step := row.step
			items = append(items,
				menuItem{"Open step details (Enter)", func() { p.activate(row) }},
				menuItem{"Move step up (Alt+↑)", func() { p.moveStepBy(-1) }},
				menuItem{"Move step down (Alt+↓)", func() { p.moveStepBy(1) }},
				menuItem{"Delete step (Del)", func() { p.requestDeleteStep(step) }},
			)
		case rowField:
			items = append(items, menuItem{"Edit " + row.ref.label() + " (Enter)", func() { p.activate(row) }})
			if row.ref.kind == fieldCriterion || row.ref.kind == fieldConstraint {
				items = append(items, menuItem{"Delete (Del)", p.requestDeleteSelected})
			}
		}
		items = append(items, menuItem{"Add step", p.addStep})
	case viewDetail:
		items = append(items,
			menuItem{"Move step up (Alt+↑)", func() { p.moveStepBy(-1) }},
			menuItem{"Move step down (Alt+↓)", func() { p.moveStepBy(1) }},
			menuItem{"Delete step (Del)", func() { p.requestDeleteStep(p.detailStep) }},
		)
	default:
		return
	}
	items = append(items, menuItem{"Apply changes (Ctrl+S)", p.apply})
	if len(p.undo) > 0 {
		items = append(items, menuItem{"Undo last edit (Ctrl+Z)", p.undoEdit})
	}
	if len(p.redo) > 0 {
		items = append(items, menuItem{"Redo (Ctrl+Y)", p.redoEdit})
	}
	p.menu, p.menuReturn = items, p.mode
	p.mode = viewMenu
	p.resetSelection()
	p.err = ""
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

// moveStepBy reorders the plan around the step the cursor is on: the selected
// step row in the list, or the open step in its details. The selection follows
// the moved step, so a held Shift+↓ walks it down the plan visibly.
func (p *Pane) moveStepBy(delta int) {
	if p.readonly {
		p.err = p.readonlyReason
		return
	}
	index := -1
	switch p.mode {
	case viewBrowse:
		rows := p.rows()
		if sel := p.activeCursor().Selected(); sel >= 0 && sel < len(rows) && rows[sel].kind == rowStep {
			index = rows[sel].step
		}
	case viewDetail:
		index = p.detailStep
	default:
		return
	}
	if index < 0 || index >= len(p.draft.Steps) {
		p.err = "alt+↑↓ moves a step — select a step row first"
		return
	}
	if p.hasLegacyStep() {
		p.err = "legacy id-less steps block adding, deleting, or reordering steps"
		return
	}
	to := index + delta
	if to < 0 || to >= len(p.draft.Steps) {
		p.err = "step is already at that edge"
		return
	}
	p.draft.Steps[index], p.draft.Steps[to] = p.draft.Steps[to], p.draft.Steps[index]
	if p.mode == viewDetail {
		p.detailStep = to
	} else {
		p.selectStepRow(to)
	}
	p.changed()
}

// selectStepRow parks the selection on one step's row in the current list.
func (p *Pane) selectStepRow(step int) {
	p.syncRows()
	for i, row := range p.rows() {
		if row.kind == rowStep && row.step == step {
			p.activeCursor().Select(i)
			return
		}
	}
}

func (p *Pane) requestDeleteSelected() {
	if p.readonly {
		p.err = p.readonlyReason
		return
	}
	rows := p.rows()
	sel := p.activeCursor().Selected()
	if sel < 0 || sel >= len(rows) {
		return
	}
	row := rows[sel]
	switch {
	case row.kind == rowField && row.ref.kind == fieldCriterion:
		if len(p.draft.SuccessCriteria) == 1 {
			p.err = "at least one success criterion is required"
			return
		}
		idx := row.ref.idx
		p.confirm.Arm(
			fmt.Sprintf("Delete success criterion %d, %q?",
				idx+1, previewValue(p.draft.SuccessCriteria[idx].Value)),
			func() {
				p.draft.SuccessCriteria = slices.Delete(p.draft.SuccessCriteria, idx, idx+1)
				p.changed()
				p.syncRows()
			},
		)
	case row.kind == rowField && row.ref.kind == fieldConstraint:
		idx := row.ref.idx
		p.confirm.Arm(
			fmt.Sprintf("Delete constraint %d, %q?",
				idx+1, previewValue(p.draft.Constraints[idx].Value)),
			func() {
				p.draft.Constraints = slices.Delete(p.draft.Constraints, idx, idx+1)
				p.changed()
				p.syncRows()
			},
		)
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
	// The question names its target: from the step details, Del works on the
	// whole step whichever row is selected, and the id is what says so.
	label := fmt.Sprintf("Delete pending step %d?", index+1)
	if step.ID != "" {
		label = fmt.Sprintf("Delete pending step %q?", step.ID)
	}
	p.confirm.Arm(label, func() {
		p.draft.Steps = slices.Delete(p.draft.Steps, index, index+1)
		p.mode, p.detailStep = viewBrowse, -1
		p.changed()
		p.syncRows()
	})
}

func (p *Pane) hasLegacyStep() bool {
	return slices.ContainsFunc(p.draft.Steps, func(step DraftStep) bool { return step.ID == "" && !step.isNew })
}

func (p *Pane) openText(ref fieldRef) {
	value := p.fieldValue(ref)
	p.textRef = ref
	p.textField = &input.TextField{
		Value: value, Cursor: len(value), MaxLines: editorMaxLines, Style: p.theme.Foreground,
		PlaceholderStyle: p.theme.Muted, Placeholder: "Enter " + ref.label(),
	}
	p.textField.OnSubmit = func(string) { p.commitText() }
	p.err = ""
}

// commitText validates the editor's value and writes it into the draft. A
// value that does not pass leaves the editor open with the reason on the
// error line.
func (p *Pane) commitText() {
	if p.textField == nil {
		return
	}
	value := strings.TrimSpace(p.textField.Value)
	ref := p.textRef
	adding := (ref.kind == fieldCriterion && ref.idx == len(p.draft.SuccessCriteria)) ||
		(ref.kind == fieldConstraint && ref.idx == len(p.draft.Constraints))
	previous := p.fieldValue(ref)
	if err := validateText(ref.label(), value, ref.limit(), ref.required()); err != nil {
		p.err = err.Error()
		return
	}
	if ref.kind == fieldID && !stepIDPattern.MatchString(value) {
		p.err = "planedit: step id must be a lowercase slug using letters, digits, '.', '_' or '-'"
		return
	}
	if err := p.setField(ref, value); err != nil {
		p.err = err.Error()
		return
	}
	p.textField = nil
	if adding || previous != value {
		p.changed()
	} else {
		p.err = ""
	}
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

// maxUndoDepth bounds the history; past it the oldest edit is forgotten.
const maxUndoDepth = 100

// changed is the single choke point every mutation path calls after editing
// the draft. It records the pre-mutation state for undo, drops the redo
// branch, and recomputes dirtiness against the baseline — so a mutation that
// turns out to be a no-op records nothing.
func (p *Pane) changed() {
	p.err = ""
	if !reflect.DeepEqual(p.draft, p.mark) {
		p.undo = append(p.undo, p.mark)
		if len(p.undo) > maxUndoDepth {
			p.undo = slices.Delete(p.undo, 0, len(p.undo)-maxUndoDepth)
		}
		p.redo = nil
		p.mark = cloneDraft(p.draft)
	}
	p.dirty = p.dirtyCount() > 0
}

// cloneDraft is a deep copy: history entries and the baseline must not share
// slices or maps with the live draft, which every mutation edits in place.
func cloneDraft(d Draft) Draft {
	d.SuccessCriteria = slices.Clone(d.SuccessCriteria)
	d.Constraints = slices.Clone(d.Constraints)
	d.ModelsByType = maps.Clone(d.ModelsByType)
	d.Steps = slices.Clone(d.Steps)
	for i := range d.Steps {
		d.Steps[i].Actions = cloneActions(d.Steps[i].Actions)
	}
	return d
}

func cloneActions(actions []session.PlanAction) []session.PlanAction {
	out := slices.Clone(actions)
	for i := range out {
		out[i].Skills = slices.Clone(out[i].Skills)
		out[i].DisabledSkills = slices.Clone(out[i].DisabledSkills)
		out[i].Runs = slices.Clone(out[i].Runs)
	}
	return out
}

// undoEdit steps the draft back one recorded change. One entry is one logical
// edit — a saved field, a toggled flag, a reorder — not one keystroke.
func (p *Pane) undoEdit() {
	if p.inChoice() {
		p.err = "finish the choice first — Esc goes back without choosing"
		return
	}
	if len(p.undo) == 0 {
		p.err = "nothing to undo"
		return
	}
	p.redo = append(p.redo, p.mark)
	last := len(p.undo) - 1
	p.draft, p.undo = p.undo[last], p.undo[:last]
	p.mark = cloneDraft(p.draft)
	p.afterHistoryJump()
}

func (p *Pane) redoEdit() {
	if p.inChoice() {
		p.err = "finish the choice first — Esc goes back without choosing"
		return
	}
	if len(p.redo) == 0 {
		p.err = "nothing to redo"
		return
	}
	p.undo = append(p.undo, p.mark)
	last := len(p.redo) - 1
	p.draft, p.redo = p.redo[last], p.redo[:last]
	p.mark = cloneDraft(p.draft)
	p.afterHistoryJump()
}

// afterHistoryJump lands the pane on a restored draft: dirtiness is
// recomputed, an armed confirmation went stale with its target, and a detail
// screen whose step the jump removed falls back to the step list.
func (p *Pane) afterHistoryJump() {
	p.err = ""
	p.dirty = p.dirtyCount() > 0
	p.confirm.Disarm()
	if p.mode == viewDetail && (p.detailStep < 0 || p.detailStep >= len(p.draft.Steps)) {
		p.mode, p.detailStep = viewBrowse, -1
		p.restoreSelection()
	}
	p.syncRows()
}

// dirtyCount is the number of unsaved edits, counted the way the ● markers
// mark rows: a changed plan field, a changed, added or removed directive, a
// changed, added, removed or moved step, a changed model pin.
func (p *Pane) dirtyCount() int {
	count := 0
	if p.draft.Goal != p.baseline.Goal {
		count++
	}
	if p.draft.Approach != p.baseline.Approach {
		count++
	}
	if p.draft.WorkingContext != p.baseline.WorkingContext {
		count++
	}
	count += directiveEdits(p.draft.SuccessCriteria, p.baseline.SuccessCriteria)
	count += directiveEdits(p.draft.Constraints, p.baseline.Constraints)
	count += p.stepEdits()
	count += modelEdits(p.draft.ModelsByType, p.baseline.ModelsByType)
	return count
}

// directiveEdits counts changed and added entries plus the removed ones —
// every surviving entry still knows the durable value it descends from, so
// deleting one row never smears dirt over the rows that shifted up.
func directiveEdits(draft, base []directiveDraft) int {
	count, survivors := 0, 0
	for _, entry := range draft {
		if entry.New {
			count++
			continue
		}
		survivors++
		if entry.Value != entry.Original {
			count++
		}
	}
	return count + max(len(base)-survivors, 0)
}

func (p *Pane) stepEdits() int {
	count, survivors := 0, 0
	for i, step := range p.draft.Steps {
		if !step.isNew {
			survivors++
		}
		if p.stepDirty(i) {
			count++
		}
	}
	return count + max(len(p.baseline.Steps)-survivors, 0)
}

// stepDirty reports whether the step list should mark step i: it is new, its
// fields differ from the durable step it descends from, or it left its place
// in the surviving order.
func (p *Pane) stepDirty(i int) bool {
	if i < 0 || i >= len(p.draft.Steps) {
		return false
	}
	step := p.draft.Steps[i]
	if step.isNew {
		return true
	}
	if step.baseIndex < 0 || step.baseIndex >= len(p.baseline.Steps) {
		return true
	}
	return !reflect.DeepEqual(step, p.baseline.Steps[step.baseIndex]) || p.stepMoved(i)
}

// stepMoved compares step i's place among the surviving durable steps with
// its durable order, so a pure reorder marks exactly the steps that moved
// while a deletion above marks nothing below it.
func (p *Pane) stepMoved(i int) bool {
	step := p.draft.Steps[i]
	before, smaller := 0, 0
	for j, other := range p.draft.Steps {
		if other.isNew || j == i {
			continue
		}
		if j < i {
			before++
		}
		if other.baseIndex < step.baseIndex {
			smaller++
		}
	}
	return before != smaller
}

func modelEdits(draft, base map[session.StepType]string) int {
	count := 0
	for typ := range draft {
		if draft[typ] != base[typ] {
			count++
		}
	}
	for typ := range base {
		if _, ok := draft[typ]; !ok && base[typ] != "" {
			count++
		}
	}
	return count
}

// apply compiles the draft against the revision it was drawn from and commits
// it. The plan moving under an open modal is a race, not a dead end: a refused
// compare-and-set rebases the draft onto the newer revision and retries once
// when nothing collided, and otherwise leaves the modal open on the new base
// with the losses named.
//
// The committed plan is not handed back to the shell: the same Store write that
// commits it publishes PlanUpdatedMsg, and the plan view is refreshed from that
// message on the next frame. A callback here would only repeat it.
func (p *Pane) apply() {
	if p.store == nil {
		p.err = "planedit: plan store unavailable"
		return
	}
	if p.readonly {
		p.err = p.readonlyReason
		return
	}
	for range 2 {
		ops, err := p.draft.ops(p.base, p.types)
		if err != nil {
			p.err = err.Error()
			return
		}
		if len(ops) == 0 {
			p.Hide()
			return
		}
		_, err = p.store.Apply(context.Background(), p.base.Revision, ops)
		if err == nil {
			p.Hide()
			return
		}
		if _, stale := errors.AsType[*session.StalePlanRevisionError](err); !stale {
			p.err = err.Error()
			return
		}
		if conflicts := p.rebase(); len(conflicts) > 0 {
			p.err = conflictMessage(p.base.Revision, conflicts)
			return
		}
	}
	// Two clean rebases in a row still lost the race: the plan is moving
	// faster than the editor can follow. The draft sits on the newest base, so
	// the next keypress is a fresh attempt, not a rewrite.
	p.err = fmt.Sprintf("plan is still moving (now rev %d); press ctrl+s again", p.base.Revision)
}

// rebase re-reads the plan and moves the draft onto it, keeping the modal
// usable: selection and detail focus are clamped to what survived.
func (p *Pane) rebase() []string {
	fresh := p.store.Snapshot()
	draft, conflicts := p.draft.rebase(p.base, fresh)
	p.base, p.draft = fresh, draft
	// The undo history indexes the old base; replaying it onto the new one
	// would restore steps against indices that no longer mean the same thing.
	// Dropped, together with the baseline it was measured against.
	p.baseline = newDraft(p.base)
	p.mark = cloneDraft(p.draft)
	p.undo, p.redo = nil, nil
	p.dirty = p.dirtyCount() > 0
	if p.detailStep >= len(p.draft.Steps) {
		p.mode, p.detailStep = viewBrowse, -1
	}
	p.syncRows()
	return conflicts
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
		if name := p.stepTitle(p.detailStep); name != "" {
			title = " Step " + name + " "
		} else {
			title = " Step details "
		}
	case viewTypes:
		title = " Choose step type "
	case viewModels:
		title = " Choose model "
	case viewActionEvent:
		title = " Choose action event "
	case viewActionType:
		title = " Choose action type "
	case viewMenu:
		title = " Actions "
	}
	if p.readonly {
		title = " Plan · read-only "
	}
	hint := keys.Footer(keys.ScopePlan)
	switch p.mode {
	case viewDetail:
		hint = keys.Footer(keys.ScopePlanDetail)
	case viewTypes, viewModels, viewActionEvent, viewActionType, viewMenu:
		hint = keys.Footer(keys.ScopePlanChoice)
	}
	if p.textField != nil {
		hint = keys.Footer(keys.ScopePlanText)
	}
	if p.jump != nil {
		hint = keys.Footer(keys.ScopePlanJump)
	}
	if p.confirm.Armed() {
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
		// The count is the number of ● markers: the header totals what the
		// rows point at.
		meta += fmt.Sprintf(" · %d unsaved · material edits may require approval again", p.dirtyCount())
	}
	if pw > 4 && ph > 2 {
		panel.Print(2, 1, layout.TruncateToWidth(meta, pw-4, ctx.Method), th.Muted, ctx.Method)
	}

	bodyTop := 3
	avail := max(ph-5, 0)
	editing := p.textField != nil
	jumping := p.jump != nil
	var field components.Surface
	editorH := 0
	if editing && avail > 1 {
		field = p.textField.Draw(components.DrawContext{
			Max:    components.Size{Width: max(pw-4, 1), Height: min(editorMaxLines, avail-1)},
			Method: ctx.Method,
		})
		editorH = min(max(field.Size.Height, editorMinLines)+1, avail)
	}
	if jumping && avail > 1 {
		field = p.jump.Draw(components.DrawContext{
			Max:    components.Size{Width: max(pw-6, 1), Height: 1},
			Method: ctx.Method,
		})
		editorH = 2
	}
	view := avail - editorH
	resized := p.viewport != view
	p.viewport = view
	p.bodyTop = y0 + bodyTop
	p.twoPaneOn = pw >= twoPaneMinPanel && !p.inChoice() && view > 0
	if p.twoPaneOn {
		masterText := max(pw*2/5-4, 30)
		divider := 2 + masterText + 2
		detailX := divider + 2
		detailText := max(pw-3-detailX, 1)
		dividerStyle := xui.Style{Fg: th.Muted.Fg, Bg: th.BackgroundElement.Bg}
		for i := range view {
			if bodyTop+i >= ph-1 {
				break
			}
			panel.SetCell(divider, bodyTop+i, xui.Cell{Char: "│", Width: 1, Style: dividerStyle})
		}
		browseRows := p.browseRows()
		focused := p.mode == viewBrowse && !editing
		p.drawListAt(&panel, ctx, th, browseRows, &p.browseCur,
			2, masterText, bodyTop, view, ph, focused, resized && focused)
		p.masterHit = hitRegion{left: x0 + 2, width: masterText + 1}
		p.detailHit = hitRegion{left: x0 + detailX, width: detailText + 1}
		if p.mode == viewDetail {
			detailRows := p.detailRowsFor(p.detailStep)
			p.overflow = len(detailRows) > view
			p.drawListAt(&panel, ctx, th, detailRows, &p.detailCur,
				detailX, detailText, bodyTop, view, ph, !editing, resized)
			p.previewStep = -1
		} else {
			p.overflow = len(browseRows) > view
			p.drawPreview(&panel, ctx, th, detailX, detailText, bodyTop, view, ph)
		}
	} else {
		rows := p.rows()
		p.overflow = len(rows) > view
		p.drawListAt(&panel, ctx, th, rows, p.activeCursor(),
			2, max(pw-5, 0), bodyTop, view, ph, !editing, resized)
		p.masterHit = hitRegion{left: x0 + min(2, pw), width: max(pw-4, 0)}
		p.detailHit = hitRegion{}
		p.previewStep = -1
	}
	if editorH > 0 && editing {
		p.drawInlineEditor(&panel, field, ctx, th, bodyTop+view, pw, ph)
	}
	if editorH > 0 && jumping {
		p.drawJumpStrip(&panel, field, ctx, th, bodyTop+view, pw, ph)
	}
	message := p.err
	if p.confirm.Armed() {
		message = p.confirm.Label() + " (y/n)"
	}
	if message != "" && ph >= 3 && pw > 4 {
		panel.Print(2, ph-2, layout.TruncateToWidth(message, pw-4, ctx.Method), th.Warning, ctx.Method)
	}
	blit(&root, panel, x0, y0)
	if editorH > 0 && field.Cursor != nil {
		fieldX := 2
		if jumping {
			fieldX = 4 // After the "/ " prompt.
		}
		root.Cursor = &components.Point{
			X: x0 + fieldX + field.Cursor.X,
			Y: y0 + bodyTop + view + 1 + field.Cursor.Y,
		}
	}
	return root
}

// twoPaneMinPanel is the panel width where the editor splits into a master
// list and a detail pane; below it the panes stack into the single list.
const twoPaneMinPanel = 86

// drawListAt renders one row list in a column of the panel: its cursor is
// synced and clamped, the focused list re-follows its selection on a resize,
// and an overflowing list gets a scrollbar just right of its text.
func (p *Pane) drawListAt(
	panel *components.Surface, ctx components.DrawContext, th components.Theme,
	rows []paneRow, cur *browse.Cursor,
	x, textW, bodyTop, view, ph int, focused, refollow bool,
) {
	cur.SetRows(len(rows), func(i int) bool { return rows[i].selectable })
	cur.SetViewport(view)
	if refollow {
		// A resize re-follows the selection; ordinary repaints must not, or
		// they would undo free wheel scrolling.
		cur.Select(cur.Selected())
	}
	for i := range view {
		idx := cur.Scroll() + i
		if idx >= len(rows) || bodyTop+i >= ph-1 {
			break
		}
		state := selNone
		if idx == cur.Selected() {
			if focused {
				state = selFocused
			} else {
				state = selPassive
			}
		}
		style, marker := p.rowStyle(rows[idx], state)
		panel.Print(x, bodyTop+i, layout.TruncateToWidth(marker+rows[idx].text, textW, ctx.Method), style, ctx.Method)
	}
	if len(rows) > view {
		drawScrollbar(panel, x+textW+1, bodyTop, view, len(rows), cur.Scroll(), th.Muted)
	}
}

// drawPreview renders the right pane while the keyboard stays on the browse
// list: the selected step's details, a field's full value, or the plan
// overview — wheeled independently, reset when the selection moves.
func (p *Pane) drawPreview(
	panel *components.Surface, ctx components.DrawContext, th components.Theme,
	x, textW, bodyTop, view, ph int,
) {
	rows, step := p.previewRows(textW, ctx.Method)
	if sel := p.browseCur.Selected(); sel != p.previewSel {
		p.previewSel, p.previewScroll = sel, 0
	}
	p.previewStep = step
	p.previewScroll = min(max(p.previewScroll, 0), max(len(rows)-view, 0))
	for i := range view {
		idx := p.previewScroll + i
		if idx >= len(rows) || bodyTop+i >= ph-1 {
			break
		}
		style, marker := p.rowStyle(rows[idx], selNone)
		panel.Print(x, bodyTop+i, layout.TruncateToWidth(marker+rows[idx].text, textW, ctx.Method), style, ctx.Method)
	}
	if len(rows) > view {
		drawScrollbar(panel, x+textW+1, bodyTop, view, len(rows), p.previewScroll, th.Muted)
	}
}

// previewRows is the right pane's content for the selected browse row: step
// rows preview their details, field rows their full value, model pins their
// resolution, and anything else the plan overview. The second result is the
// previewed step, -1 when the preview is not a step.
func (p *Pane) previewRows(width int, method xui.WidthMethod) ([]paneRow, int) {
	rows := p.browseRows()
	sel := p.browseCur.Selected()
	if sel < 0 || sel >= len(rows) {
		return p.overviewRows(width, method), -1
	}
	row := rows[sel]
	switch row.kind {
	case rowStep:
		return p.detailRowsFor(row.step), row.step
	case rowField:
		return p.fieldPreviewRows(row.ref, width, method), -1
	case rowModelType:
		return p.modelPreviewRows(row.step, width, method), -1
	default:
		return p.overviewRows(width, method), -1
	}
}

// fieldPreviewRows shows a field in full — the list compacts long values,
// the preview wraps them.
func (p *Pane) fieldPreviewRows(ref fieldRef, width int, method xui.WidthMethod) []paneRow {
	value := p.fieldValue(ref)
	head := fmt.Sprintf("%s · %d/%d", titleize(ref.label()), utf8.RuneCountInString(value), ref.limit())
	rows := []paneRow{{text: head, kind: rowHeading}, {text: "", kind: rowText}}
	if value == "" {
		return append(rows, paneRow{text: "(none)", kind: rowInfo})
	}
	return append(rows, wrapRows(value, width, method, rowText)...)
}

func (p *Pane) modelPreviewRows(idx, width int, method xui.WidthMethod) []paneRow {
	if idx < 0 || idx >= len(p.types) {
		return []paneRow{{text: "Model pin", kind: rowHeading}}
	}
	typ := p.types[idx]
	value := "(type default)"
	if pin := p.draft.ModelsByType[typ]; pin != "" {
		value = pin
	}
	rows := []paneRow{
		{text: "Model pin · " + string(typ), kind: rowHeading},
		{text: "", kind: rowText},
		{text: value, kind: rowText},
		{text: "", kind: rowText},
	}
	hint := "Enter opens the model list; a pin overrides the type default for steps of this type."
	return append(rows, wrapRows(hint, width, method, rowInfo)...)
}

// overviewRows is the preview fallback: what the plan is about and where it
// stands, for rows with nothing of their own to expand.
func (p *Pane) overviewRows(width int, method xui.WidthMethod) []paneRow {
	rows := []paneRow{{text: "Overview", kind: rowHeading}, {text: "", kind: rowText}}
	rows = append(rows, wrapRows(compactValue(p.draft.Goal), width, method, rowText)...)
	done := 0
	for _, step := range p.draft.Steps {
		if step.Status == session.PlanCompleted {
			done++
		}
	}
	return append(rows,
		paneRow{text: "", kind: rowText},
		paneRow{text: fmt.Sprintf("%d steps · %d done", len(p.draft.Steps), done), kind: rowInfo},
	)
}

// wrapRows soft-wraps one value into preview rows of the given kind; the
// two marker columns every row carries are already paid for here.
func wrapRows(value string, width int, method xui.WidthMethod, kind rowKind) []paneRow {
	lines := components.WrapSpans([]components.Span{{Text: value}}, max(width-2, 1), method)
	rows := make([]paneRow, 0, len(lines))
	for _, line := range lines {
		var text strings.Builder
		for _, span := range line {
			text.WriteString(span.Text)
		}
		rows = append(rows, paneRow{text: text.String(), kind: kind})
	}
	return rows
}

func titleize(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// The inline editor grows with its content between these visible line
// counts; the labeled rule row sits on top of them.
const (
	editorMinLines = 4
	editorMaxLines = 6
)

// drawInlineEditor renders the bottom editing strip that replaced the
// centered popup: a rule row naming the field being edited — a step id
// first when the field belongs to a step, so an edit never reads like it
// edits the whole plan — then the text itself. The list stays visible
// above it, with a passive cursor on the row the editor came from.
func (p *Pane) drawInlineEditor(
	panel *components.Surface, field components.Surface,
	ctx components.DrawContext, th components.Theme, top, pw, ph int,
) {
	if top < 1 || top >= ph-1 || pw < 2 {
		return
	}
	rule := xui.Style{Fg: th.Muted.Fg, Bg: th.BackgroundElement.Bg}
	panel.SetCell(0, top, xui.Cell{Char: "├", Width: 1, Style: rule})
	for x := 1; x < pw-1; x++ {
		panel.SetCell(x, top, xui.Cell{Char: "─", Width: 1, Style: rule})
	}
	panel.SetCell(pw-1, top, xui.Cell{Char: "┤", Width: 1, Style: rule})
	owner := ""
	if name := p.stepName(p.textRef.step); name != "" {
		owner = name + " · "
	}
	label := fmt.Sprintf(
		" Edit %s%s · %d/%d ",
		owner,
		p.textRef.label(),
		utf8.RuneCountInString(p.textField.Value),
		p.textRef.limit(),
	)
	if pw > 4 {
		panel.Print(2, top, layout.TruncateToWidth(label, pw-4, ctx.Method), th.Foreground, ctx.Method)
	}
	blit(panel, field, 2, top+1)
}

// drawJumpStrip renders the `/` fuzzy-jump strip: a rule row counting the
// matches, then the query behind a "/" prompt. The list above keeps the
// keyboard's selection — the strip only steers it.
func (p *Pane) drawJumpStrip(
	panel *components.Surface, field components.Surface,
	ctx components.DrawContext, th components.Theme, top, pw, ph int,
) {
	if top < 1 || top >= ph-1 || pw < 2 {
		return
	}
	rule := xui.Style{Fg: th.Muted.Fg, Bg: th.BackgroundElement.Bg}
	panel.SetCell(0, top, xui.Cell{Char: "├", Width: 1, Style: rule})
	for x := 1; x < pw-1; x++ {
		panel.SetCell(x, top, xui.Cell{Char: "─", Width: 1, Style: rule})
	}
	panel.SetCell(pw-1, top, xui.Cell{Char: "┤", Width: 1, Style: rule})
	label, style := " Jump ", th.Foreground
	switch {
	case strings.TrimSpace(p.jump.Value) == "":
	case len(p.jumpMatches) == 0:
		label, style = " Jump · no match ", th.Warning
	case len(p.jumpMatches) == 1:
		label = " Jump · 1 match "
	default:
		label = fmt.Sprintf(" Jump · match %d/%d ", p.jumpPos+1, len(p.jumpMatches))
	}
	if pw > 4 {
		panel.Print(2, top, layout.TruncateToWidth(label, pw-4, ctx.Method), style, ctx.Method)
		panel.Print(2, top+1, "/", th.Muted, ctx.Method)
	}
	blit(panel, field, 4, top+1)
}

// rowSelState is how a drawn row relates to its list's cursor: unselected,
// selected in the focused pane, or selected in the pane the keyboard left.
type rowSelState uint8

const (
	selNone rowSelState = iota
	selFocused
	selPassive
)

func (p *Pane) rowStyle(row paneRow, state rowSelState) (xui.Style, string) {
	if row.selectable && state == selFocused {
		return xui.Style{Reverse: true}, "› "
	}
	if row.selectable && state == selPassive {
		return p.theme.ToolName, "› "
	}
	switch row.kind {
	case rowHeading:
		return xui.Style{Bold: true, Fg: p.theme.Foreground.Fg}, "  "
	case rowInfo:
		return p.theme.Muted, "  "
	case rowAddCriterion,
		rowAddConstraint,
		rowAddStep,
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
		rowEventChoice,
		rowActionKindChoice,
		rowActionBack,
		rowMenuItem:
		return p.theme.ToolName, "  "
	default:
		return p.theme.Foreground, "  "
	}
}

func (p *Pane) rows() []paneRow {
	switch p.mode {
	case viewDetail:
		return p.detailRowsFor(p.detailStep)
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
	case viewActionEvent:
		rows := make([]paneRow, 0, len(actionEventOptions()))
		for i, event := range actionEventOptions() {
			rows = append(rows, paneRow{text: "  " + string(event), kind: rowEventChoice, step: i, selectable: true})
		}
		return rows
	case viewMenu:
		rows := make([]paneRow, 0, len(p.menu))
		for i, item := range p.menu {
			rows = append(rows, paneRow{text: "  " + item.label, kind: rowMenuItem, step: i, selectable: true})
		}
		return rows
	case viewActionType:
		rows := make([]paneRow, 0, len(actionTypeOptions()))
		for i, typ := range actionTypeOptions() {
			rows = append(rows, paneRow{text: "  " + string(typ), kind: rowActionKindChoice, step: i, selectable: true})
		}
		return rows
	default:
		return p.browseRows()
	}
}

// dirtyPrefix replaces a row's two-space indent with the unsaved-edit dot,
// keeping the columns aligned.
func dirtyPrefix(dirty bool) string {
	if dirty {
		return "● "
	}
	return "  "
}

func directiveDirty(entry directiveDraft) bool {
	return entry.New || entry.Value != entry.Original
}

func (p *Pane) browseRows() []paneRow {
	rows := []paneRow{{text: "Plan", kind: rowHeading}}
	addField := func(label string, ref fieldRef, value string, dirty bool) {
		rows = append(rows, paneRow{
			text: dirtyPrefix(dirty) + label + ": " + compactValue(value),
			kind: rowField, ref: ref, selectable: true,
		})
	}
	addField("Goal", fieldRef{kind: fieldGoal, step: -1}, p.draft.Goal, p.draft.Goal != p.baseline.Goal)
	addField(
		"Approach", fieldRef{kind: fieldApproach, step: -1},
		p.draft.Approach, p.draft.Approach != p.baseline.Approach,
	)
	addField(
		"Context", fieldRef{kind: fieldContext, step: -1},
		p.draft.WorkingContext, p.draft.WorkingContext != p.baseline.WorkingContext,
	)
	rows = append(rows, paneRow{text: "Success criteria", kind: rowHeading})
	for i, entry := range p.draft.SuccessCriteria {
		ref := fieldRef{kind: fieldCriterion, idx: i, step: -1}
		addField(strconv.Itoa(i+1), ref, entry.Value, directiveDirty(entry))
	}
	rows = append(rows,
		paneRow{text: "  + Add success criterion", kind: rowAddCriterion, selectable: true},
		paneRow{text: "Constraints", kind: rowHeading},
	)
	for i, entry := range p.draft.Constraints {
		ref := fieldRef{kind: fieldConstraint, idx: i, step: -1}
		addField(strconv.Itoa(i+1), ref, entry.Value, directiveDirty(entry))
	}
	rows = append(rows,
		paneRow{text: "  + Add constraint", kind: rowAddConstraint, selectable: true},
		paneRow{text: "Steps", kind: rowHeading},
	)
	for i, step := range p.draft.Steps {
		// The id rides the row so the list, the detail title and the field editor
		// all name steps the same way.
		name := step.ID
		if name == "" && step.isNew {
			name = "(new)"
		}
		label := fmt.Sprintf(
			"%s%d %s %s %s — %s",
			dirtyPrefix(p.stepDirty(i)),
			i+1,
			statusIcon(step.Status),
			stepTypeLabel(step.Type),
			name,
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
	// The settings section: one pin per configured step type. Clearing a
	// pin hands that type back to the session default.
	rows = append(rows,
		paneRow{text: "  + Add step", kind: rowAddStep, selectable: true},
		paneRow{text: "Step models", kind: rowHeading},
	)
	if len(p.types) == 0 {
		rows = append(rows, paneRow{text: "  (no step types configured)", kind: rowInfo})
	} else {
		for i, typ := range p.types {
			label := "(type default)"
			if name := p.draft.ModelsByType[typ]; name != "" {
				label = name
			}
			pinDirty := p.draft.ModelsByType[typ] != p.baseline.ModelsByType[typ]
			rows = append(rows, paneRow{
				text: dirtyPrefix(pinDirty) + string(typ) + ": " + label,
				kind: rowModelType, step: i, selectable: true,
			})
		}
	}
	return rows
}

// stepTitle names the step a screen is about — position and id — so two
// steps' editors are never mistaken for each other. Legacy id-less steps
// fall back to a content preview; unnamed new steps keep their position.
func (p *Pane) stepTitle(index int) string {
	if index < 0 || index >= len(p.draft.Steps) {
		return ""
	}
	step := p.draft.Steps[index]
	name := step.ID
	if name == "" && !step.isNew {
		name = compactValue(step.Content)
	}
	return fmt.Sprintf("%d/%d %s", index+1, len(p.draft.Steps), name)
}

// stepName is the popup-level identity: the step id alone, or a stable
// positional fallback when the id is still blank.
func (p *Pane) stepName(index int) string {
	if index < 0 || index >= len(p.draft.Steps) {
		return ""
	}
	if name := p.draft.Steps[index].ID; name != "" {
		return name
	}
	return fmt.Sprintf("step %d", index+1)
}

// baselineStep is the durable counterpart the detail rows compare against; a
// new or unmatched step compares against the zero step, so its filled fields
// read as edits.
func (p *Pane) baselineStep(i int) DraftStep {
	if i < 0 || i >= len(p.draft.Steps) {
		return DraftStep{}
	}
	step := p.draft.Steps[i]
	if step.isNew || step.baseIndex < 0 || step.baseIndex >= len(p.baseline.Steps) {
		return DraftStep{}
	}
	return p.baseline.Steps[step.baseIndex]
}

// detailRowsFor builds the step-detail rows for one step: the focused detail
// screen and the two-pane preview render the same truth.
func (p *Pane) detailRowsFor(index int) []paneRow {
	if index < 0 || index >= len(p.draft.Steps) {
		return []paneRow{{text: "Step is no longer available", kind: rowInfo}}
	}
	step := p.draft.Steps[index]
	base := p.baselineStep(index)
	rows := []paneRow{
		{text: "Step " + p.stepTitle(index) + " · identity (read-only after creation)", kind: rowHeading},
	}
	if step.isNew {
		rows = append(rows,
			paneRow{
				text: dirtyPrefix(step.ID != "") + "ID: " + compactValue(step.ID), kind: rowField,
				ref: fieldRef{kind: fieldID, step: index}, selectable: true,
			},
			paneRow{
				text: "  Type: " + stepTypeLabel(step.Type) + " — choose…",
				kind: rowTypeChoice, step: index, selectable: true,
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
			base  string
		}{
			{"Content", fieldContent, step.Content, base.Content},
			{"Why", fieldWhy, step.Why, base.Why},
			{"Done when", fieldDoneWhen, step.DoneWhen, base.DoneWhen},
			{"Note", fieldNote, step.Note, base.Note},
			{"Risk", fieldRisk, step.Risk, base.Risk},
		} {
			text := dirtyPrefix(spec.value != spec.base) + spec.label + ": " + compactValue(spec.value)
			rows = append(rows, paneRow{
				text: text, kind: rowField,
				ref: fieldRef{kind: spec.kind, step: index}, selectable: true,
			})
		}
	}
	if step.isNew {
		rows = append(rows, paneRow{
			text: dirtyPrefix(step.JIT != base.JIT) + "Toggle just-in-time posture (currently " + jitPosture + ")",
			kind: rowActionToggleJIT, step: index, selectable: true,
		})
	}
	rows = append(rows, paneRow{text: "Actions", kind: rowHeading})
	if step.ID != "" || step.isNew {
		// Reordering is a list operation and lives on Shift+↑↓ — here and on
		// the step list — where the move is visible; no row repeats it.
		rows = append(rows,
			paneRow{text: "  Delete pending step…", kind: rowActionDelete, step: index, selectable: true},
		)
	}
	if step.ID != "" || step.isNew {
		// Automation lives with the step: actions first, then the model pin —
		// both compile into one update_step patch.
		rows = append(rows, paneRow{text: "Automation", kind: rowHeading})
		for i, action := range step.Actions {
			ref := fieldRef{kind: fieldSkills, step: index, idx: i}
			// An added action marks every row it owns; on a surviving one each
			// row marks only the aspect that changed.
			added := i >= len(base.Actions)
			var baseAction session.PlanAction
			if !added {
				baseAction = base.Actions[i]
			}
			rows = append(rows,
				paneRow{
					text: fmt.Sprintf("%s⚙ %d event: %s — choose…",
						dirtyPrefix(added || action.Event != baseAction.Event), i+1, action.Event),
					kind: rowActionEvent, ref: ref, step: index, selectable: true,
				},
				paneRow{
					text: fmt.Sprintf("%s⚙ %d type: %s — choose…",
						dirtyPrefix(added || action.Type != baseAction.Type), i+1, action.Type),
					kind: rowActionType, ref: ref, step: index, selectable: true,
				},
			)
			if action.Type == session.PlanActionInjectSkill {
				skills := skillListSummary(action)
				skillsDirty := added ||
					!slices.Equal(action.Skills, baseAction.Skills) ||
					!slices.Equal(action.DisabledSkills, baseAction.DisabledSkills)
				rows = append(rows, paneRow{
					text: fmt.Sprintf("%s⚙ %d inject_skill · skills: %s", dirtyPrefix(skillsDirty), i+1, skills),
					kind: rowActionSkills, ref: ref, step: index, selectable: true,
				})
			}
			rows = append(rows, paneRow{
				text: fmt.Sprintf("  ⚙ %d %s · remove", i+1, action.Type),
				kind: rowActionRemove, ref: ref, step: index, selectable: true,
			})
		}
		rows = append(rows, paneRow{text: "  + Add action", kind: rowAddAction, step: index, selectable: true})
		model := step.Model
		if model == "" {
			model = "(type default)"
		}
		rows = append(rows, paneRow{
			text: dirtyPrefix(step.Model != base.Model) + "Model: " + model,
			kind: rowStepModel, step: index, selectable: true,
		})
	}
	rows = append(rows, paneRow{text: "  ← Back to plan", kind: rowActionBack, selectable: true})
	return rows
}

// actionEventOptions lists the moments a step action can fire on, in list
// order; the plan editor's choice screen renders exactly these.
func actionEventOptions() []session.PlanActionEvent {
	return []session.PlanActionEvent{session.PlanActionOnStepStart, session.PlanActionOnStepEnd}
}

func actionEventIndex(event session.PlanActionEvent) int {
	for i, candidate := range actionEventOptions() {
		if candidate == event {
			return i
		}
	}
	return 0
}

// actionTypeOptions lists the built-in commands an action can run.
func actionTypeOptions() []session.PlanActionType {
	return []session.PlanActionType{session.PlanActionCompact, session.PlanActionInjectSkill}
}

func actionTypeIndex(typ session.PlanActionType) int {
	for i, candidate := range actionTypeOptions() {
		if candidate == typ {
			return i
		}
	}
	return 0
}

func compactValue(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "(none)"
	}
	return value
}

// previewValue shortens a value for a confirmation label: the question names
// what it deletes without pushing the y/n hint off the message row.
func previewValue(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	const limit = 32
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit-1]) + "…"
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
	case session.PlanSuperseded:
		return "↪"
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
