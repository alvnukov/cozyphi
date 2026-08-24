package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/pulseaiclub/phi/internal/components/toast"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	"github.com/pulseaiclub/phi/internal/tui/footer"
	"github.com/pulseaiclub/phi/internal/tui/sidebar"
	"github.com/pulseaiclub/phi/internal/tui/transcript"
)

// SessionCommands owns /sessions, /resume, and /clear UI side effects.
type SessionCommands struct {
	Ctrl       *controller.Controller
	Transcript *transcript.TranscriptPane
	Footer     *footer.FooterChrome
	Sidebar    *sidebar.Sidebar
	Toast      toast.Toast
	SyncHooks  func()
}

// NewSessionCommands builds session command handlers.
func NewSessionCommands(
	ctrl *controller.Controller,
	transcript *transcript.TranscriptPane,
	footer *footer.FooterChrome,
	side *sidebar.Sidebar,
	toast toast.Toast,
	syncHooks func(),
) *SessionCommands {
	return &SessionCommands{
		Ctrl:       ctrl,
		Transcript: transcript,
		Footer:     footer,
		Sidebar:    side,
		Toast:      toast,
		SyncHooks:  syncHooks,
	}
}

// Show lists recent sessions for the current session directory.
func (s *SessionCommands) Show() {
	if s == nil {
		return
	}
	dir := ""
	if s.Ctrl != nil {
		dir = s.Ctrl.SessionDir()
	}
	list, err := session.ListSessions(dir)
	if err != nil {
		s.Toast.Show(err.Error(), toast.ToastError, 3*time.Second)
		return
	}
	const maxN = 12
	var b strings.Builder
	if len(list) == 0 {
		b.WriteString("No sessions for this directory")
	} else {
		fmt.Fprintf(&b, "Sessions in this directory (%d):\n", len(list))
		n := len(list)
		n = min(n, maxN)
		for i := 0; i < n; i++ {
			m := list[i]
			short := m.ID
			if len(short) > 8 {
				short = short[:8]
			}
			preview := m.Preview
			if preview == "" {
				preview = "(no preview)"
			}
			fmt.Fprintf(&b, "  %s  %s  %s\n", short, m.Mtime.Format("01-02 15:04"), preview)
		}
		b.WriteString("Resume with /resume <id>")
	}
	s.Transcript.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    fmt.Sprintf("sessions-%d", time.Now().UnixNano()),
		State: session.StateComplete,
		Text:  b.String(),
		Content: []session.ContentBlock{
			{Type: session.BlockText, Text: b.String()},
		},
	}})
	s.Transcript.Sync()
	s.Transcript.StickToBottom()
}

// Resume loads a prior session by id into the UI.
func (s *SessionCommands) Resume(id string) {
	if s == nil {
		return
	}
	warn, err := s.Ctrl.Resume(id)
	if err != nil {
		s.Toast.Show(err.Error(), toast.ToastError, 4*time.Second)
		return
	}
	if s.SyncHooks != nil {
		s.SyncHooks()
	}
	s.Transcript.LoadReplay(s.Ctrl.ReplaySnapshot())
	if s.Sidebar != nil {
		s.Sidebar.SetPlan(s.Ctrl.Plan())
	}
	s.Transcript.Sync()
	s.Transcript.StickToBottom()
	msg := "Resumed " + session.ShortID(s.Ctrl.SessionID())
	if warn != "" {
		s.Toast.Show(msg+": "+warn, toast.ToastWarning, 5*time.Second)
		return
	}
	s.Toast.Show(msg, toast.ToastSuccess, 3*time.Second)
}

// Clear starts a new empty session. Caller must ensure the stream is idle
// (see Submitter.StreamActive / CommandBridge ClearSession).
func (s *SessionCommands) Clear() {
	if s == nil {
		return
	}
	if err := s.Ctrl.Clear(); err != nil {
		s.Toast.Show(err.Error(), toast.ToastError, 4*time.Second)
		return
	}
	s.Transcript.LoadReplay(s.Ctrl.ReplaySnapshot())
	s.Transcript.ResetSubagents()
	s.Footer.ClearTokenDisplay()
	if s.Sidebar != nil {
		s.Sidebar.ClearUsage()
		s.Sidebar.SetPlan(s.Ctrl.Plan())
	}
	s.Footer.Activity().Apply(controller.ActivityIdle)
	s.Transcript.Sync()
	s.Transcript.StickToBottom()
	s.Toast.Show("Cleared "+session.ShortID(s.Ctrl.SessionID()), toast.ToastSuccess, 3*time.Second)
}
