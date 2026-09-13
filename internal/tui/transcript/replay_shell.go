package transcript

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/shelltask"
)

// ReplayShellTasks reconstructs process facts from host-authored receipts.
// A launch receipt proves acceptance, never that the process is still alive.
func ReplayShellTasks(entries []session.MessageEntry) []shelltask.Snapshot {
	return sortedShellRecords(replayShellRecords(entries))
}

func sortedShellRecords(records map[string]shelltask.Snapshot) []shelltask.Snapshot {
	tasks := make([]shelltask.Snapshot, 0, len(records))
	for _, task := range records {
		tasks = append(tasks, task)
	}
	slices.SortFunc(tasks, func(a, b shelltask.Snapshot) int { return strings.Compare(a.ID, b.ID) })
	return tasks
}

func replayShellRecords(entries []session.MessageEntry) map[string]shelltask.Snapshot {
	records := make(map[string]shelltask.Snapshot)
	for _, entry := range entries {
		message, ok := entry.(session.SessionMessageEntry)
		if !ok {
			continue
		}
		if outcome, valid := parseShellOutcomeDelivery(message); valid {
			records[outcome.ToolUseID] = outcome.Snapshot
			continue
		}
		if message.Message.Role != llm.RoleTool || message.Message.ToolCallID == "" {
			continue
		}
		id, valid := shellStartedID(message.DeliveryID)
		if !valid {
			continue
		}
		callID := message.Message.ToolCallID
		if prior, exists := records[callID]; exists && prior.State.Terminal() {
			continue
		}
		records[callID] = shelltask.Snapshot{ID: id, ToolUseID: callID, State: shelltask.Unknown, Background: true}
	}
	// Commands come from structured tool arguments, never the human-readable receipt.
	for _, entry := range entries {
		message, ok := entry.(session.SessionMessageEntry)
		if !ok || message.Message.Role != llm.RoleAssistant {
			continue
		}
		for _, call := range message.Message.ToolCalls {
			task, exists := records[call.ID]
			if !exists || !strings.EqualFold(call.Function.Name, "bash") || task.Command != "" {
				continue
			}
			var input struct {
				Command string `json:"command"`
			}
			if json.Unmarshal([]byte(call.Function.Arguments), &input) == nil {
				task.Command = input.Command
				records[call.ID] = task
			}
		}
	}
	return records
}

func shellStartedID(deliveryID string) (string, bool) {
	if !strings.HasPrefix(deliveryID, "shell:") || !strings.HasSuffix(deliveryID, ":started") {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(deliveryID, "shell:"), ":started")
	return id, id != ""
}

func parseShellOutcomeDelivery(entry session.SessionMessageEntry) (shelltask.Outcome, bool) {
	if entry.Message.Role != llm.RoleUser || !strings.HasPrefix(entry.DeliveryID, "shell:") ||
		!strings.HasSuffix(entry.DeliveryID, ":terminal") {
		return shelltask.Outcome{}, false
	}
	body := entry.Message.Content
	start, end := strings.IndexByte(body, '{'), strings.LastIndexByte(body, '}')
	if start < 0 || end <= start {
		return shelltask.Outcome{}, false
	}
	var outcome shelltask.Outcome
	if json.Unmarshal([]byte(body[start:end+1]), &outcome) != nil {
		return shelltask.Outcome{}, false
	}
	if outcome.ID == "" || outcome.ToolUseID == "" || !outcome.Background || !outcome.State.Terminal() ||
		outcome.EventID != entry.DeliveryID || outcome.EventID != "shell:"+outcome.ID+":terminal" {
		return shelltask.Outcome{}, false
	}
	return outcome, true
}
