package sessions

import (
	"fmt"

	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
	"github.com/alvnukov/cozyphi/internal/version"
)

// ConfigureStatusTabs connects the dashboard's stable string tab IDs to preferences.
// With no callback (or an invalid choice), the dashboard opens Usage.
func (e *View) ConfigureStatusTabs(choose func() string, closed func(string)) {
	e.status.ConfigureTabs(choose, func(tab string) {
		e.statusHistory.Cancel()
		if closed != nil {
			closed(tab)
		}
	})
}

// ConfigureStatusHistory connects an asynchronous loader (0, 7, or 30 days).
func (e *View) ConfigureStatusHistory(request func(int)) { e.status.ConfigureHistory(request) }

// ApplyStatusHistory must run on the UI goroutine; callers also request a redraw.
func (e *View) ApplyStatusHistory(h statuspane.History) { e.status.ApplyHistory(h) }

// ShowStatus opens the dashboard with allowlisted, detached runtime metadata.
func (e *View) ShowStatus() {
	if e.status == nil {
		return
	}
	e.composer.HideCompleters()
	e.composer.HidePalette()
	s := statuspane.Snapshot{Version: version.Version, CWD: e.cwd}
	if e.statusStore != nil {
		snap := e.statusStore.Snapshot()
		s.ConfigRows = statusSettingsRows(snap)
		if snap.Path != "" {
			s.ConfigSources = append(s.ConfigSources, "Harness settings: "+snap.Path)
		}
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
		s.ConfigRows = append(s.ConfigRows,
			fmt.Sprintf("Effective agent mode: %s", e.ctrl.Mode()),
			fmt.Sprintf("Effective sub-agents enabled: %t", e.ctrl.AgentsEnabled()),
			fmt.Sprintf("Effective task access: %s", e.ctrl.TasksAccess()))
	}
	e.status.Show(s)
	e.FocusEditor()
}
