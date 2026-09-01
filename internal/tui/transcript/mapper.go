package transcript

import (
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// Mapper converts session.Snapshot items into transcript widgets.
// It owns expand-state and has no dependency on Editor / xui / agent.
type Mapper struct {
	theme        components.Theme
	spinner      *status.Spinner
	expanded     map[string]bool
	onInvalidate func() // e.g. MessageList.InvalidateHeights
	// Children returns nested sub-agent tool rows for a parent tool_use id.
	Children func(parentToolUseID string) []block.ChildTool
	// ChildrenByJob returns nested rows keyed by job id (fallback for spawn/task).
	ChildrenByJob func(jobID string) []block.ChildTool
	// expandEdits is the sidebar's "edit cards render expanded" switch;
	// a diff card without an explicit per-row toggle is born under it.
	expandEdits bool
	// verbose renders every turn in full, bypassing turn condensation.
	verbose bool
	// summaries carries each condensed turn's stats, keyed by summary row id,
	// rebuilt by groupTurns on every full sync.
	summaries map[string]turnStats
	// onRegroup re-runs a full sync after a summary toggle: expanding a turn
	// changes which rows exist, which a height invalidation alone cannot show.
	onRegroup func()
}

// keepFullTurns is how many trailing turns always render in full: the
// running turn and the finished one just before it, which the reader is most
// likely still reviewing.
const keepFullTurns = 2

// itemTurnSummary is the mapper-local row kind for a condensed turn's
// summary line; session.Project never emits it.
const itemTurnSummary session.ItemKind = -1

// turnStats is what a condensed turn's summary row says about the rows it
// hides.
type turnStats struct {
	Duration time.Duration
	Tools    int
	Failed   int
	Rows     int
	Files    []string
}

// NewMapper builds a Mapper with the given theme, spinner, and invalidation callback.
func NewMapper(theme components.Theme, spinner *status.Spinner, onInvalidate func()) *Mapper {
	return &Mapper{
		theme:        theme,
		spinner:      spinner,
		expanded:     make(map[string]bool),
		expandEdits:  true,
		onInvalidate: onInvalidate,
	}
}

// SetTheme updates the theme used for newly built and patched widgets.
func (m *Mapper) SetTheme(theme components.Theme) {
	if m != nil {
		m.theme = theme
	}
}

// SetVerbose turns turn condensation off (true) or back on (false).
func (m *Mapper) SetVerbose(v bool) {
	if m != nil {
		m.verbose = v
	}
}

// Verbose reports whether turn condensation is off.
func (m *Mapper) Verbose() bool {
	return m != nil && m.verbose
}

// SetExpandEdits flips the default expansion of edit (diff) cards. Turning
// the switch off folds every card in entries and pins it collapsed; turning
// it on pins every existing card's current state instead, so only cards the
// feed has not seen yet are born open. Returns the indices of entries whose
// height changed.
func (m *Mapper) SetExpandEdits(enabled bool, entries []components.Widget, listIDs []string) []int {
	if m == nil || m.expandEdits == enabled {
		return nil
	}
	m.expandEdits = enabled
	var changed []int
	for i, w := range entries {
		d, ok := w.(*block.DiffBlock)
		if !ok {
			continue
		}
		id := entryID(listIDs, i)
		if id == "" {
			continue
		}
		if enabled {
			if _, seen := m.expanded[id]; !seen {
				m.expanded[id] = d.Expanded
			}
			continue
		}
		m.expanded[id] = false
		if d.Expanded {
			d.Expanded = false
			changed = append(changed, i)
		}
	}
	return changed
}

// Reset drops remembered expand state. Call it when entry identity resets —
// replaying another session's history — so stale ids cannot collide with the
// new transcript's ids and resurrect someone else's expanded rows.
func (m *Mapper) Reset() {
	if m != nil {
		clear(m.expanded)
	}
}

// Sync rebuilds the widget list from snap, reusing widgets when patchable.
// dirty lists new-entry indices whose height-relevant content changed (or are new).
func (m *Mapper) Sync(
	entries []components.Widget,
	listIDs []string,
	snap session.Snapshot,
) (newEntries []components.Widget, newIDs []string, dirty []int) {
	items := m.groupTurns(dropServiceRefusals(session.Project(snap)), snap)
	n := len(items)
	byID := make(map[string]int, len(entries))
	for i, w := range entries {
		id := entryID(listIDs, i)
		if id == "" {
			continue
		}
		byID[id] = i
		// DiffBlock and TurnSummaryBlock are deliberately absent: their
		// expansion is recomputed each sync (a diff card opens while its
		// change belongs to the running turn; a turn fold stays shut), and
		// only an explicit user toggle — recorded by OnToggle — overrides it.
		switch b := w.(type) {
		case *block.ThinkingBlock:
			m.expanded[id] = b.Expanded
		case *block.ToolBlock:
			m.expanded[id] = b.Expanded
		case *block.BashBlock:
			m.expanded[id] = b.Expanded
		case *block.AgentBlock:
			m.expanded[id] = b.Expanded
		case *block.CompactionBlock:
			m.expanded[id] = b.Expanded
		}
	}

	newEntries = make([]components.Widget, 0, n)
	newIDs = make([]string, 0, n)
	for _, it := range items {
		idx := len(newEntries)
		newIDs = append(newIDs, it.ID)
		if oldIdx, ok := byID[it.ID]; ok {
			if ok, changed := m.patchItem(entries[oldIdx], it); ok {
				newEntries = append(newEntries, entries[oldIdx])
				if changed {
					dirty = append(dirty, idx)
				}
				continue
			}
		}
		newEntries = append(newEntries, m.widgetFor(it))
		dirty = append(dirty, idx)
	}
	return newEntries, newIDs, dirty
}

// syncTail patches an unchanged set of rows projected by the last message.
// It fails closed when the projected shape changed or thinking coalesced across
// a message boundary; the caller must use the full Sync path in that case.
func (m *Mapper) syncTail(entries []components.Widget, listIDs []string, snap session.Snapshot) ([]int, bool) {
	if len(snap.Messages) == 0 || len(entries) != len(listIDs) {
		return nil, false
	}
	last := snap.Messages[len(snap.Messages)-1]
	items := dropServiceRefusals(session.Project(session.Snapshot{
		Messages: []session.Message{last},
		Tools:    snap.Tools,
	}))
	if len(items) == 0 || len(items) > len(listIDs) {
		return nil, false
	}
	start := len(listIDs) - len(items)
	for i, item := range items {
		if listIDs[start+i] != item.ID {
			return nil, false
		}
	}
	dirty := make([]int, 0, len(items))
	for i, item := range items {
		idx := start + i
		ok, changed := m.patchItem(entries[idx], item)
		if !ok {
			return nil, false
		}
		if changed {
			dirty = append(dirty, idx)
		}
	}
	return dirty, true
}

func entryID(listIDs []string, i int) string {
	if i >= 0 && i < len(listIDs) {
		return listIDs[i]
	}
	return ""
}

func (m *Mapper) patchItem(w components.Widget, it session.Item) (ok, dirty bool) {
	if it.Kind == itemTurnSummary {
		s, ok := w.(*block.TurnSummaryBlock)
		if !ok {
			return false, false
		}
		st := m.summaries[it.ID]
		prevExp := s.Expanded
		dirty = s.Duration != st.Duration || s.Tools != st.Tools || s.Failed != st.Failed ||
			s.Rows != st.Rows || !slices.Equal(s.Files, st.Files)
		s.Duration = st.Duration
		s.Tools = st.Tools
		s.Failed = st.Failed
		s.Rows = st.Rows
		s.Files = st.Files
		s.Theme = m.theme
		s.Expanded = m.expanded[it.ID]
		if s.Expanded != prevExp {
			dirty = true
		}
		return true, dirty
	}
	switch it.Kind {
	case session.ItemUser:
		u, ok := w.(*block.UserBlock)
		if !ok {
			return false, false
		}
		dirty = u.Text != it.Text || u.Queued != it.Queued
		u.Text = it.Text
		u.Queued = it.Queued
		u.Theme = m.theme
		return true, dirty
	case session.ItemAssistant:
		a, ok := w.(*block.AssistantBlock)
		if !ok {
			return false, false
		}
		label, tail := formatItemMeta(it)
		dirty = a.Text != it.Text || a.State != it.State || a.MetaLabel != label || a.MetaTail != tail
		a.Text = it.Text
		a.State = it.State
		a.MetaLabel = label
		a.MetaTail = tail
		a.Theme = m.theme
		return true, dirty
	case session.ItemThinking:
		t, ok := w.(*block.ThinkingBlock)
		if !ok {
			return false, false
		}
		prevExp := t.Expanded
		dirty = t.Text != it.Thinking || t.Streaming != it.Streaming ||
			t.Interrupted != it.Interrupted || t.Duration != it.ThinkingDuration ||
			t.Model != it.TurnMeta.Model
		t.Text = it.Thinking
		t.Streaming = it.Streaming
		t.Interrupted = it.Interrupted
		t.Duration = it.ThinkingDuration
		t.Model = it.TurnMeta.Model
		t.Theme = m.theme
		t.Spinner = m.spinner
		if exp, ok := m.expanded[it.ID]; ok {
			t.Expanded = exp
		}
		if t.Expanded != prevExp {
			dirty = true
		}
		return true, dirty
	case session.ItemCompaction:
		c, ok := w.(*block.CompactionBlock)
		if !ok {
			return false, false
		}
		prevExp := c.Expanded
		dirty = c.Text != it.Text || c.Summary != it.Summary
		c.Text = it.Text
		c.Summary = it.Summary
		c.Theme = m.theme
		if exp, ok := m.expanded[it.ID]; ok {
			c.Expanded = exp
		}
		if c.Expanded != prevExp {
			dirty = true
		}
		return true, dirty
	case session.ItemTool:
		return m.patchTool(w, it)
	}
	return false, false
}

func (m *Mapper) patchTool(w components.Widget, it session.Item) (ok, dirty bool) {
	name := strings.ToLower(it.ToolName)
	if name == "bash" {
		b, ok := w.(*block.BashBlock)
		if !ok {
			return false, false
		}
		cmd := it.ToolInput
		if it.ToolRun.Detail != "" {
			cmd = it.ToolRun.Detail
		}
		st := bashStatus(it.ToolRun.Status)
		prevExp := b.Expanded
		dirty = b.Command != cmd || b.Output != it.ToolRun.Output || b.Status != st || b.ExitCode != it.ToolRun.ExitCode
		b.Command = cmd
		b.Output = it.ToolRun.Output
		b.Status = st
		b.ExitCode = it.ToolRun.ExitCode
		b.Theme = m.theme
		if exp, ok := m.expanded[it.ID]; ok {
			b.Expanded = exp
		} else if it.ToolRun.Local {
			// User "!cmd" results should stay open so output is visible.
			b.Expanded = true
		} else if b.Status == block.BashRunning && b.Output != "" {
			b.Expanded = true
		}
		if b.Expanded != prevExp {
			dirty = true
		}
		return true, dirty
	}
	if isDiffTool(name) {
		d, ok := w.(*block.DiffBlock)
		if !ok {
			return false, false
		}
		prev := diffHeightSnap{
			Name:     d.Name,
			Path:     d.Path,
			Diff:     d.Diff,
			Error:    d.Error,
			Status:   d.Status,
			Expanded: d.Expanded,
		}
		m.fillDiffBlock(d, it)
		dirty = prev != diffHeightSnap{
			Name:     d.Name,
			Path:     d.Path,
			Diff:     d.Diff,
			Error:    d.Error,
			Status:   d.Status,
			Expanded: d.Expanded,
		}
		return true, dirty
	}
	if isAgentTreeTool(name) {
		a, ok := w.(*block.AgentBlock)
		if !ok {
			return false, false
		}
		prev := agentHeightSnap{
			Name:     a.Name,
			Detail:   a.Detail,
			Status:   a.Status,
			Error:    a.Error,
			Summary:  a.Summary,
			Expanded: a.Expanded,
			Children: a.Children,
		}
		m.fillAgentBlock(a, it)
		dirty = prev.Name != a.Name || prev.Detail != a.Detail || prev.Status != a.Status ||
			prev.Error != a.Error || prev.Summary != a.Summary || prev.Expanded != a.Expanded ||
			!childToolsEqual(prev.Children, a.Children)
		return true, dirty
	}
	t, ok := w.(*block.ToolBlock)
	if !ok {
		return false, false
	}
	detail := it.ToolInput
	if it.ToolRun.Detail != "" {
		detail = it.ToolRun.Detail
	}
	st := uiToolStatus(it.ToolRun.Status)
	prevExp := t.Expanded
	dirty = t.Name != it.ToolName || t.Detail != detail || t.Output != it.ToolRun.Output ||
		t.Error != it.ToolRun.Error || t.Status != st
	t.Name = it.ToolName
	t.Detail = detail
	t.Output = it.ToolRun.Output
	t.Error = it.ToolRun.Error
	t.Status = st
	t.Theme = m.theme
	t.Spinner = m.spinner
	if exp, ok := m.expanded[it.ID]; ok {
		t.Expanded = exp
	} else if t.Status == status.ToolRunning && t.Output != "" {
		t.Expanded = true
	}
	if t.Expanded != prevExp {
		dirty = true
	}
	return true, dirty
}

type agentHeightSnap struct {
	Name     string
	Detail   string
	Status   status.ToolStatus
	Error    string
	Summary  string
	Expanded bool
	Children []block.ChildTool
}

func childToolsEqual(a, b []block.ChildTool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (m *Mapper) widgetFor(it session.Item) components.Widget {
	exp := m.expanded[it.ID]
	id := it.ID
	switch it.Kind {
	case itemTurnSummary:
		st := m.summaries[id]
		return &block.TurnSummaryBlock{
			Duration: st.Duration,
			Tools:    st.Tools,
			Failed:   st.Failed,
			Rows:     st.Rows,
			Files:    st.Files,
			Expanded: exp,
			Theme:    m.theme,
			OnToggle: func(expanded bool) {
				m.expanded[id] = expanded
				if m.onRegroup != nil {
					m.onRegroup()
				} else if m.onInvalidate != nil {
					m.onInvalidate()
				}
			},
		}
	case session.ItemUser:
		return &block.UserBlock{Text: it.Text, Queued: it.Queued, Theme: m.theme}
	case session.ItemThinking:
		return &block.ThinkingBlock{
			Text:        it.Thinking,
			Streaming:   it.Streaming,
			Interrupted: it.Interrupted,
			Duration:    it.ThinkingDuration,
			Model:       it.TurnMeta.Model,
			// Collapsed by default — streaming included: the header spinner
			// is the activity signal, the body appears only on user toggle.
			Expanded: exp,
			Theme:    m.theme,
			Spinner:  m.spinner,
			OnToggle: func(expanded bool) {
				m.expanded[id] = expanded
				if m.onInvalidate != nil {
					m.onInvalidate()
				}
			},
		}
	case session.ItemCompaction:
		return &block.CompactionBlock{
			Text:     it.Text,
			Summary:  it.Summary,
			Expanded: exp,
			Theme:    m.theme,
			OnToggle: func(expanded bool) {
				m.expanded[id] = expanded
				if m.onInvalidate != nil {
					m.onInvalidate()
				}
			},
		}
	case session.ItemTool:
		return m.toolWidget(it, exp)
	default:
		label, tail := formatItemMeta(it)
		return &block.AssistantBlock{
			Text:      it.Text,
			State:     it.State,
			MetaLabel: label,
			MetaTail:  tail,
			Theme:     m.theme,
		}
	}
}

func (m *Mapper) toolWidget(it session.Item, exp bool) components.Widget {
	detail := it.ToolInput
	if it.ToolRun.Detail != "" {
		detail = it.ToolRun.Detail
	}
	autoExp := exp
	if !exp {
		if it.ToolRun.Local {
			autoExp = true
		} else if it.ToolRun.Status == session.ToolInProgress && it.ToolRun.Output != "" {
			autoExp = true
		}
	}
	id := it.ID
	if strings.EqualFold(it.ToolName, "bash") {
		return &block.BashBlock{
			Command:  detail,
			Output:   it.ToolRun.Output,
			Status:   bashStatus(it.ToolRun.Status),
			ExitCode: it.ToolRun.ExitCode,
			Expanded: autoExp,
			Theme:    m.theme,
			OnToggle: func(expanded bool) {
				m.expanded[id] = expanded
				if m.onInvalidate != nil {
					m.onInvalidate()
				}
			},
		}
	}
	if isDiffTool(it.ToolName) {
		d := &block.DiffBlock{
			Theme:   m.theme,
			Spinner: m.spinner,
			OnToggle: func(expanded bool) {
				m.expanded[id] = expanded
				if m.onInvalidate != nil {
					m.onInvalidate()
				}
			},
		}
		m.fillDiffBlock(d, it)
		return d
	}
	if isAgentTreeTool(it.ToolName) {
		a := &block.AgentBlock{
			Theme:   m.theme,
			Spinner: m.spinner,
			OnToggle: func(expanded bool) {
				m.expanded[id] = expanded
				if m.onInvalidate != nil {
					m.onInvalidate()
				}
			},
		}
		m.fillAgentBlock(a, it)
		return a
	}
	return &block.ToolBlock{
		Name:     it.ToolName,
		Detail:   detail,
		Output:   it.ToolRun.Output,
		Error:    it.ToolRun.Error,
		Status:   uiToolStatus(it.ToolRun.Status),
		Expanded: autoExp,
		Theme:    m.theme,
		Spinner:  m.spinner,
		OnToggle: func(expanded bool) {
			m.expanded[id] = expanded
			if m.onInvalidate != nil {
				m.onInvalidate()
			}
		},
	}
}

func isAgentTreeTool(name string) bool {
	switch strings.ToLower(name) {
	case "agent_spawn", "agent_wait":
		return true
	default:
		return false
	}
}

// isDiffTool reports a file-changing tool rendered as a diff card.
func isDiffTool(name string) bool {
	switch strings.ToLower(name) {
	case "edit", "write":
		return true
	default:
		return false
	}
}

// diffHeightSnap captures the DiffBlock fields whose change moves its height.
type diffHeightSnap struct {
	Name     string
	Path     string
	Diff     string
	Error    string
	Status   status.ToolStatus
	Expanded bool
}

func (m *Mapper) fillDiffBlock(d *block.DiffBlock, it session.Item) {
	d.Name = strings.ToLower(it.ToolName)
	path := it.ToolRun.Detail
	if path == "" {
		path = it.ToolInput
	}
	d.Path = path
	d.Diff = it.ToolRun.Output
	d.Error = it.ToolRun.Error
	d.Status = uiToolStatus(it.ToolRun.Status)
	d.Theme = m.theme
	d.Spinner = m.spinner
	if exp, ok := m.expanded[it.ID]; ok {
		d.Expanded = exp
		return
	}
	// The card's default follows the sidebar's expand-edits switch. The
	// explicit per-row toggle honored above outlives it either way.
	d.Expanded = m.expandEdits && strings.TrimSpace(d.Diff) != ""
}

// groupTurns condenses finished turns older than the trailing keepFullTurns
// into a summary row: the user prompt and the turn's final reply stay, the
// working rows between them fold behind a "worked 42s · 7 tools · …" line.
// Failed tool rows, queued prompts and compaction markers never fold; the
// verbose switch turns grouping off wholesale.
func (m *Mapper) groupTurns(items []session.Item, snap session.Snapshot) []session.Item {
	if m.summaries == nil {
		m.summaries = make(map[string]turnStats)
	}
	clear(m.summaries)
	if m.verbose {
		return items
	}
	var starts []int
	for i, it := range items {
		if it.Kind == session.ItemUser && !it.Queued {
			starts = append(starts, i)
		}
	}
	if len(starts) <= keepFullTurns {
		return items
	}
	durations := turnDurations(snap)
	out := make([]session.Item, 0, len(items))
	out = append(out, items[:starts[0]]...)
	for t, start := range starts {
		end := len(items)
		if t+1 < len(starts) {
			end = starts[t+1]
		}
		if t >= len(starts)-keepFullTurns {
			out = append(out, items[start:end]...)
			continue
		}
		out = append(out, m.condenseTurn(items[start:end], durations[items[start].ID])...)
	}
	return out
}

// condenseTurn folds one finished turn's working rows behind a summary row.
// turn[0] is the opening user prompt; the trailing run of assistant text is
// the turn's answer and stays out of the fold too.
func (m *Mapper) condenseTurn(turn []session.Item, dur time.Duration) []session.Item {
	tail := len(turn)
	for tail > 1 && turn[tail-1].Kind == session.ItemAssistant {
		tail--
	}
	work := turn[1:tail]
	if len(work) == 0 {
		return turn
	}
	id := "turnsum-" + turn[0].ID
	st := turnStats{Duration: dur, Rows: len(work)}
	for _, it := range work {
		if it.Kind != session.ItemTool {
			continue
		}
		st.Tools++
		if failedToolRun(it.ToolRun.Status) {
			st.Failed++
		}
		if isDiffTool(it.ToolName) && it.ToolRun.Detail != "" {
			name := filepath.Base(it.ToolRun.Detail)
			if !slices.Contains(st.Files, name) {
				st.Files = append(st.Files, name)
			}
		}
	}
	m.summaries[id] = st

	out := make([]session.Item, 0, len(turn)+1)
	out = append(out, turn[0], session.Item{ID: id, Kind: itemTurnSummary})
	expanded := m.expanded[id]
	for _, it := range work {
		if expanded || keepVisible(it) {
			out = append(out, it)
		}
	}
	return append(out, turn[tail:]...)
}

// keepVisible reports a row a condensed turn may never hide: a failed or
// rejected tool call, a queued user prompt, a compaction marker.
func keepVisible(it session.Item) bool {
	switch it.Kind {
	case session.ItemUser, session.ItemCompaction:
		return true
	case session.ItemTool:
		return failedToolRun(it.ToolRun.Status)
	default:
		return false
	}
}

func failedToolRun(s session.ToolStatus) bool {
	return s == session.ToolError || s == session.ToolRejected
}

// dropServiceRefusals removes skill-preload refusal rows from the projection.
// Such a refusal is delivery choreography, not an outcome: the model retries
// the same call at once, the executed action already left its "⚙ plan" row,
// and rendering the refusal as a rejected tool would alarm the reader with a
// failure that never happened. Filtering here keeps every consumer honest —
// grouping never counts the row as failed, and no turn pins it visible.
func dropServiceRefusals(items []session.Item) []session.Item {
	kept := items[:0:0]
	for _, it := range items {
		if it.Kind == session.ItemTool && plangate.IsSkillPreloadRefusal(it.ToolRun) {
			continue
		}
		kept = append(kept, it)
	}
	return kept
}

// turnDurations sums each turn's assistant round durations, keyed by the id
// of the user message that opened the turn.
func turnDurations(snap session.Snapshot) map[string]time.Duration {
	out := make(map[string]time.Duration)
	current := ""
	for _, msg := range snap.Messages {
		switch {
		case msg.Role == session.RoleUser && !msg.Queued:
			current = msg.ID
		case msg.Role == session.RoleAssistant && current != "":
			out[current] += msg.TurnDuration()
		}
	}
	return out
}

func (m *Mapper) fillAgentBlock(a *block.AgentBlock, it session.Item) {
	detail := it.ToolInput
	if it.ToolRun.Detail != "" {
		detail = it.ToolRun.Detail
	}
	a.Name = it.ToolName
	a.Detail = detail
	a.Status = uiToolStatus(it.ToolRun.Status)
	a.Theme = m.theme
	a.Spinner = m.spinner
	a.Error = it.ToolRun.Error

	parsed := tools.ParseAgentResult(it.ToolRun.Output)
	// agent_spawn names the model the child will run — only a resolved pin;
	// an inheriting child stays unmarked.
	if strings.EqualFold(it.ToolName, "agent_spawn") && parsed.OK && parsed.Model != "" &&
		parsed.Model != tools.InheritModel {
		a.Detail = strings.TrimSpace(a.Detail + " · " + parsed.Model)
	}
	if sum := parsed.RenderableSummary(); sum != "" {
		a.Summary = sum
	} else {
		a.Summary = ""
	}

	// agent_wait: summary only — the live tree already lives on agent_spawn.
	// agent_spawn: nested child tools from SubagentStore.
	a.Children = nil
	if !strings.EqualFold(it.ToolName, "agent_wait") && m.Children != nil {
		a.Children = m.Children(it.ToolUseID)
		if len(a.Children) == 0 && parsed.JobID != "" && m.ChildrenByJob != nil {
			a.Children = m.ChildrenByJob(parsed.JobID)
		}
	}

	if exp, ok := m.expanded[it.ID]; ok {
		a.Expanded = exp
	} else if a.Status == status.ToolRunning || len(a.Children) > 0 || a.Summary != "" {
		a.Expanded = true
	}
}

func bashStatus(s session.ToolStatus) block.BashStatus {
	switch s {
	case session.ToolDone:
		return block.BashDone
	case session.ToolError:
		return block.BashError
	case session.ToolCancelled:
		return block.BashCancelled
	case session.ToolRejected:
		return block.BashRejected
	default:
		return block.BashRunning
	}
}

func uiToolStatus(s session.ToolStatus) status.ToolStatus {
	switch s {
	case session.ToolDone:
		return status.ToolDone
	case session.ToolError:
		return status.ToolError
	case session.ToolCancelled:
		return status.ToolCancelled
	case session.ToolRejected:
		return status.ToolRejected
	case session.ToolQueued:
		return status.ToolQueued
	default:
		return status.ToolRunning
	}
}
