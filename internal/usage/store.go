// Package usage keeps local picker history and ranks choices by usefulness.
package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	SlashCommands = "slash_commands"
	Models        = "models"
	Skills        = "skills"
	Palette       = "palette"

	fileVersion = 1
)

const recencyWindow = 30 * 24 * time.Hour

type entry struct {
	Count    uint64    `json:"count"`
	LastUsed time.Time `json:"lastUsed"`
}

type diskState struct {
	Version int                         `json:"version"`
	Entries map[string]map[string]entry `json:"entries"`
}

// Store owns local usage history. An empty path creates an in-memory store.
type Store struct {
	mu      sync.RWMutex
	path    string
	now     func() time.Time
	entries map[string]map[string]entry
}

// Open loads local usage history. If the file is malformed, it returns an
// empty usable store together with an actionable error.
func Open(path string) (*Store, error) {
	store := newStore(path, time.Now)
	if path == "" {
		return store, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, fmt.Errorf("read usage history: %w", err)
	}
	var state diskState
	if err := json.Unmarshal(data, &state); err != nil {
		return store, fmt.Errorf("parse usage history: %w", err)
	}
	if state.Entries != nil {
		store.entries = state.Entries
	}
	return store, nil
}

func newStore(path string, now func() time.Time) *Store {
	return &Store{
		path:    path,
		now:     now,
		entries: make(map[string]map[string]entry),
	}
}

// Record persists one successful use of an item.
func (s *Store) Record(scope, id string) error {
	if s == nil {
		return nil
	}
	if !validScope(scope) {
		return fmt.Errorf("record usage: unknown scope %q", scope)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("record usage: item ID is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.entries[scope]
	if items == nil {
		items = make(map[string]entry)
		s.entries[scope] = items
	}
	previous, existed := items[id]
	updated := previous
	updated.Count++
	updated.LastUsed = s.now().UTC()
	items[id] = updated
	if err := s.saveLocked(); err != nil {
		if existed {
			items[id] = previous
		} else {
			delete(items, id)
		}
		return err
	}
	return nil
}

// Rank returns a ranked copy of items without changing their input order.
// Equal scores retain the caller's stable default order.
func Rank[T any](store *Store, scope string, items []T, id func(T) string) []T {
	ranked := append([]T(nil), items...)
	if store == nil || len(ranked) < 2 {
		return ranked
	}

	store.mu.RLock()
	history := make(map[string]entry, len(store.entries[scope]))
	maps.Copy(history, store.entries[scope])
	now := store.now()
	store.mu.RUnlock()

	sort.SliceStable(ranked, func(i, j int) bool {
		return score(history[id(ranked[i])], now) > score(history[id(ranked[j])], now)
	})
	return ranked
}

func score(item entry, now time.Time) float64 {
	if item.Count == 0 || item.LastUsed.IsZero() {
		return 0
	}
	age := now.Sub(item.LastUsed)
	age = max(age, 0)
	frequency := math.Log1p(float64(item.Count))
	recency := math.Exp(-float64(age) / float64(recencyWindow))
	return frequency + recency
}

func validScope(scope string) bool {
	switch scope {
	case SlashCommands, Models, Skills, Palette:
		return true
	default:
		return false
	}
}

func (s *Store) saveLocked() (retErr error) {
	if s.path == "" {
		return nil
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create usage history directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".usage-*.json")
	if err != nil {
		return fmt.Errorf("create usage history: %w", err)
	}
	tmpPath := tmp.Name()
	closed := false
	renamed := false
	defer func() {
		if !closed {
			if closeErr := tmp.Close(); retErr == nil && closeErr != nil {
				retErr = fmt.Errorf("close usage history: %w", closeErr)
			}
		}
		if !renamed {
			if removeErr := os.Remove(
				tmpPath,
			); retErr == nil && removeErr != nil &&
				!errors.Is(removeErr, os.ErrNotExist) {
				retErr = fmt.Errorf("remove temporary usage history: %w", removeErr)
			}
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("protect usage history: %w", err)
	}
	state := diskState{Version: fileVersion, Entries: s.entries}
	if err := json.NewEncoder(tmp).Encode(state); err != nil {
		return fmt.Errorf("encode usage history: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync usage history: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close usage history: %w", err)
	}
	closed = true
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace usage history: %w", err)
	}
	renamed = true
	return nil
}
