package sessions

import (
	"slices"
	"time"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/shelltask"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/tui/shellpane"
)

func (e *View) initShellTasks(theme components.Theme) {
	e.shells = shellpane.New(theme, e.ctrl.BackgroundShellTask, e.ctrl.StopShellTask,
		func() { e.composer.FocusChat() })
	e.applyShellTasks(e.ctrl.ShellTasks())
}

func (e *View) applyShellTasks(tasks []shelltask.Snapshot) {
	e.shellTasks = slices.Clone(tasks)
	if e.shells != nil {
		e.shells.SetTasks(e.shellTasks)
	}
	if e.ctrl != nil {
		e.shellSessionID = e.ctrl.SessionID()
	}
	if e.shells != nil {
		e.shells.SetSessionID(e.shellSessionID)
	}
	if e.transcript != nil {
		e.transcript.SetShellTasks(shellTasksForSession(e.shellTasks, e.shellSessionID))
	}
	e.updateShellFooter()
}

func (e *View) updateShellFooter() {
	if e.footer == nil {
		return
	}
	live, foreground := 0, 0
	for _, task := range e.shellTasks {
		if task.State == shelltask.Running {
			live++
			if !task.Background && task.ParentSessionID == e.shellSessionID {
				foreground++
			}
		}
	}
	hint := ""
	if foreground > 0 {
		if key := keys.Label(keys.CmdBackgroundShell); key != "" {
			hint = key + " backgrounds · /tasks"
		} else {
			hint = "/tasks to background"
		}
	}
	e.footer.SetShellActivity(live, hint)
}

func (e *View) shellRunning() bool {
	for _, task := range e.shellTasks {
		if task.State == shelltask.Running {
			return true
		}
	}
	return false
}

// ShowShellTasks opens the shell task browser and moves keyboard focus to it.
func (e *View) ShowShellTasks() {
	if e.shells != nil {
		e.shells.Show()
		e.FocusEditor()
	}
}

// BackgroundShell promotes the sole foreground command, or opens explicit selection.
func (e *View) BackgroundShell() {
	id, count := foregroundShell(shellTasksForSession(e.shellTasks, e.shellSessionID))
	if count > 1 {
		e.ShowShellTasks()
		return
	}
	if count == 0 {
		e.Toast("No foreground shell command is running", toast.ToastWarning, 3*time.Second)
		return
	}
	if e.ctrl == nil {
		return
	}
	if err := e.ctrl.BackgroundShellTask(id); err != nil {
		e.Toast("Cannot background command: "+err.Error(), toast.ToastError, 4*time.Second)
	}
}

func (e *View) syncShellSession() {
	if e.ctrl == nil || e.shellSessionID == e.ctrl.SessionID() {
		return
	}
	e.applyShellTasks(e.shellTasks)
	e.transcript.Sync()
}

func shellTasksForSession(tasks []shelltask.Snapshot, sessionID string) []shelltask.Snapshot {
	var selected []shelltask.Snapshot
	for _, task := range tasks {
		if task.ParentSessionID == sessionID {
			selected = append(selected, task)
		}
	}
	return selected
}

func foregroundShell(tasks []shelltask.Snapshot) (string, int) {
	id, count := "", 0
	for _, task := range tasks {
		if task.State == shelltask.Running && !task.Background {
			id = task.ID
			count++
		}
	}
	if count != 1 {
		id = ""
	}
	return id, count
}
