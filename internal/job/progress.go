package job

import "time"

// Progress is a structured live update from a running job (UI overlay, not LLM context).
type Progress struct {
	JobID           string
	ParentID        string // originating session; routing never follows the selected view
	OwnerID         string // live controller lifetime, even when a history is opened twice
	ParentToolUseID string // parent agent tool_use id when known
	ToolUseID       string
	Name            string
	Status          string // e.g. session.ToolStatus.String(): in-progress, done, error
	Detail          string
	Time            time.Time
}
