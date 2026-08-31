// Package settings renders the global harness settings modal. Pane owns only
// an editable draft and interaction state; validation, merge, persistence, and
// live publication remain behind Store.
package settings

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/browse"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

// Tab identifies one top-level settings section.
type Tab uint8

const (
	TabPlanDefaults Tab = iota
	TabGeneral
	TabAgents
	tabCount
)

// Store is the complete persistence seam used by the modal.
type Store interface {
	Snapshot() harnesssettings.Snapshot
	Apply(context.Context, harnesssettings.Draft) (harnesssettings.Snapshot, error)
}

// agentRoles fixes the Agents tab row order; an Agents row's typeIndex
// addresses it (-1 names the bulk "all roles" picker). job.Roles owns the
// canonical order.
var agentRoles = job.Roles()

// State is a detached behavioral snapshot for shell integration and tests.
type State struct {
	Tab      Tab
	Scroll   int
	Selected int
	Overflow bool
	Dirty    bool
	Error    string
}

type region struct {
	x0, x1 int
	y      int
	tab    Tab
}

type rowKind uint8

const (
	rowLabel rowKind = iota
	rowAddType
	rowReset
	rowRenameType
	rowDeleteType
	rowMoveTypeUp
	rowMoveTypeDown
	rowTypeModel
	rowModelOption
	rowPermission
	rowOutsidePlan
	rowLocked
	rowCompactThreshold
	rowAgentBulkModel
	rowAgentModel
	rowAgentModelOption

	// Default plan actions: one add row per scope, then per action a remove
	// header plus one row per editable field. Plan-scope rows address
	// draft.Plan.Actions, type-scope rows draft.Plan.Types[typeIndex].Actions.
	rowPlanActionAdd
	rowPlanActionRemove
	rowPlanActionEvent
	rowPlanActionType
	rowPlanActionSkills
	rowTypeActionAdd
	rowTypeActionRemove
	rowTypeActionEvent
	rowTypeActionType
	rowTypeActionSkills
	rowSkillOption
	rowAuthoringPolicy
)

type paneRow struct {
	text      string
	kind      rowKind
	typeIndex int
	tool      string
	// modelIndex addresses one entry of a type's inline model list; -1 is
	// the "(session default)" clear entry.
	modelIndex int
	// actionIndex addresses one action of the plan-level list or of the
	// row's type, when kind is an action kind.
	actionIndex int
	// skillIndex addresses one entry of a skills picker's known list.
	skillIndex int
}

type nameMode uint8

const (
	nameNone nameMode = iota
	nameAdd
	nameRename
	nameThreshold
)

// Pane is a full-screen modal containing a centered settings panel. It is
// mutated and rendered only by the UI goroutine.
type Pane struct {
	theme components.Theme
	store Store

	onClose   func()
	onApplied func(harnesssettings.Snapshot)
	visible   bool
	tab       Tab
	draft     harnesssettings.Draft
	dirty     bool
	errText   string

	configPath string
	typeInUse  func(session.StepType) bool
	skills     []string

	availabilitySet bool
	availableTools  map[string]struct{}

	nameMode      nameMode
	nameTypeIndex int
	nameDraft     string
	// skillsTypeOpen/skillsActionOpen address the action whose skill list is
	// expanded into the picker (-1 = none). typeIndex -1 addresses the
	// plan-scope action, otherwise the type's.
	skillsTypeOpen   int
	skillsActionOpen int

	// modelNames feeds the per-type model picker; modelTypeOpen is the type
	// whose inline list is expanded (-1 = none).
	modelNames    []string
	modelTypeOpen int

	// agentModelOpen is the role whose picker is expanded (-1 = none);
	// agentBulkOpen is the bulk "all roles" picker.
	agentModelOpen int
	agentBulkOpen  bool

	// One cursor per tab, so switching tabs keeps each list's place; the
	// motion parser is shared — pending input dies with the tab switch.
	cursors  [tabCount]browse.Cursor
	motions  browse.Motions
	viewport [tabCount]int
	overflow [tabCount]bool

	tabRegions []region
	bodyTop    int
	bodyLeft   int
	bodyWidth  int
}

// New returns a hidden modal.
func New(theme components.Theme, store Store, onClose func()) *Pane {
	return &Pane{
		theme: theme, store: store, onClose: onClose,
		modelTypeOpen: -1, nameTypeIndex: -1, skillsTypeOpen: -1, skillsActionOpen: -1,
		agentModelOpen: -1,
	}
}

// SetSkills supplies the skill names an inject_skill action can name; the
// plan tab lists them so authoring does not require guessing names.
func (p *Pane) SetSkills(names []string) {
	if p != nil {
		p.skills = append([]string(nil), names...)
	}
}

// SetModelNames supplies the configured model names a type's model picker
// offers. The editor refreshes it alongside every other model surface.
func (p *Pane) SetModelNames(names []string) {
	if p != nil {
		p.modelNames = append([]string(nil), names...)
	}
}

// SetTheme updates modal chrome styling.
func (p *Pane) SetTheme(theme components.Theme) {
	if p != nil {
		p.theme = theme
	}
}

