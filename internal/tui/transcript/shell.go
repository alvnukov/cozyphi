package transcript

import (
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/shelltask"
)

// SetShellTasks changes the live overlay, including rows from earlier turns.
// The task snapshot wins over a late tool launch receipt.
func (t *TranscriptPane) SetShellTasks(tasks []shelltask.Snapshot) {
	if t == nil || t.mapper == nil {
		return
	}
	if t.mapper.historicalShellTasks == nil {
		t.mapper.historicalShellTasks = make(map[string]shelltask.Snapshot)
	}
	t.mapper.shellTasks = make(map[string]shelltask.Snapshot, len(tasks))
	for _, task := range tasks {
		if task.ToolUseID != "" {
			t.mapper.shellTasks[task.ToolUseID] = task
			if task.State.Terminal() {
				t.mapper.historicalShellTasks[task.ToolUseID] = task
			}
		}
	}
	t.syncMode = projectionSyncFull
}

// SetHistoricalShellTasks installs host-authored replay facts independently of live state.
func (t *TranscriptPane) SetHistoricalShellTasks(tasks []shelltask.Snapshot) {
	if t == nil || t.mapper == nil {
		return
	}
	t.mapper.historicalShellTasks = make(map[string]shelltask.Snapshot, len(tasks))
	for _, task := range tasks {
		if task.ToolUseID != "" {
			t.mapper.historicalShellTasks[task.ToolUseID] = task
		}
	}
	t.syncMode = projectionSyncFull
}

func (m *Mapper) shellSnapshot(id string) (shelltask.Snapshot, bool) {
	if task, ok := m.shellTasks[id]; ok {
		return task, true
	}
	task, ok := m.historicalShellTasks[id]
	return task, ok
}

func (m *Mapper) shellItems(items []session.Item) []session.Item {
	for i, item := range items {
		task, ok := m.shellSnapshot(item.ToolUseID)
		if !ok || item.ToolName != "bash" {
			continue
		}
		items[i].ToolRun.Status = shellToolStatus(task.State)
		items[i].ToolRun.Detail = components.PlainText(task.Command)
		items[i].ToolRun.Output = components.PlainText(task.Output)
		if task.ExitCode != nil {
			items[i].ToolRun.ExitCode = *task.ExitCode
		}
		items[i].ToolRun.Error = components.PlainText(task.Error)
	}
	return items
}

func shellToolStatus(state shelltask.State) session.ToolStatus {
	switch state {
	case shelltask.Running:
		return session.ToolInProgress
	case shelltask.Failed:
		return session.ToolError
	case shelltask.Stopped:
		return session.ToolCancelled
	default:
		return session.ToolDone
	}
}

func (m *Mapper) shellBlock(b *block.BashBlock, item session.Item) {
	if task, ok := m.shellSnapshot(item.ToolUseID); ok {
		if task.State == shelltask.Unknown {
			b.Status = block.BashUnknown
		}
		b.Background = task.Background
		if task.Deadline != nil {
			b.Deadline = *task.Deadline
		}
		b.Truncated = task.Truncated
	}
}
