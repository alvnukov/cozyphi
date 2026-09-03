package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/alvnukov/cozyphi/internal/atomicfile"
)

// UIState contains non-secret, global TUI preferences.
type UIState struct {
	SidebarWidth        int    `json:"sidebarWidth,omitempty"`
	SidebarHidden       bool   `json:"sidebarHidden,omitempty"`
	StopLimitDisabled   bool   `json:"stopLimitDisabled,omitempty"`
	PlanDisabled        bool   `json:"planDisabled,omitempty"`
	ExpandEditsDisabled bool   `json:"expandEditsDisabled,omitempty"`
	LastModel           string `json:"lastModel,omitempty"`
}

// SidebarVisible resolves the default-on visibility preference. Encoding the
// inverse keeps older and missing UI state files visible without migration.
func (s UIState) SidebarVisible() bool {
	return !s.SidebarHidden
}

// StopLimitEnabled resolves the default-enabled tool-round stop. Encoding the
// inverse keeps older and missing UI state files enabling the stop by default.
func (s UIState) StopLimitEnabled() bool {
	return !s.StopLimitDisabled
}

// PlanEnabled resolves the default-on plan feature: tool, prompt gate, sidebar
// pane and /plan command. The inverse encoding keeps older and missing UI
// state files enabling the plan without migration, and leaves the durable
// plan itself untouched when the feature is switched off.
func (s UIState) PlanEnabled() bool {
	return !s.PlanDisabled
}

// ExpandEdits resolves the default-on "edit cards render expanded"
// preference. The inverse encoding keeps older and missing UI state files
// expanding edits without migration.
func (s UIState) ExpandEdits() bool {
	return !s.ExpandEditsDisabled
}

// LoadUIState reads global TUI preferences. A missing file is the zero state;
// malformed content is actionable instead of silently discarded.
func LoadUIState(global GlobalLayout) (UIState, error) {
	data, err := os.ReadFile(global.UIStateFile())
	if errors.Is(err, os.ErrNotExist) {
		return UIState{}, nil
	}
	if err != nil {
		return UIState{}, fmt.Errorf("read UI preferences: %w", err)
	}
	var state UIState
	if err := json.Unmarshal(data, &state); err != nil {
		return UIState{}, fmt.Errorf("parse UI preferences: %w", err)
	}
	return state, nil
}

// MutateUIState loads the global TUI preferences, applies mutate, and
// atomically persists the result. Siblings the closure never touches survive
// the cycle, so callers set one field without learning the whole layout.
func MutateUIState(global GlobalLayout, mutate func(*UIState)) error {
	state, err := LoadUIState(global)
	if err != nil {
		return err
	}
	mutate(&state)
	return saveUIState(global, state)
}

// saveUIState atomically writes owner-only global TUI preferences.
func saveUIState(global GlobalLayout, state UIState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode UI preferences: %w", err)
	}
	path := global.UIStateFile()
	if err := atomicfile.Write(path, 0o600, append(data, '\n')); err != nil {
		return fmt.Errorf("write UI preferences: %w", err)
	}
	return nil
}
