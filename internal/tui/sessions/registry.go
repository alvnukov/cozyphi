package sessions

import (
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Entry is a membership snapshot. View is the retained UI object, not a copy.
// ID identifies this live membership independently of its history conversation.
// LastViewed is zero until the entry is first selected; Accent survives selection and removal of other entries.
type Entry struct {
	ID         string
	Name       string
	View       *View
	Accent     int
	LastViewed time.Time
}

// Registry retains ordered views and selection history on the UI goroutine.
// It neither manages execution nor disposes views. Callers own view activity and presentation.
type Registry struct {
	entries    []Entry
	active     string
	visited    []string // Oldest to newest, excluding the current selection.
	limit      int
	nextAccent int
	onChange   func()
}

// NewRegistry uses a limit of 12 when limit is nonpositive.
// onChange runs synchronously after each committed mutation, never for errors or no-ops.
func NewRegistry(limit int, onChange func()) *Registry {
	if limit <= 0 {
		limit = 12
	}
	return &Registry{limit: limit, onChange: onChange}
}

// Open retains view under a new opaque live ID. Only the first open selects it.
func (r *Registry) Open(name string, view *View) (string, error) {
	if view == nil {
		return "", errors.New("cannot open a nil view: provide an initialized session view")
	}
	for _, entry := range r.entries {
		if entry.View == view {
			return "", fmt.Errorf("view is already open as session %q: activate it instead", entry.ID)
		}
	}
	if len(r.entries) >= r.limit {
		return "", fmt.Errorf("session limit (%d) reached: close a session before opening another", r.limit)
	}
	id := rand.Text()
	for r.index(id) >= 0 {
		id = rand.Text()
	}
	entry := Entry{ID: id, Name: name, View: view, Accent: r.nextAccent}
	r.nextAccent++
	if len(r.entries) == 0 {
		r.active = id
		entry.LastViewed = time.Now()
	}
	r.entries = append(r.entries, entry)
	r.changed()
	return id, nil
}

// Active returns a value snapshot of the selected entry, if any.
func (r *Registry) Active() (Entry, bool) {
	if i := r.index(r.active); i >= 0 {
		return r.entries[i], true
	}
	return Entry{}, false
}

// Entries returns snapshots in stable opening order, with an independent backing slice.
func (r *Registry) Entries() []Entry { return slices.Clone(r.entries) }

// Len returns the number of retained views.
func (r *Registry) Len() int { return len(r.entries) }

// Activate selects a live ID, remembering the previous selection once.
func (r *Registry) Activate(id string) error {
	if len(r.entries) == 0 {
		return r.emptyError()
	}
	i := r.index(id)
	if i < 0 {
		return fmt.Errorf("session %q is not open: select an ID from the session list", id)
	}
	if id == r.active {
		return nil
	}
	r.forget(id)
	r.forget(r.active)
	r.visited = append(r.visited, r.active)
	r.selectEntry(i)
	r.changed()
	return nil
}

// Next selects the next entry, wrapping to the first.
func (r *Registry) Next() error { return r.cycle(1) }

// Prev selects the previous entry, wrapping to the last.
func (r *Registry) Prev() error { return r.cycle(-1) }

// Jump selects a 1-based position in opening order.
func (r *Registry) Jump(n int) error {
	if len(r.entries) == 0 {
		return r.emptyError()
	}
	if n < 1 || n > len(r.entries) {
		return fmt.Errorf("session position %d is out of range: choose 1 through %d", n, len(r.entries))
	}
	return r.Activate(r.entries[n-1].ID)
}

// Back consumes the most recently visited live selection. It does not push the
// current selection back onto history, so repeated calls walk backward rather than toggle.
func (r *Registry) Back() error {
	if len(r.entries) == 0 {
		return r.emptyError()
	}
	if len(r.visited) == 0 {
		return errors.New("no previously viewed session: select another session first")
	}
	id := r.visited[len(r.visited)-1]
	r.visited = r.visited[:len(r.visited)-1]
	r.selectEntry(r.index(id))
	r.changed()
	return nil
}

// Close removes a membership and returns its view for caller-owned disposal.
// An active close selects the latest live visited entry, otherwise the next
// adjacent entry in opening order (or the previous entry when closing the last).
func (r *Registry) Close(id string) (*View, error) {
	if len(r.entries) == 0 {
		return nil, r.emptyError()
	}
	i := r.index(id)
	if i < 0 {
		return nil, fmt.Errorf("session %q is not open: choose a session from the session list to close", id)
	}
	view := r.entries[i].View
	r.entries = slices.Delete(r.entries, i, i+1)
	r.forget(id)
	if r.active == id {
		r.active = ""
		if n := len(r.visited); n > 0 {
			previous := r.visited[n-1]
			r.visited = r.visited[:n-1]
			r.selectEntry(r.index(previous))
		} else if len(r.entries) > 0 {
			r.selectEntry(min(i, len(r.entries)-1))
		}
	}
	r.changed()
	return view, nil
}

func (r *Registry) index(id string) int {
	for i := range r.entries {
		if r.entries[i].ID == id {
			return i
		}
	}
	return -1
}

func (r *Registry) forget(id string) {
	r.visited = slices.DeleteFunc(r.visited, func(previous string) bool { return previous == id })
}

func (r *Registry) selectEntry(i int) {
	r.active = r.entries[i].ID
	r.entries[i].LastViewed = time.Now()
}

func (r *Registry) cycle(delta int) error {
	if len(r.entries) == 0 {
		return r.emptyError()
	}
	i := (r.index(r.active) + delta + len(r.entries)) % len(r.entries)
	return r.Activate(r.entries[i].ID)
}

func (*Registry) emptyError() error {
	return errors.New("no sessions are open: open a session first")
}

func (r *Registry) changed() {
	if r.onChange != nil {
		r.onChange()
	}
}
