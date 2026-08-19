package plugin

import (
	"fmt"
	"slices"
	"sync"
)

// Point is a named extension slot that can be listed and cleared via [Registry].
// [*Hook] and [*Chain] implement Point.
type Point interface {
	Name() string
	Len() int
	Clear()
}

// Registry is a name → Point directory for one host instance (e.g. an Engine).
// It is not a global singleton; create one per host.
type Registry struct {
	mu     sync.RWMutex
	points map[string]Point
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{points: make(map[string]Point)}
}

// Add registers p under p.Name(). Fails if the name is empty, p is nil, or the
// name is already taken.
func (r *Registry) Add(p Point) error {
	if r == nil {
		return fmt.Errorf("plugin: nil registry")
	}
	if p == nil {
		return fmt.Errorf("plugin: nil point")
	}
	name := p.Name()
	if name == "" {
		return fmt.Errorf("plugin: point name is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.points == nil {
		r.points = make(map[string]Point)
	}
	if _, ok := r.points[name]; ok {
		return fmt.Errorf("plugin: duplicate point %q", name)
	}
	r.points[name] = p
	return nil
}

// MustAdd is like Add but panics on error.
func (r *Registry) MustAdd(p Point) {
	if err := r.Add(p); err != nil {
		panic(err)
	}
}

// Get returns the point named name.
func (r *Registry) Get(name string) (Point, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.points[name]
	return p, ok
}

// Names returns registered point names in sorted order.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.points))
	for name := range r.points {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// Len returns the number of registered points.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.points)
}

// ClearAll clears every point's subscribers and removes all points from the
// registry.
func (r *Registry) ClearAll() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.points {
		p.Clear()
	}
	clear(r.points)
}