// SetOnApplied receives the committed snapshot before the modal closes.
func (p *Pane) SetOnApplied(onApplied func(harnesssettings.Snapshot)) {
	if p != nil {
		p.onApplied = onApplied
	}
}

// SetTypeInUse supplies the current-plan usage check used before deleting a
// type. The callback is queried on the UI goroutine.
func (p *Pane) SetTypeInUse(inUse func(session.StepType) bool) {
	if p != nil {
		p.typeInUse = inUse
	}
}

// SetAvailableTools records the tool names present in the current runtime.
// Known but absent tools remain editable and are rendered as unavailable.
func (p *Pane) SetAvailableTools(names []string) {
	if p == nil {
		return
	}
	p.availabilitySet = true
	p.availableTools = make(map[string]struct{}, len(names))
	for _, name := range names {
		p.availableTools[name] = struct{}{}
	}
}

// Show opens a fresh draft of the latest committed settings.
func (p *Pane) Show() {
	if p == nil {
		return
	}
	p.visible = true
	p.tab = TabPlanDefaults
	p.cursors = [tabCount]browse.Cursor{}
	p.motions.Reset()
	p.viewport = [tabCount]int{}
	p.overflow = [tabCount]bool{}
	p.dirty = false
	p.errText = ""
	p.configPath = "(config unavailable)"
	p.cancelNameEntry()
	// A picker left open against a stale draft would resurrect on reopen.
	p.modelTypeOpen, p.skillsTypeOpen, p.skillsActionOpen = -1, -1, -1
	p.agentModelOpen, p.agentBulkOpen = -1, false
	if p.store != nil {
		snapshot := p.store.Snapshot()
		p.draft = snapshot.Draft()
		p.configPath = snapshot.Path
	} else {
		p.draft = harnesssettings.Draft{}
	}
}

// Hide discards the in-memory draft and closes the modal.
func (p *Pane) Hide() {
	if p == nil || !p.visible {
		return
	}
	p.visible = false
	p.draft = harnesssettings.Draft{}
	p.errText = ""
	p.cancelNameEntry()
	if p.onClose != nil {
		p.onClose()
	}
}

// Visible reports whether the modal owns the screen and input.
func (p *Pane) Visible() bool { return p != nil && p.visible }

