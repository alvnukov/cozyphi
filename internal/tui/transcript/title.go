package transcript

import (
	"strings"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/session"
)

// Naming is metadata, not a collapsible tool-output document. Never show raw
// arguments while pending; rejected input may contain terminal controls.
func titleDetail(it session.Item) string {
	if it.ToolRun.Error != "" {
		return strings.Join(strings.Fields(it.ToolRun.Error), " ")
	}
	if it.ToolRun.Status == session.ToolDone && it.ToolRun.Detail != "" {
		return it.ToolRun.Detail
	}
	return "set_title"
}

func (m *Mapper) titleWidget(it session.Item) *block.ToolBlock {
	return &block.ToolBlock{
		Name: "Title", Detail: titleDetail(it), Status: uiToolStatus(it.ToolRun.Status),
		Theme: m.theme, Spinner: m.spinner,
	}
}

func (m *Mapper) patchTitle(w components.Widget, it session.Item) (bool, bool) {
	b, ok := w.(*block.ToolBlock)
	if !ok {
		return false, false
	}
	next := m.titleWidget(it)
	dirty := b.Name != next.Name || b.Detail != next.Detail || b.Status != next.Status
	*b = *next
	return true, dirty
}
