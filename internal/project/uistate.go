package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// UIState contains non-secret, global TUI preferences.
type UIState struct {
	SidebarWidth int `json:"sidebarWidth,omitempty"`
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

// SaveUIState atomically writes owner-only global TUI preferences.
func SaveUIState(global GlobalLayout, state UIState) (retErr error) {
	path := global.UIStateFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create UI preferences directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ui-*.json")
	if err != nil {
		return fmt.Errorf("create UI preferences: %w", err)
	}
	tmpPath := tmp.Name()
	closed := false
	renamed := false
	defer func() {
		if !closed {
			if closeErr := tmp.Close(); retErr == nil && closeErr != nil {
				retErr = fmt.Errorf("close UI preferences: %w", closeErr)
			}
		}
		if !renamed {
			if removeErr := os.Remove(
				tmpPath,
			); retErr == nil && removeErr != nil &&
				!errors.Is(removeErr, os.ErrNotExist) {
				retErr = fmt.Errorf("remove temporary UI preferences: %w", removeErr)
			}
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("protect UI preferences: %w", err)
	}
	if err := json.NewEncoder(tmp).Encode(state); err != nil {
		return fmt.Errorf("encode UI preferences: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync UI preferences: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close UI preferences: %w", err)
	}
	closed = true
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace UI preferences: %w", err)
	}
	renamed = true
	return nil
}
