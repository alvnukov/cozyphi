package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/components/toast"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	uitranscript "github.com/pulseaiclub/phi/internal/tui/transcript"
)

// SessionActions owns /sessions, /resume, and /clear UI side effects.
type SessionActions struct {
	e *Editor
}

// Register adds the session slash commands onto r.
func (s *SessionActions) Register(r *CommandRegistry) {
	if s == nil || r == nil {
		return
	}
	r.Register(Command{
		Name:        "sessions",
		Description: "List sessions for this directory",
		Slash:       true,
		Insert:      "/sessions",
		Run: func(CommandContext) error {
			s.Show()
			return nil
		},
	})
	r.Register(Command{
		Name:        "resume",
		Description: "Resume a session in this directory — /resume <id>",
		Slash:       true,
		Insert:      "/resume ",
		Run: func(ctx CommandContext) error {
			if len(ctx.Args) < 1 {
				ctx.toast("Usage: /resume <session-id>", toast.ToastWarning, 3*time.Second)
				return nil
			}
			s.Resume(ctx.Args[0])
			return nil
		},
	})
	r.Register(Command{
		Name:        "clear",
		Description: "Start a new empty session",
		Slash:       true,
		Insert:      "/clear",
		Run: func(CommandContext) error {
			e := s.e
			if e != nil && e.streamActive() {
				e.toast.Show("Cannot clear while a reply or command is running", toast.ToastWarning, 3*time.Second)
				return nil
			}
			s.Clear()
			return nil
		},
	})
}

// Show lists recent sessions for the current session directory.
func (s *SessionActions) Show() {
	e := s.e
	dir := ""
	if e.ctrl != nil {
		dir = e.ctrl.SessionDir()
	}
	list, err := session.ListSessions(dir)
	if err != nil {
		e.toast.Show(err.Error(), toast.ToastError, 3*time.Second)
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
	e.applySessionEvent(session.AssistantMessageUpdate{Message: session.Message{
		ID:    fmt.Sprintf("sessions-%d", time.Now().UnixNano()),
		State: session.StateComplete,
		Text:  b.String(),
		Content: []session.ContentBlock{
			{Type: session.BlockText, Text: b.String()},
		},
	}})
	e.syncThread()
	e.list.StickToBottom()
}

// Resume loads a prior session by id into the UI.
func (s *SessionActions) Resume(id string) {
	e := s.e
	warn, err := e.ctrl.Resume(id)
	if err != nil {
		e.toast.Show(err.Error(), toast.ToastError, 4*time.Second)
		return
	}
	if e.hookCmds != nil {
		e.hookCmds.Sync()
	}
	e.snap = e.ctrl.ReplaySnapshot()
	e.list.Entries = nil
	e.listIDs = nil
	e.list.InvalidateHeights()
	e.syncThread()
	e.list.StickToBottom()
	msg := "Resumed " + shortSessionID(e.ctrl.SessionID())
	if warn != "" {
		e.toast.Show(msg+": "+warn, toast.ToastWarning, 5*time.Second)
		return
	}
	e.toast.Show(msg, toast.ToastSuccess, 3*time.Second)
}

// Clear starts a new empty session. Caller must ensure the stream is idle
// (see Editor.streamActive / commandContext ClearSession).
func (s *SessionActions) Clear() {
	e := s.e
	if err := e.ctrl.Clear(); err != nil {
		e.toast.Show(err.Error(), toast.ToastError, 4*time.Second)
		return
	}
	e.snap = e.ctrl.ReplaySnapshot()
	e.list.Entries = nil
	e.listIDs = nil
	e.list.InvalidateHeights()
	e.subagents = uitranscript.NewSubagentStore()
	if e.mapper != nil {
		e.mapper.Children = e.subagents.Children
		e.mapper.ChildrenByJob = e.subagents.ChildrenByJob
	}
	e.lastUsage = session.TokenUsage{}
	e.Chat.BottomLeftLabel = layout.BorderLabel{}
	e.activity.Apply(controller.ActivityIdle)
	e.syncThread()
	e.list.StickToBottom()
	e.toast.Show("Cleared "+shortSessionID(e.ctrl.SessionID()), toast.ToastSuccess, 3*time.Second)
}

func shortSessionID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
