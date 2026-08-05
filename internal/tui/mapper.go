package tui

import (
	"strings"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/block"
	"github.com/pulseaiclub/phi/internal/components/status"
	"github.com/pulseaiclub/phi/internal/session"
)

// Mapper converts session.Snapshot items into transcript widgets.
// It owns expand-state and has no dependency on Editor / xui / agent.
type Mapper struct {
	theme        components.Theme
	spinner      *status.Spinner
	expanded     map[string]bool
	onInvalidate func() // e.g. MessageList.InvalidateHeights
}

func NewMapper(theme components.Theme, spinner *status.Spinner, onInvalidate func()) *Mapper {
	return &Mapper{
		theme:        theme,
		spinner:      spinner,
		expanded:     make(map[string]bool),
		onInvalidate: onInvalidate,
	}
}

// Sync rebuilds the widget list from snap, reusing widgets when patchable.
func (m *Mapper) Sync(entries []components.Widget, listIDs []string, snap session.Snapshot) (newEntries []components.Widget, newIDs []string) {
	items := session.Project(snap)
	n := len(items)
	byID := make(map[string]int, len(entries))
	for i, w := range entries {
		id := entryID(listIDs, i)
		if id == "" {
			continue
		}
		byID[id] = i
		switch b := w.(type) {
		case *block.ThinkingBlock:
			m.expanded[id] = b.Expanded
		case *block.ToolBlock:
			m.expanded[id] = b.Expanded
		case *block.BashBlock:
			m.expanded[id] = b.Expanded
		}
	}

	newEntries = make([]components.Widget, 0, n)
	newIDs = make([]string, 0, n)
	for _, it := range items {
		newIDs = append(newIDs, it.ID)
		if oldIdx, ok := byID[it.ID]; ok {
			if m.patchItem(entries[oldIdx], it) {
				newEntries = append(newEntries, entries[oldIdx])
				continue
			}
		}
		newEntries = append(newEntries, m.widgetFor(it))
	}
	return newEntries, newIDs
}

func entryID(listIDs []string, i int) string {
	if i >= 0 && i < len(listIDs) {
		return listIDs[i]
	}
	return ""
}

func (m *Mapper) patchItem(w components.Widget, it session.Item) bool {
	switch it.Kind {
	case session.ItemUser:
		u, ok := w.(*block.UserBlock)
		if !ok {
			return false
		}
		u.Text = it.Text
		u.Theme = m.theme
		return true
	case session.ItemAssistant:
		a, ok := w.(*block.AssistantBlock)
		if !ok {
			return false
		}
		a.Text = it.Text
		a.State = it.State
		a.Theme = m.theme
		return true
	case session.ItemThinking:
		t, ok := w.(*block.ThinkingBlock)
		if !ok {
			return false
		}
		t.Text = it.Thinking
		t.Streaming = it.Streaming
		t.Interrupted = it.Interrupted
		t.Theme = m.theme
		t.Spinner = m.spinner
		if exp, ok := m.expanded[it.ID]; ok {
			t.Expanded = exp
		}
		return true
	case session.ItemCompaction:
		c, ok := w.(*block.CompactionBlock)
		if !ok {
			return false
		}
		c.Theme = m.theme
		return true
	case session.ItemTool:
		return m.patchTool(w, it)
	}
	return false
}

func (m *Mapper) patchTool(w components.Widget, it session.Item) bool {
	name := strings.ToLower(it.ToolName)
	if name == "bash" {
		b, ok := w.(*block.BashBlock)
		if !ok {
			return false
		}
		b.Command = it.ToolInput
		if it.ToolRun.Detail != "" {
			b.Command = it.ToolRun.Detail
		}
		b.Output = it.ToolRun.Output
		b.Status = bashStatus(it.ToolRun.Status)
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
		return true
	}
	t, ok := w.(*block.ToolBlock)
	if !ok {
		return false
	}
	t.Name = it.ToolName
	t.Detail = it.ToolInput
	if it.ToolRun.Detail != "" {
		t.Detail = it.ToolRun.Detail
	}
	t.Output = it.ToolRun.Output
	t.Error = it.ToolRun.Error
	t.Status = uiToolStatus(it.ToolRun.Status)
	t.Theme = m.theme
	t.Spinner = m.spinner
	if exp, ok := m.expanded[it.ID]; ok {
		t.Expanded = exp
	} else if t.Status == status.ToolRunning && t.Output != "" {
		t.Expanded = true
	}
	return true
}

func (m *Mapper) widgetFor(it session.Item) components.Widget {
	exp := m.expanded[it.ID]
	id := it.ID
	switch it.Kind {
	case session.ItemUser:
		return &block.UserBlock{Text: it.Text, Theme: m.theme}
	case session.ItemThinking:
		return &block.ThinkingBlock{
			Text:        it.Thinking,
			Streaming:   it.Streaming,
			Interrupted: it.Interrupted,
			Expanded:    exp || it.Streaming,
			Theme:       m.theme,
			Spinner:     m.spinner,
			OnToggle: func(expanded bool) {
				m.expanded[id] = expanded
				if m.onInvalidate != nil {
					m.onInvalidate()
				}
			},
		}
	case session.ItemCompaction:
		return &block.CompactionBlock{Theme: m.theme}
	case session.ItemTool:
		return m.toolWidget(it, exp)
	default:
		return &block.AssistantBlock{Text: it.Text, State: it.State, Theme: m.theme}
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