// State returns interaction state for the active tab.
func (p *Pane) State() State {
	if p == nil {
		return State{}
	}
	return State{
		Tab:      p.tab,
		Scroll:   p.cursors[p.tab].Scroll(),
		Selected: p.cursors[p.tab].Selected(),
		Overflow: p.overflow[p.tab],
		Dirty:    p.dirty,
		Error:    p.errText,
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
	if p.nameMode != nameNone {
		p.handleNameKey(event)
		return
	}
	if event.Code == xui.KeyRune && event.Mods == xui.ModCtrl && event.HotkeyRune() == 's' {
		p.apply()
		return
	}
	if event.Code == xui.KeyTab {
		if event.Mods.Has(xui.ModShift) {
			p.selectTab(-1)
		} else {
			p.selectTab(1)
		}
		return
	}
	// Plain runes are free outside name entry, so the list speaks the whole
	// standard dialect through the shared parser.
	if m, ok := p.motions.Key(event); ok {
		p.clampSelection()
		p.cursor().Apply(m)
		return
	}
	switch event.Code {
	case xui.KeyEscape:
		// An open picker closes first; only a bare Escape hides the modal.
		if p.skillsActionOpen >= 0 {
			p.skillsTypeOpen, p.skillsActionOpen = -1, -1
			p.clampSelection()
			return
		}
		if p.modelTypeOpen >= 0 {
			p.modelTypeOpen = -1
			p.clampSelection()
			return
		}
		if p.agentModelOpen >= 0 || p.agentBulkOpen {
			p.agentModelOpen, p.agentBulkOpen = -1, false
			p.clampSelection()
			return
		}
		p.Hide()
	case xui.KeyEnter:
		p.activateSelected()
	case xui.KeyRune:
		if event.Mods == 0 && event.Rune == ' ' {
			p.activateSelected()
		}
	}
}

func (p *Pane) handleNameKey(event xui.KeyEvent) {
	switch event.Code {
	case xui.KeyEscape:
		p.cancelNameEntry()
		p.errText = ""
	case xui.KeyEnter:
		p.commitNameEntry()
	case xui.KeyBackspace:
		runes := []rune(p.nameDraft)
		if len(runes) > 0 {
			p.nameDraft = string(runes[:len(runes)-1])
		}
	case xui.KeyRune:
		if !event.Mods.Has(xui.ModCtrl) && !event.Mods.Has(xui.ModAlt) {
			if p.nameMode == nameThreshold && (event.Rune < '0' || event.Rune > '9') {
				return
			}
			p.nameDraft += string(event.Rune)
		}
	}
}

func (p *Pane) handleMouse(event xui.MouseEvent) {
	if p.nameMode != nameNone {
		return
	}
	if event.Action == xui.MousePress && event.Button == xui.MouseLeft {
		for _, hit := range p.tabRegions {
			if event.Y == hit.y && event.X >= hit.x0 && event.X < hit.x1 {
				p.tab = hit.tab
				return
			}
		}
		if event.Y >= p.bodyTop && event.Y < p.bodyTop+p.viewport[p.tab] &&
			event.X >= p.bodyLeft && event.X < p.bodyLeft+p.bodyWidth {
			idx := p.cursors[p.tab].Scroll() + event.Y - p.bodyTop
			rows := p.rows(p.tab)
			if idx < len(rows) {
				p.clampSelection()
				p.cursor().Select(idx)
				p.activate(rows[idx])
			}
		}
		return
	}
	p.clampSelection()
	p.cursor().Wheel(event)
}

func (p *Pane) selectTab(delta int) {
	tabs := []Tab{TabPlanDefaults, TabGeneral, TabAgents}
	idx := max(slices.Index(tabs, p.tab), 0)
	idx = ((idx+delta)%len(tabs) + len(tabs)) % len(tabs)
	p.tab = tabs[idx]
	p.motions.Reset()
	p.clampSelection()
}

// cursor is the active tab's list cursor.
func (p *Pane) cursor() *browse.Cursor { return &p.cursors[p.tab] }

// clampSelection re-teaches the cursor the active tab's rows; call it
// whenever they may have been rebuilt — a picker opened or closed, a type
// added or removed — so the selection stays inside the list.
func (p *Pane) clampSelection() {
	p.cursor().SetRows(len(p.rows(p.tab)), nil)
}

// followSelection brings the selection back into view after the rows moved
// under it; the cursor does the following itself.
func (p *Pane) followSelection() { p.clampSelection() }

func (p *Pane) activateSelected() {
	rows := p.rows(p.tab)
	idx := p.cursor().Selected()
	if idx >= 0 && idx < len(rows) {
		p.activate(rows[idx])
	}
}

func (p *Pane) activate(row paneRow) {
	if row.kind == rowCompactThreshold {
		p.nameMode = nameThreshold
		p.nameDraft = ""
		if p.draft.CompactReminderTokens > 0 {
			p.nameDraft = strconv.Itoa(p.draft.CompactReminderTokens)
		}
		p.errText = ""
		return
	}
	if p.tab == TabAgents {
		p.activateAgents(row)
		return
	}
	if p.tab != TabPlanDefaults {
		return
	}
	switch row.kind {
	case rowAuthoringPolicy:
		next := plangate.AuthoringLegacy
		if p.draft.Plan.AuthoringPolicy == plangate.AuthoringLegacy {
			next = plangate.AuthoringAdaptiveMinimal
		}
		if err := p.draft.SetAuthoringPolicy(next); err != nil {
			p.errText = err.Error()
			return
		}
		p.markDirty()
	case rowAddType:
		p.nameMode = nameAdd
		p.nameTypeIndex = len(p.draft.Plan.Types)
		p.nameDraft = ""
		p.errText = ""
	case rowReset:
		p.resetBuiltIns()
	case rowRenameType:
		if row.typeIndex >= 0 && row.typeIndex < len(p.draft.Plan.Types) {
			p.nameMode = nameRename
			p.nameTypeIndex = row.typeIndex
			p.nameDraft = string(p.draft.Plan.Types[row.typeIndex].Name)
			p.errText = ""
		}
	case rowDeleteType:
		p.deleteType(row.typeIndex)
	case rowMoveTypeUp:
		p.moveType(row.typeIndex, -1)
	case rowMoveTypeDown:
		p.moveType(row.typeIndex, 1)
	case rowTypeModel:
		// Toggle the inline model list; only one type stays open at a time.
		if p.modelTypeOpen == row.typeIndex {
			p.modelTypeOpen = -1
		} else {
			p.modelTypeOpen = row.typeIndex
			p.skillsTypeOpen, p.skillsActionOpen = -1, -1
		}
		p.clampSelection()
	case rowModelOption:
		if row.typeIndex >= 0 && row.typeIndex < len(p.draft.Plan.Types) {
			model := ""
			if row.modelIndex >= 0 && row.modelIndex < len(p.modelNames) {
				model = p.modelNames[row.modelIndex]
			}
			p.draft.Plan.Types[row.typeIndex].Model = model
			p.modelTypeOpen = -1
			p.markDirty()
			p.clampSelection()
		}
	case rowPermission:
		p.togglePermission(row.typeIndex, row.tool)
	case rowOutsidePlan:
		p.toggleOutsidePlan(row.tool)
	case rowPlanActionAdd:
		if err := p.draft.AddPlanAction(); err != nil {
			p.errText = err.Error()
			return
		}
		p.markDirty()
		p.clampSelection()
	case rowPlanActionEvent:
		if action := p.planActionAt(row.actionIndex); action != nil {
			action.Event = cyclePlanActionEvent(action.Event)
			p.markDirty()
		}
	case rowPlanActionType:
		if action := p.planActionAt(row.actionIndex); action != nil {
			action.Type = cycleActionType(action.Type)
			if action.Type == session.PlanActionCompact {
				action.Skills = nil
			}
			p.markDirty()
		}
	case rowPlanActionSkills:
		p.modelTypeOpen = -1
		p.toggleSkillsPicker(-1, row.actionIndex)
		p.errText = ""
	case rowPlanActionRemove:
		p.draft.RemovePlanAction(row.actionIndex)
		p.markDirty()
		p.clampSelection()
		p.followSelection()
	case rowTypeActionAdd:
		if err := p.draft.AddTypeAction(row.typeIndex); err != nil {
			p.errText = err.Error()
			return
		}
		p.markDirty()
		p.clampSelection()
	case rowTypeActionEvent:
		if action := p.typeActionAt(row.typeIndex, row.actionIndex); action != nil {
			action.Event = cycleStepActionEvent(action.Event)
			p.markDirty()
		}
	case rowTypeActionType:
		if action := p.typeActionAt(row.typeIndex, row.actionIndex); action != nil {
			action.Type = cycleActionType(action.Type)
			if action.Type == session.PlanActionCompact {
				action.Skills = nil
			}
			p.markDirty()
		}
	case rowTypeActionSkills:
		p.modelTypeOpen = -1
		p.toggleSkillsPicker(row.typeIndex, row.actionIndex)
		p.errText = ""
	case rowSkillOption:
		p.toggleActionSkill(row.typeIndex, row.actionIndex, row.skillIndex)
	case rowTypeActionRemove:
		p.draft.RemoveTypeAction(row.typeIndex, row.actionIndex)
		p.markDirty()
		p.clampSelection()
		p.followSelection()
	case rowLocked:
		p.errText = row.tool + " is always allowed outside plan"
	}
}

// commitThresholdEntry parses the digit entry into the draft. Empty means
// the default threshold; anything but a non-negative integer is refused.
func (p *Pane) commitThresholdEntry() {
	text := strings.TrimSpace(p.nameDraft)
	if text == "" {
		p.draft.CompactReminderTokens = 0
	} else {
		n, err := strconv.Atoi(text)
		if err != nil || n < 0 {
			p.errText = "threshold must be a non-negative integer number of tokens"
			return
		}
		p.draft.CompactReminderTokens = n
	}
	p.cancelNameEntry()
	p.markDirty()
	p.clampSelection()
}

// agentModelOptionRows renders one open picker's choices: the inherit-clear
// entry first, then every configured model name. roleIndex -1 names the
// bulk "all roles" picker; modelIndex -1 is the inherit entry itself.
func (p *Pane) agentModelOptionRows(roleIndex int) []paneRow {
	rows := []paneRow{
		{text: "      (inherit session model)", kind: rowAgentModelOption, typeIndex: roleIndex, modelIndex: -1},
	}
	for j, name := range p.modelNames {
		rows = append(
			rows,
			paneRow{text: "      " + name, kind: rowAgentModelOption, typeIndex: roleIndex, modelIndex: j},
		)
	}
	return rows
}

// activateAgents routes Agents-tab rows: headers toggle inline pickers,
// options pin or clear one role — or every role, from the bulk picker.
func (p *Pane) activateAgents(row paneRow) {
	switch row.kind {
	case rowAgentBulkModel:
		p.agentModelOpen = -1
		p.agentBulkOpen = !p.agentBulkOpen
	case rowAgentModel:
		p.agentBulkOpen = false
		if p.agentModelOpen == row.typeIndex {
			p.agentModelOpen = -1
		} else {
			p.agentModelOpen = row.typeIndex
		}
	case rowAgentModelOption:
		model := ""
		if row.modelIndex >= 0 && row.modelIndex < len(p.modelNames) {
			model = p.modelNames[row.modelIndex]
		}
		if row.typeIndex < 0 {
			for _, role := range agentRoles {
				p.setAgentModel(role, model)
			}
			p.agentBulkOpen = false
		} else if row.typeIndex < len(agentRoles) {
			p.setAgentModel(agentRoles[row.typeIndex], model)
			p.agentModelOpen = -1
		}
		p.markDirty()
	}
	p.clampSelection()
}

// setAgentModel pins or clears one role in the draft. An empty model means
// "inherit" and is represented by absence, never by an empty entry.
func (p *Pane) setAgentModel(role job.Role, model string) {
	if model == "" {
		delete(p.draft.AgentModels, string(role))
		return
	}
	if p.draft.AgentModels == nil {
		p.draft.AgentModels = make(map[string]string, len(agentRoles))
	}
	p.draft.AgentModels[string(role)] = model
}

// agentBulkValue renders the shared pin for the bulk row: "mixed" when
// roles disagree, otherwise the common name or the inherit placeholder.
func (p *Pane) agentBulkValue() string {
	shared := ""
	for i, role := range agentRoles {
		pin := p.draft.AgentModels[string(role)]
		if i == 0 {
			shared = pin
		} else if pin != shared {
			return "mixed"
		}
	}
	if shared == "" {
		return "(inherit session model)"
	}
	return shared
}

// toggleSkillsPicker expands the known-skills list under the addressed
// action's skills row, or collapses it when already open. typeIndex -1
// addresses the plan-scope action, otherwise the type's; opening one inline
// picker closes the other, so Escape closes a picker before the modal.
func (p *Pane) toggleSkillsPicker(typeIndex, actionIndex int) {
	if p.skillsTypeOpen == typeIndex && p.skillsActionOpen == actionIndex {
		p.skillsTypeOpen, p.skillsActionOpen = -1, -1
	} else {
		p.skillsTypeOpen, p.skillsActionOpen = typeIndex, actionIndex
	}
	p.clampSelection()
}

// toggleActionSkill flips one known skill's membership in the addressed
// action's list; the picker stays open so a pick can be taken back at once.
func (p *Pane) toggleActionSkill(typeIndex, actionIndex, skillIndex int) {
	var action *session.PlanAction
	if typeIndex >= 0 {
		action = p.typeActionAt(typeIndex, actionIndex)
	} else {
		action = p.planActionAt(actionIndex)
	}
	if action == nil || skillIndex < 0 || skillIndex >= len(p.skills) {
		return
	}
	name := p.skills[skillIndex]
	next := make([]string, 0, len(action.Skills)+1)
	selected := false
	for _, skill := range action.Skills {
		if skill == name {
			selected = true
			continue
		}
		next = append(next, skill)
	}
	if !selected {
		next = append(next, name)
	}
	action.Skills = next
	p.markDirty()
}

func (p *Pane) cancelNameEntry() {
	p.nameMode = nameNone
	p.nameTypeIndex = -1
	p.nameDraft = ""
}

func (p *Pane) commitNameEntry() {
	if p.nameMode == nameThreshold {
		p.commitThresholdEntry()
		return
	}
	if p.nameDraft != strings.TrimSpace(p.nameDraft) {
		p.errText = "step type names must be lowercase slugs without surrounding spaces"
		return
	}
	name := session.StepType(p.nameDraft)
	var old session.StepType
	switch p.nameMode {
	case nameAdd:
		if err := p.draft.AddType(name); err != nil {
			p.errText = err.Error()
			return
		}
	case nameRename:
		renamed, err := p.draft.RenameType(p.nameTypeIndex, name)
		if err != nil {
			p.errText = err.Error()
			return
		}
		old = renamed
	default:
		return
	}
	if p.nameMode == nameRename {
		if old == name {
			p.cancelNameEntry()
			p.errText = ""
			return
		}
		p.draft.RecordRename(old, name)
	}
	p.cancelNameEntry()
	p.markDirty()
	p.clampSelection()
}

// moveType swaps a type with its neighbor in the capability hierarchy.
// Reordering changes only cascade semantics — plan references and renames
// are untouched, so a type in use may still move.
func (p *Pane) moveType(index, delta int) {
	if p.draft.MoveType(index, delta) {
		p.markDirty()
		p.clampSelection()
		p.followSelection()
	}
}

func (p *Pane) deleteType(index int) {
	if index < 0 || index >= len(p.draft.Plan.Types) {
		return
	}
	if p.isTypeInUse(p.draft.Plan.Types[index].Name) {
		p.errText = fmt.Sprintf("step type %q is used by the current plan", p.draft.Plan.Types[index].Name)
		return
	}
	p.draft.DeleteType(index)
	p.markDirty()
	p.clampSelection()
	p.followSelection()
}

func (p *Pane) isTypeInUse(name session.StepType) bool {
	if p.typeInUse == nil {
		return false
	}
	if p.typeInUse(name) {
		return true
	}
	for from, to := range p.draft.TypeRenames {
		if to == name && p.typeInUse(from) {
			return true
		}
	}
	return false
}

func (p *Pane) resetBuiltIns() {
	defaults := plangate.DefaultDefaults()
	defaultNames := make(map[session.StepType]struct{}, len(defaults.Types))
	for _, typ := range defaults.Types {
		defaultNames[typ.Name] = struct{}{}
	}
	for _, typ := range p.draft.Plan.Types {
		if _, retained := defaultNames[typ.Name]; !retained && p.isTypeInUse(typ.Name) {
			p.errText = fmt.Sprintf("step type %q is used by the current plan", typ.Name)
			return
		}
	}
	if reflect.DeepEqual(p.draft.Plan, defaults) && len(p.draft.TypeRenames) == 0 {
		p.errText = ""
		return
	}
	p.draft.Reset()
	p.markDirty()
	p.clampSelection()
	p.followSelection()
}

func (p *Pane) togglePermission(typeIndex int, tool string) {
	p.draft.TogglePermission(typeIndex, tool)
	p.markDirty()
}

func (p *Pane) toggleOutsidePlan(tool string) {
	p.draft.ToggleOutsidePlan(tool)
	p.markDirty()
}

func (p *Pane) markDirty() {
	p.dirty = true
	p.errText = ""
}

func (p *Pane) apply() {
	if p.store == nil {
		p.errText = "settings store unavailable"
		return
	}
	snapshot, err := p.store.Apply(context.Background(), p.draft)
	if err != nil {
		p.errText = err.Error()
		return
	}
	if p.onApplied != nil {
		p.onApplied(snapshot)
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

	pw := min(min(max(w-4, 20), 96), w)
	ph := min(min(max(h-2, 6), 30), h)
	x0, y0 := (w-pw)/2, (h-ph)/2
	panel := components.NewSurface(pw, ph, p)
	fillSurface(&panel, xui.Style{Fg: th.Foreground.Fg, Bg: th.BackgroundElement.Bg})
	layout.DrawRoundedBorder(
		&panel,
		layout.BorderRounded,
		xui.Style{Fg: th.Muted.Fg, Bg: th.BackgroundElement.Bg},
		&layout.BorderLabel{Text: " Harness settings ", Style: th.Foreground},
		nil,
		nil,
		&layout.BorderLabel{Text: keys.Footer(keys.ScopeSettings), Style: th.Muted},
		ctx.Method,
	)

	tabs := []struct {
		tab   Tab
		label string
	}{{TabPlanDefaults, "Plan defaults"}, {TabGeneral, "General"}, {TabAgents, "Agents"}}
	p.tabRegions = p.tabRegions[:0]
	x := 2
	for _, item := range tabs {
		label := " " + item.label + " "
		style := th.Muted
		if p.tab == item.tab {
			style = xui.Style{Bold: true, Reverse: true}
		}
		panel.Print(x, 1, label, style, ctx.Method)
		p.tabRegions = append(p.tabRegions, region{x0: x0 + x, x1: x0 + x + len(label), y: y0 + 1, tab: item.tab})
		x += len(label) + 1
	}

	bodyTop := 3
	view := max(ph-5, 0)
	p.viewport[p.tab] = view
	p.cursors[p.tab].SetViewport(view)
	rows := p.rows(p.tab)
	p.overflow[p.tab] = len(rows) > view
	p.clampSelection()
	p.bodyTop, p.bodyLeft, p.bodyWidth = y0+bodyTop, x0+min(2, pw), max(pw-4, 0)

	for i := range view {
		idx := p.cursors[p.tab].Scroll() + i
		if idx >= len(rows) || bodyTop+i >= ph-1 {
			break
		}
		style := th.Foreground
		marker := "  "
		if idx == p.cursors[p.tab].Selected() {
			style = xui.Style{Reverse: true}
			marker = "› "
		}
		line := marker + rows[idx].text
		panel.Print(2, bodyTop+i, layout.TruncateToWidth(line, pw-5, ctx.Method), style, ctx.Method)
	}
	if p.overflow[p.tab] {
		drawScrollbar(&panel, pw-2, bodyTop, view, len(rows), p.cursors[p.tab].Scroll(), th.Muted)
	}
	if p.errText != "" && ph >= 3 {
		panel.Print(2, ph-2, layout.TruncateToWidth("Error: "+p.errText, pw-4, ctx.Method), th.Warning, ctx.Method)
	}
	blit(&root, panel, x0, y0)
	return root
}

func (p *Pane) rows(tab Tab) []paneRow {
	if tab == TabGeneral {
		value := "default"
		if p.draft.CompactReminderTokens > 0 {
			value = fmt.Sprintf("%d tokens", p.draft.CompactReminderTokens)
		}
		text := "Compact reminder threshold: " + value
		if p.nameMode == nameThreshold {
			text = "Compact reminder threshold (tokens): " + p.nameDraft + "_"
		}
		return []paneRow{
			{text: text, kind: rowCompactThreshold},
			{text: "Config path: " + p.configPath},
			{text: "Scope: global"},
			{text: "Live apply: always on — Apply publishes the policy for the next inference"},
			{text: "Saved changes affect the current gate and future sessions."},
		}
	}

	if tab == TabAgents {
		rows := []paneRow{{text: "Sub-agent models · a pin applies at spawn; unset roles inherit the session model"}}
		rows = append(rows, paneRow{text: "Model for all roles: " + p.agentBulkValue(), kind: rowAgentBulkModel})
		for i, role := range agentRoles {
			model := p.draft.AgentModels[string(role)]
			if model == "" {
				model = "(inherit session model)"
			}
			rows = append(
				rows,
				paneRow{text: "Model for " + string(role) + ": " + model, kind: rowAgentModel, typeIndex: i},
			)
			if p.agentModelOpen == i {
				rows = append(rows, p.agentModelOptionRows(i)...)
			}
		}
		if p.agentBulkOpen {
			rows = append(rows, p.agentModelOptionRows(-1)...)
		}
		rows = append(rows,
			paneRow{text: "Pins persist under agents.models; a stale model name degrades to inherit with a warning."},
			paneRow{text: "Saved changes affect the next spawn and future sessions."},
		)
		return rows
	}

	grammar := p.draft.Plan.AuthoringPolicy
	if grammar == "" {
		grammar = plangate.AuthoringAdaptiveMinimal
	}
	rows := []paneRow{
		{text: "Step types · ordered least to most capable"},
		{text: "Authoring grammar: " + string(grammar), kind: rowAuthoringPolicy},
	}
	// The available skills lead the tab: inject_skill actions name them, and
	// a list hidden under the tool catalog is a list nobody finds.
	for _, line := range skillsLines(p.skills, 58) {
		rows = append(rows, paneRow{text: line})
	}
	// Plan-scope default actions ride the top of the tab: new plans without
	// their own automation inherit this list.
	rows = append(rows, paneRow{text: "Plan actions · new plans inherit them"})
	if len(p.draft.Plan.Actions) == 0 {
		rows = append(rows, paneRow{text: "  (none)"})
	}
	for j, action := range p.draft.Plan.Actions {
		rows = append(rows,
			paneRow{
				text:        fmt.Sprintf("  [-] Action %d: on %s → %s", j+1, action.Event, action.Type),
				kind:        rowPlanActionRemove,
				actionIndex: j,
			},
			paneRow{text: "      event: " + string(action.Event), kind: rowPlanActionEvent, actionIndex: j},
			paneRow{text: "      type: " + string(action.Type), kind: rowPlanActionType, actionIndex: j},
		)
		if action.Type == session.PlanActionInjectSkill {
			rows = append(rows, paneRow{
				text:        "      " + actionSkillsText(action.Skills),
				kind:        rowPlanActionSkills,
				actionIndex: j,
			})
			if p.skillsTypeOpen < 0 && p.skillsActionOpen == j {
				rows = append(rows, skillOptionRows(action.Skills, p.skills, -1, j)...)
			}
		}
	}
	rows = append(rows, paneRow{text: "  [+] Add plan action", kind: rowPlanActionAdd})
	addText := "[+] Add type"
	if p.nameMode == nameAdd {
		addText = "Add type: " + p.nameDraft + "_"
	}
	rows = append(rows,
		paneRow{text: addText, kind: rowAddType},
		paneRow{text: "[↺] Reset built-in defaults", kind: rowReset},
	)
	catalog := plangate.KnownTools()
	for i, typ := range p.draft.Plan.Types {
		rows = append(rows, paneRow{text: fmt.Sprintf("Type: %s", typ.Name)})
		renameText := fmt.Sprintf("  Rename type %s", typ.Name)
		if p.nameMode == nameRename && p.nameTypeIndex == i {
			renameText = fmt.Sprintf("  Rename type %s: %s_", typ.Name, p.nameDraft)
		}
		rows = append(rows,
			paneRow{text: renameText, kind: rowRenameType, typeIndex: i},
			paneRow{text: fmt.Sprintf("  Move type %s up", typ.Name), kind: rowMoveTypeUp, typeIndex: i},
			paneRow{text: fmt.Sprintf("  Move type %s down", typ.Name), kind: rowMoveTypeDown, typeIndex: i},
			paneRow{text: fmt.Sprintf("  Delete type %s", typ.Name), kind: rowDeleteType, typeIndex: i},
		)
		// The type's model pin: new plans inherit it as their ModelsByType
		// map entry. Enter expands the inline choice list.
		model := typ.Model
		if model == "" {
			model = "(session default)"
		}
		rows = append(rows, paneRow{text: "  Model: " + model, kind: rowTypeModel, typeIndex: i})
		if p.modelTypeOpen == i {
			rows = append(
				rows,
				paneRow{text: "      (session default)", kind: rowModelOption, typeIndex: i, modelIndex: -1},
			)
			for j, name := range p.modelNames {
				rows = append(rows, paneRow{text: "      " + name, kind: rowModelOption, typeIndex: i, modelIndex: j})
			}
		}
		// Step-scope default actions: new steps of this type inherit them.
		for j, action := range typ.Actions {
			rows = append(rows,
				paneRow{
					text:        fmt.Sprintf("    [-] Action %d: on %s → %s", j+1, action.Event, action.Type),
					kind:        rowTypeActionRemove,
					typeIndex:   i,
					actionIndex: j,
				},
				paneRow{
					text:        "        event: " + string(action.Event),
					kind:        rowTypeActionEvent,
					typeIndex:   i,
					actionIndex: j,
				},
				paneRow{
					text:        "        type: " + string(action.Type),
					kind:        rowTypeActionType,
					typeIndex:   i,
					actionIndex: j,
				},
			)
			if action.Type == session.PlanActionInjectSkill {
				rows = append(rows, paneRow{
					text:        "        " + actionSkillsText(action.Skills),
					kind:        rowTypeActionSkills,
					typeIndex:   i,
					actionIndex: j,
				})
				if p.skillsTypeOpen == i && p.skillsActionOpen == j {
					rows = append(rows, skillOptionRows(action.Skills, p.skills, i, j)...)
				}
			}
		}
		rows = append(rows, paneRow{
			text:      fmt.Sprintf("    [+] Add action to %s", typ.Name),
			kind:      rowTypeActionAdd,
			typeIndex: i,
		})
		for _, tool := range catalog {
			if tool.MandatoryExemption {
				continue
			}
			availability := p.toolAvailability(tool.Name)
			minimum := p.draft.AssignmentRank(tool.Name)
			checked := minimum >= 0 && minimum <= i
			mark := " "
			if checked {
				mark = "x"
			}
			rows = append(rows, paneRow{
				text:      fmt.Sprintf("  [%s] %s · for %s · %s", mark, tool.Name, typ.Name, availability),
				kind:      rowPermission,
				typeIndex: i,
				tool:      tool.Name,
			})
		}
	}

	rows = append(rows, paneRow{text: "Allowed outside plan · bypasses the plan gate"})
	for _, tool := range catalog {
		if tool.MandatoryExemption {
			continue
		}
		mark := " "
		if slices.Contains(p.draft.Plan.AdditionalExemptions, tool.Name) {
			mark = "x"
		}
		rows = append(rows, paneRow{
			text: fmt.Sprintf(
				"  [%s] %s · allowed outside plan · %s",
				mark,
				tool.Name,
				p.toolAvailability(tool.Name),
			),
			kind: rowOutsidePlan,
			tool: tool.Name,
		})
	}
	rows = append(rows, paneRow{text: "Mandatory outside plan · always bypasses the plan gate"})
	for _, tool := range catalog {
		if !tool.MandatoryExemption {
			continue
		}
		rows = append(rows, paneRow{
			text: fmt.Sprintf("  [x] %s · always allowed (locked) · %s", tool.Name, p.toolAvailability(tool.Name)),
			kind: rowLocked,
			tool: tool.Name,
		})
	}
	return rows
}

// skillsLines lays the available skill names out as one enumeration —
// "skills: a, b, c" — wrapped at width with continuation lines indented under
// the names, so a long catalog reads as a block instead of one clipped line.
func skillsLines(names []string, width int) []string {
	if len(names) == 0 {
		return []string{"skills: none"}
	}
	const prefix = "skills: "
	indent := strings.Repeat(" ", len(prefix))
	lines := []string{prefix}
	for i, name := range names {
		item := name
		if i < len(names)-1 {
			item += ","
		}
		switch {
		case strings.HasSuffix(lines[len(lines)-1], prefix):
			lines[len(lines)-1] += item
		case len([]rune(lines[len(lines)-1]))+1+len([]rune(item)) <= width:
			lines[len(lines)-1] += " " + item
		default:
			lines = append(lines, indent+item)
		}
	}
	return lines
}

// cyclePlanActionEvent and cycleStepActionEvent step an event inside its own
// scope: plan actions live on the plan boundary, step actions on the step
// boundary, and crossing scopes would fail authoring validation anyway.
func cyclePlanActionEvent(event session.PlanActionEvent) session.PlanActionEvent {
	if event == session.PlanActionOnPlanStart {
		return session.PlanActionOnPlanEnd
	}
	return session.PlanActionOnPlanStart
}

func cycleStepActionEvent(event session.PlanActionEvent) session.PlanActionEvent {
	if event == session.PlanActionOnStepStart {
		return session.PlanActionOnStepEnd
	}
	return session.PlanActionOnStepStart
}

// cycleActionType flips the action's built-in. compact ignores skills and
// authoring rejects compact-with-skills, so flipping to compact drops them.
func cycleActionType(kind session.PlanActionType) session.PlanActionType {
	if kind == session.PlanActionCompact {
		return session.PlanActionInjectSkill
	}
	return session.PlanActionCompact
}

// planActionAt addresses the draft's plan-scope action at index; nil when the
// draft changed underneath the row.
func (p *Pane) planActionAt(index int) *session.PlanAction {
	if index < 0 || index >= len(p.draft.Plan.Actions) {
		return nil
	}
	return &p.draft.Plan.Actions[index]
}

// typeActionAt addresses a type's step-scope action, same staleness rule.
func (p *Pane) typeActionAt(typeIndex, actionIndex int) *session.PlanAction {
	if typeIndex < 0 || typeIndex >= len(p.draft.Plan.Types) {
		return nil
	}
	actions := p.draft.Plan.Types[typeIndex].Actions
	if actionIndex < 0 || actionIndex >= len(actions) {
		return nil
	}
	return &actions[actionIndex]
}

// actionSkillsText renders an action's skills line.
func actionSkillsText(skills []string) string {
	if len(skills) == 0 {
		return "skills: (none — open to pick)"
	}
	return "skills: " + strings.Join(skills, ", ")
}

// skillOptionRows lays the skills picker out: one toggle row per known
// skill, marked with its membership in the action, indented under the
// action's own rows.
func skillOptionRows(skills, names []string, typeIndex, actionIndex int) []paneRow {
	indent := "        "
	if typeIndex < 0 {
		indent = "      "
	}
	if len(names) == 0 {
		return []paneRow{{text: indent + "(no skills installed)"}}
	}
	rows := make([]paneRow, 0, len(names))
	for k, name := range names {
		mark := "[ ]"
		if slices.Contains(skills, name) {
			mark = "[x]"
		}
		rows = append(rows, paneRow{
			text:        indent + mark + " " + name,
			kind:        rowSkillOption,
			typeIndex:   typeIndex,
			actionIndex: actionIndex,
			skillIndex:  k,
		})
	}
	return rows
}

func (p *Pane) toolAvailability(tool string) string {
	if !p.availabilitySet {
		return "available"
	}
	if _, ok := p.availableTools[tool]; ok {
		return "available"
	}
	return "unavailable"
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
