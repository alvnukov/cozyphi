package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pulseaiclub/phi/internal/components/toast"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tools"
)

// handleBashSubmit runs a user "!cmd" shell locally (not via the agent).
// Returns true when the input was consumed as a bash command.
func (editor *Editor) handleBashSubmit(text string) bool {
	if !strings.HasPrefix(text, "!") {
		return false
	}
	command := strings.TrimSpace(text[1:])
	if command == "" {
		return false
	}
	if session.IsStreaming(editor.snap) {
		editor.toast.Show("Unable to use shell mode while agent is active", toast.ToastWarning, 3*time.Second)
		return true
	}
	if editor.bashRunning.Load() {
		editor.toast.Show("A bash command is already running. Press Esc to cancel it first.", toast.ToastWarning, 3*time.Second)
		return true
	}

	editor.hideCompleters()
	editor.Chat.Value = ""
	editor.Chat.Cursor = 0
	editor.syncBashModeBorder("")

	id := fmt.Sprintf("bash-%d", time.Now().UnixNano())
	editor.applySessionEvent(session.LocalBashStart{ID: id, Command: command})
	editor.list.Entries, editor.listIDs = editor.mapper.Sync(editor.list.Entries, editor.listIDs, editor.snap)
	editor.list.InvalidateHeights()
	editor.list.StickToBottom()

	go editor.runBash(id, command)
	return true
}

func (editor *Editor) runBash(id, command string) {
	editor.bashMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	editor.bashCancel = cancel
	editor.bashMu.Unlock()
	editor.bashRunning.Store(true)
	defer func() {
		editor.bashRunning.Store(false)
		editor.bashMu.Lock()
		editor.bashCancel = nil
		editor.bashMu.Unlock()
	}()

	var (
		outMu sync.Mutex
		out   strings.Builder
	)
	publishOutput := func() {
		outMu.Lock()
		cur := out.String()
		outMu.Unlock()
		editor.Publish(SessionEventMsg{Event: session.ToolData{Run: session.ToolRun{
			ToolUseID: id,
			Name:      "bash",
			Status:    session.ToolInProgress,
			Detail:    command,
			Output:    cur,
			Local:     true,
		}}})
	}

	result, err := tools.ExecShell(ctx, command, tools.ShellExecOptions{
		OnChunk: func(chunk string) {
			outMu.Lock()
			out.WriteString(chunk)
			outMu.Unlock()
			publishOutput()
		},
	})
	if err != nil {
		editor.Publish(SessionEventMsg{Event: session.ToolData{Run: session.ToolRun{
			ToolUseID: id,
			Name:      "bash",
			Status:    session.ToolError,
			Detail:    command,
			Output:    result.Output,
			Error:     err.Error(),
			Local:     true,
		}}})
		return
	}
	status := session.ToolDone
	if result.Canceled {
		status = session.ToolCancelled
	} else if result.ExitCode != 0 {
		status = session.ToolError
	}
	outText := result.Output
	if strings.TrimSpace(outText) == "" && !result.Canceled {
		outText = "(no output)"
	}
	editor.Publish(SessionEventMsg{Event: session.ToolData{Run: session.ToolRun{
		ToolUseID: id,
		Name:      "bash",
		Status:    status,
		Detail:    command,
		Output:    outText,
		ExitCode:  result.ExitCode,
		Local:     true,
	}}})
}

// cancelBash aborts a running user "!cmd". Returns true if one was cancelled.
func (editor *Editor) cancelBash() bool {
	if !editor.bashRunning.Load() {
		return false
	}
	editor.bashMu.Lock()
	cancel := editor.bashCancel
	editor.bashMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return true
}

func (editor *Editor) syncBashModeBorder(text string) {
	bash := strings.HasPrefix(strings.TrimLeft(text, " \t"), "!")
	if bash {
		editor.Chat.BorderStyle = editor.theme.ToolName
	} else {
		editor.Chat.BorderStyle = editor.theme.Border
	}
}
