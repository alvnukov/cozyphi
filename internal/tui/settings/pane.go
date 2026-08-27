// Package settings renders the global harness settings modal. Pane owns only
// an editable draft and interaction state; validation, merge, persistence, and
// live publication remain behind Store.
package settings

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

// Tab identifies one top-level settings section.
type Tab uint8

const (
	TabPlanDefaults Tab = iota
	TabGeneral
	tabCount
)

// Store is the complete persistence seam used by the modal.
type Store interface {
	Snapshot() harnesssettings.Snapshot
	Apply(context.Context, harnesssettings.Draft) (harnesssettings.Snapshot, error)
}

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
	rowPermission
	rowOutsidePlan
	rowLocked
)

type paneRow struct {
	text      string
	kind      rowKind
	typeIndex int
	tool      string
}

type nameMode uint8

const (
	nameNone nameMode = iota
	nameAdd
	nameRename
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

	configPath    string
	typeInUse     func(session.StepType) bool
	baseTypeNames map[session.StepType]struct{}

	availabilitySet bool
	availableTools  map[string]struct{}

	nameMode      nameMode
	nameTypeIndex int
	nameDraft     string

	scroll   [tabCount]int
	selected [tabCount]int
	viewport [tabCount]int
	overflow [tabCount]bool

	tabRegions []region
	bodyTop    int
	bodyLeft   int
	bodyWidth  int
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
	p.scroll = [tabCount]int{}
	p.selected = [tabCount]int{}
	p.viewport = [tabCount]int{}
	p.overflow = [tabCount]bool{}
	p.dirty = false
	p.errText = ""
	p.configPath = "(config unavailable)"
	p.cancelNameEntry()
	if p.store != nil {
		snapshot := p.store.Snapshot()
		p.draft = snapshot.Draft()
		p.configPath = snapshot.Path
		p.baseTypeNames = make(map[session.StepType]struct{}, len(snapshot.Plan.Types))
		for _, typ := range snapshot.Plan.Types {
			p.baseTypeNames[typ.Name] = struct{}{}
		}
	} else {
		p.draft = harnesssettings.Draft{}
		p.baseTypeNames = nil
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
		Scroll:   p.scroll[p.tab],
		Selected: p.selected[p.tab],
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
	switch event.Code {
	case xui.KeyEscape:
		p.Hide()
	case xui.KeyTab:
		if event.Mods.Has(xui.ModShift) {
			p.selectTab(-1)
		} else {
			p.selectTab(1)
		}
	case xui.KeyUp:
		p.moveSelection(-1)
	case xui.KeyDown:
		p.moveSelection(1)
	case xui.KeyPageUp:
		p.moveSelection(-max(p.viewport[p.tab]-1, 1))
	case xui.KeyPageDown:
		p.moveSelection(max(p.viewport[p.tab]-1, 1))
	case xui.KeyHome:
		p.selected[p.tab] = 0
		p.followSelection()
	case xui.KeyEnd:
		p.selected[p.tab] = max(len(p.rows(p.tab))-1, 0)
		p.followSelection()
	case xui.KeyEnter:
		p.activateSelected()
	case xui.KeyRune:
		if event.Rune == ' ' && event.Mods == 0 {
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
			idx := p.scroll[p.tab] + event.Y - p.bodyTop
			rows := p.rows(p.tab)
			if idx < len(rows) {
				p.selected[p.tab] = idx
				p.activate(rows[idx])
			}
		}
		return
	}
	step := max(event.Wheel, 1) * 3
	switch event.Button {
	case xui.MouseWheelUp:
		p.scroll[p.tab] -= step
	case xui.MouseWheelDown:
		p.scroll[p.tab] += step
	}
	p.clampScroll()
}

func (p *Pane) selectTab(delta int) {
	tabs := []Tab{TabPlanDefaults, TabGeneral}
	idx := max(slices.Index(tabs, p.tab), 0)
	idx = ((idx+delta)%len(tabs) + len(tabs)) % len(tabs)
	p.tab = tabs[idx]
	p.clampSelection()
}

func (p *Pane) moveSelection(delta int) {
	p.selected[p.tab] += delta
	p.clampSelection()
	p.followSelection()
}

func (p *Pane) clampSelection() {
	p.selected[p.tab] = clamp(p.selected[p.tab], 0, max(len(p.rows(p.tab))-1, 0))
}

func (p *Pane) followSelection() {
	view := p.viewport[p.tab]
	if view <= 0 {
		p.scroll[p.tab] = 0
		return
	}
	p.scroll[p.tab] = min(p.scroll[p.tab], p.selected[p.tab])
	if p.selected[p.tab] >= p.scroll[p.tab]+view {
		p.scroll[p.tab] = p.selected[p.tab] - view + 1
	}
	p.clampScroll()
}

func (p *Pane) clampScroll() {
	view := p.viewport[p.tab]
	if view <= 0 {
		p.scroll[p.tab] = 0
		return
	}
	p.scroll[p.tab] = clamp(p.scroll[p.tab], 0, max(len(p.rows(p.tab))-view, 0))
}

func (p *Pane) activateSelected() {
	rows := p.rows(p.tab)
	idx := p.selected[p.tab]
	if idx >= 0 && idx < len(rows) {
		p.activate(rows[idx])
	}
}

func (p *Pane) activate(row paneRow) {
	if p.tab != TabPlanDefaults {
		return
	}
	switch row.kind {
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
	case rowPermission:
		p.togglePermission(row.typeIndex, row.tool)
	case rowOutsidePlan:
		p.toggleOutsidePlan(row.tool)
	case rowLocked:
		p.errText = row.tool + " is always allowed outside plan"
	}
}

func (p *Pane) cancelNameEntry() {
	p.nameMode = nameNone
	p.nameTypeIndex = -1
	p.nameDraft = ""
}

func (p *Pane) commitNameEntry() {
	policy, err := plangate.Compile(p.draft.Plan)
	if err != nil {
		p.errText = err.Error()
		return
	}
	candidate := policy.Defaults()
	if p.nameDraft != strings.TrimSpace(p.nameDraft) {
		p.errText = "step type names must be lowercase slugs without surrounding spaces"
		return
	}
	name := session.StepType(p.nameDraft)
	var old session.StepType
	switch p.nameMode {
	case nameAdd:
		candidate.Types = append(candidate.Types, plangate.TypeDefaults{Name: name})
	case nameRename:
		if p.nameTypeIndex < 0 || p.nameTypeIndex >= len(candidate.Types) {
			p.errText = "selected step type no longer exists"
			return
		}
		old = candidate.Types[p.nameTypeIndex].Name
		candidate.Types[p.nameTypeIndex].Name = name
	default:
		return
	}
	validated, err := plangate.Compile(candidate)
	if err != nil {
		p.errText = err.Error()
		return
	}
	if p.nameMode == nameRename && old == name {
		p.cancelNameEntry()
		p.errText = ""
		return
	}
	p.draft.Plan = validated.Defaults()
	if p.nameMode == nameRename {
		p.recordRename(old, name)
	}
	p.cancelNameEntry()
	p.markDirty()
	p.clampSelection()
}

func (p *Pane) recordRename(old, name session.StepType) {
	source := old
	for from, to := range p.draft.TypeRenames {
		if to == old {
			source = from
			delete(p.draft.TypeRenames, from)
			break
		}
	}
	if source == name {
		return
	}
	if _, existedWhenOpened := p.baseTypeNames[source]; !existedWhenOpened {
		return
	}
	if p.draft.TypeRenames == nil {
		p.draft.TypeRenames = make(map[session.StepType]session.StepType)
	}
	p.draft.TypeRenames[source] = name
}

// moveType swaps a type with its neighbor in the capability hierarchy.
// Reordering changes only cascade semantics — plan references and renames
// are untouched, so a type in use may still move.
func (p *Pane) moveType(index, delta int) {
	target := index + delta
	if index < 0 || index >= len(p.draft.Plan.Types) || target < 0 || target >= len(p.draft.Plan.Types) {
		return
	}
	p.draft.Plan.Types[index], p.draft.Plan.Types[target] = p.draft.Plan.Types[target], p.draft.Plan.Types[index]
	p.markDirty()
	p.clampSelection()
	p.followSelection()
}

func (p *Pane) deleteType(index int) {
	if index < 0 || index >= len(p.draft.Plan.Types) {
		return
	}
	name := p.draft.Plan.Types[index].Name
	if p.isTypeInUse(name) {
		p.errText = fmt.Sprintf("step type %q is used by the current plan", name)
		return
	}
	p.draft.Plan.Types = slices.Delete(p.draft.Plan.Types, index, index+1)
	for from, to := range p.draft.TypeRenames {
		if from == name || to == name {
			delete(p.draft.TypeRenames, from)
		}
	}
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
	p.draft.Plan = defaults
	p.draft.TypeRenames = nil
	p.markDirty()
	p.clampSelection()
	p.followSelection()
}

func (p *Pane) togglePermission(typeIndex int, tool string) {
	if typeIndex < 0 || typeIndex >= len(p.draft.Plan.Types) {
		return
	}
	minimum := p.assignmentRank(tool)
	allowed := minimum >= 0 && minimum <= typeIndex
	p.removeToolAssignments(tool)
	p.draft.Plan.AdditionalExemptions = deleteString(p.draft.Plan.AdditionalExemptions, tool)
	if allowed {
		if typeIndex+1 < len(p.draft.Plan.Types) {
			p.draft.Plan.Types[typeIndex+1].Tools = append(p.draft.Plan.Types[typeIndex+1].Tools, tool)
		}
	} else {
		p.draft.Plan.Types[typeIndex].Tools = append(p.draft.Plan.Types[typeIndex].Tools, tool)
	}
	p.markDirty()
}

func (p *Pane) toggleOutsidePlan(tool string) {
	if slices.Contains(p.draft.Plan.AdditionalExemptions, tool) {
		p.draft.Plan.AdditionalExemptions = deleteString(p.draft.Plan.AdditionalExemptions, tool)
	} else {
		p.removeToolAssignments(tool)
		p.draft.Plan.AdditionalExemptions = append(p.draft.Plan.AdditionalExemptions, tool)
	}
	p.markDirty()
}

func (p *Pane) assignmentRank(tool string) int {
	for i, typ := range p.draft.Plan.Types {
		if slices.Contains(typ.Tools, tool) {
			return i
		}
	}
	return -1
}

func (p *Pane) removeToolAssignments(tool string) {
	for i := range p.draft.Plan.Types {
		p.draft.Plan.Types[i].Tools = deleteString(p.draft.Plan.Types[i].Tools, tool)
	}
}

func deleteString(values []string, value string) []string {
	return slices.DeleteFunc(values, func(item string) bool { return item == value })
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
		&layout.BorderLabel{Text: " Ctrl+S apply · Esc discard ", Style: th.Muted},
		ctx.Method,
	)

	tabs := []struct {
		tab   Tab
		label string
	}{{TabPlanDefaults, "Plan defaults"}, {TabGeneral, "General"}}
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
	rows := p.rows(p.tab)
	p.overflow[p.tab] = len(rows) > view
	p.clampSelection()
	p.clampScroll()
	p.bodyTop, p.bodyLeft, p.bodyWidth = y0+bodyTop, x0+min(2, pw), max(pw-4, 0)

	for i := range view {
		idx := p.scroll[p.tab] + i
		if idx >= len(rows) || bodyTop+i >= ph-1 {
			break
		}
		style := th.Foreground
		marker := "  "
		if idx == p.selected[p.tab] {
			style = xui.Style{Reverse: true}
			marker = "› "
		}
		line := marker + rows[idx].text
		panel.Print(2, bodyTop+i, layout.TruncateToWidth(line, pw-5, ctx.Method), style, ctx.Method)
	}
	if p.overflow[p.tab] {
		drawScrollbar(&panel, pw-2, bodyTop, view, len(rows), p.scroll[p.tab], th.Muted)
	}
	if p.errText != "" && ph >= 3 {
		panel.Print(2, ph-2, layout.TruncateToWidth("Error: "+p.errText, pw-4, ctx.Method), th.Warning, ctx.Method)
	}
	blit(&root, panel, x0, y0)
	return root
}

func (p *Pane) rows(tab Tab) []paneRow {
	if tab == TabGeneral {
		return []paneRow{
			{text: "Config path: " + p.configPath},
			{text: "Scope: global"},
			{text: "Live apply: always on — Apply publishes the policy for the next inference"},
			{text: "Saved changes affect the current gate and future sessions."},
		}
	}

	rows := []paneRow{{text: "Step types · ordered least to most capable"}}
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
		for _, tool := range catalog {
			if tool.MandatoryExemption {
				continue
			}
			availability := p.toolAvailability(tool.Name)
			minimum := p.assignmentRank(tool.Name)
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

func clamp(value, low, high int) int { return min(max(value, low), high) }
