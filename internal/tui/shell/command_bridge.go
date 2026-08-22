package shell

import (
	"time"

	"github.com/pulseaiclub/phi/internal/components/palette"
	"github.com/pulseaiclub/phi/internal/components/toast"
	"github.com/pulseaiclub/phi/internal/tui/commands"
	"github.com/pulseaiclub/phi/internal/tui/composer"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	"github.com/pulseaiclub/phi/internal/tui/submit"
	"github.com/pulseaiclub/phi/internal/tui/transcript"
)

// CommandBridge builds CommandContext for slash/palette dispatch without a Shell back-pointer.
type CommandBridge struct {
	toast      toast.Toast
	composer   *composer.ComposerPane
	transcript *transcript.TranscriptPane
	ctrl       *controller.Controller
	submitter  *submit.Submitter
	sessions   *commands.SessionCommands

	reloadHooks     func()
	listHooks       func() []palette.PaletteCommand
	setModel        func(string)
	applyTheme      func(string)
	setPermissions  func(bool)
	setAgents       func(bool)
	addSkill        func(string)
	copyLastMessage func()

	modelNames []string
	skillPath  string
}

// CommandBridgeDeps wires collaborators for CommandBridge.
type CommandBridgeDeps struct {
	Toast      toast.Toast
	Composer   *composer.ComposerPane
	Transcript *transcript.TranscriptPane
	Ctrl       *controller.Controller
	Submitter  *submit.Submitter
	Sessions   *commands.SessionCommands

	ReloadHooks     func()
	ListHooks       func() []palette.PaletteCommand
	SetModel        func(string)
	ApplyTheme      func(string)
	SetPermissions  func(bool)
	SetAgents       func(bool)
	AddSkill        func(string)
	CopyLastMessage func()
	ModelNames      []string
	SkillPath       string
}

// NewCommandBridge returns a bridge for registry slash/palette dispatch.
func NewCommandBridge(d CommandBridgeDeps) *CommandBridge {
	return &CommandBridge{
		toast:           d.Toast,
		composer:        d.Composer,
		transcript:      d.Transcript,
		ctrl:            d.Ctrl,
		submitter:       d.Submitter,
		sessions:        d.Sessions,
		reloadHooks:     d.ReloadHooks,
		listHooks:       d.ListHooks,
		setModel:        d.SetModel,
		applyTheme:      d.ApplyTheme,
		setPermissions:  d.SetPermissions,
		setAgents:       d.SetAgents,
		addSkill:        d.AddSkill,
		copyLastMessage: d.CopyLastMessage,
		modelNames:      append([]string(nil), d.ModelNames...),
		skillPath:       d.SkillPath,
	}
}

// Context returns the capability surface for command Run / palette builders.
func (b *CommandBridge) Context() commands.CommandContext {
	if b == nil {
		return commands.CommandContext{}
	}
	return commands.CommandContext{
		Toast: func(msg string, kind toast.ToastKind, d time.Duration) {
			b.toast.Show(msg, kind, d)
		},
		PushSubmenu: func(title string, cmds []palette.PaletteCommand) {
			b.composer.PushPalette(title, cmds)
		},
		ShowSessions:  b.sessions.Show,
		ResumeSession: b.sessions.Resume,
		ClearSession: func() {
			if b.submitter != nil && b.submitter.StreamActive() {
				b.toast.Show("Cannot clear while a reply or command is running", toast.ToastWarning, 3*time.Second)
				return
			}
			b.sessions.Clear()
		},
		SetModel:        b.setModel,
		ApplyTheme:      b.applyTheme,
		SetPermissions:  b.setPermissions,
		SetAgents:       b.setAgents,
		ReloadHooks:     b.reloadHooks,
		ListHooks:       b.listHooks,
		AddSkill:        b.addSkill,
		CopyLastMessage: b.copyLastMessage,
		ModelNames:      b.modelNames,
		SkillPath:       b.skillPath,
	}
}
