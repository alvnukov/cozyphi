package editor

import (
	"fmt"

	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
	"github.com/alvnukov/cozyphi/internal/version"
)

// ConfigureStatusTabs connects the dashboard's stable string tab IDs to preferences.
// With no callback (or an invalid choice), the dashboard opens Usage.
func (e *Editor) ConfigureStatusTabs(choose func() string, closed func(string)) {
	e.status.ConfigureTabs(choose, func(tab string) {
		e.statusHistory.Cancel()
		if closed != nil {
			closed(tab)
		}
	})
}

// ConfigureStatusHistory connects an asynchronous loader (0, 7, or 30 days).
func (e *Editor) ConfigureStatusHistory(request func(int)) { e.status.ConfigureHistory(request) }

// ApplyStatusHistory must run on the UI goroutine; callers also request a redraw.
func (e *Editor) ApplyStatusHistory(h statuspane.History) { e.status.ApplyHistory(h) }

// ShowStatus opens the dashboard with allowlisted, detached runtime metadata.
func (e *Editor) ShowStatus() {
	if e.status == nil {
		return
	}
	e.composer.HideCompleters()
	e.composer.HidePalette()
	s := statuspane.Snapshot{Version: version.Version, CWD: e.cwd}
	if e.statusConfigPath != "" {
		s.ConfigSources = append(s.ConfigSources, "Harness settings: "+e.statusConfigPath)
	}
	if e.ctrl != nil {
		s.ConfigSources = append(s.ConfigSources, e.ctrl.StatusConfigSources()...)
		s.Session = e.ctrl.SessionID()
		stats := e.ctrl.SessionStats()
		s.Model = stats.Model
		s.Provider = stats.ProviderID
		for _, server := range e.ctrl.MCPStatuses() {
			s.MCP = append(s.MCP, fmt.Sprintf("  %s: %s", server.Name, server.State))
		}
		if e.settings != nil {
			e.settings.SetAvailableTools(e.ctrl.ToolNames())
			e.settings.SetModelNames(e.modelNames)
		}
	}
	e.status.Show(s)
	e.FocusEditor()
}
