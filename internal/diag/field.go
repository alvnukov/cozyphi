package diag

import (
	"context"
	"time"
)

// Availability says whether a category can be observed at all. It is the
// honest answer to "is this wired yet", and it is the reason a catalog never
// has to lie: a category with no collector reports NotImplemented instead of
// vanishing or reporting empty data.
type Availability string

// Availability values.
const (
	// AvailabilityAvailable means the collector is wired and can observe now.
	AvailabilityAvailable Availability = "available"
	// AvailabilityNotImplemented means no collector is wired for this
	// category yet. It is a gap in the harness, not in the process.
	AvailabilityNotImplemented Availability = "not_implemented"
	// AvailabilityUnavailable means a collector exists but could not observe
	// right now; Reason says why.
	AvailabilityUnavailable Availability = "unavailable"
	// AvailabilityNotApplicable means the category cannot exist in this
	// process shape (a TUI-only category in a headless run, say).
	AvailabilityNotApplicable Availability = "not_applicable"
)

// State says what kind of answer one observation is. It separates the four
// things an empty value could otherwise be confused for.
type State string

// State values.
const (
	// StatePresent means the value below is the real one.
	StatePresent State = "present"
	// StateUnset means nothing set this layer; the value is the zero value.
	StateUnset State = "unset"
	// StateRedacted means a real value existed and had to be masked.
	StateRedacted State = "redacted"
	// StateUnavailable means the owner could not answer right now.
	StateUnavailable State = "unavailable"
	// StateNotApplicable means this layer does not exist for this field —
	// a build constant has no configured layer.
	StateNotApplicable State = "not_applicable"
)

// Apply says what it would take for a change to this field to take effect.
type Apply string

// Apply values, from cheapest to most disruptive.
const (
	ApplyImmediate  Apply = "immediate"
	ApplyNextTurn   Apply = "next_turn"
	ApplyReload     Apply = "reload"
	ApplyNewSession Apply = "new_session"
	ApplyRestart    Apply = "restart"
)

// Scope says how far a field's value reaches.
type Scope string

// Scope values, from widest to narrowest.
const (
	ScopeProcess   Scope = "process"
	ScopeSession   Scope = "session"
	ScopeWorkspace Scope = "workspace"
	ScopeTurn      Scope = "turn"
	ScopeStep      Scope = "step"
)

// Kind tags which member of Value carries the answer. Values are a closed
// union rather than an any: the wire shape then never depends on what a
// collector happened to put in an interface, and false and 0 stay visible
// instead of collapsing into "absent".
type Kind string

// Kind values.
const (
	KindNone     Kind = "none"
	KindString   Kind = "string"
	KindInt      Kind = "int"
	KindBool     Kind = "bool"
	KindDuration Kind = "duration"
	KindList     Kind = "list"
)

// Value is one observed value. Every member is serialized on every value —
// no omitempty anywhere in this package — so a false or a 0 reaches the
// model as a false or a 0 and never as a missing field.
type Value struct {
	Kind Kind     `json:"kind"`
	Str  string   `json:"string"`
	Int  int64    `json:"int"`
	Bool bool     `json:"bool"`
	List []string `json:"list"`
}

// StringValue builds a string value.
func StringValue(s string) Value {
	return Value{Kind: KindString, Str: s, List: []string{}}
}

// IntValue builds an integer value.
func IntValue(n int64) Value {
	return Value{Kind: KindInt, Int: n, List: []string{}}
}

// BoolValue builds a boolean value.
func BoolValue(b bool) Value {
	return Value{Kind: KindBool, Bool: b, List: []string{}}
}

// DurationValue builds a duration value. It carries both shapes: Int is
// nanoseconds for arithmetic, Str is the human spelling.
func DurationValue(d time.Duration) Value {
	return Value{Kind: KindDuration, Str: d.String(), Int: int64(d), List: []string{}}
}

// ListValue builds a list value from a detached copy of items.
func ListValue(items []string) Value {
	list := make([]string, len(items))
	copy(list, items)
	return Value{Kind: KindList, List: list}
}

// NoValue is the value of an observation that has none.
func NoValue() Value {
	return Value{Kind: KindNone, List: []string{}}
}

// SourceKind names where a layer's value came from. It is a closed set: a
// collector that does not know says so with SourceUnknown rather than
// inventing provenance.
type SourceKind string

// SourceKind values. The middle of the list is also an override order:
// a default is replaced by the config file, the config file by the
// environment, the environment by a session-time choice, and all of them,
// for as long as a step runs, by a plan pin.
const (
	SourceCLIFlag    SourceKind = "cli_flag"
	SourceDefault    SourceKind = "default"
	SourceConfigFile SourceKind = "config_file"
	SourceEnv        SourceKind = "env"
	SourceSession    SourceKind = "session"
	SourcePlan       SourceKind = "plan"
	SourceComputed   SourceKind = "computed"
	SourceBuild      SourceKind = "build"
	SourceUnknown    SourceKind = "unknown"
)

// Source is where one layer's value came from. Ref is safe metadata that
// names the origin — a flag name, a config key, a sanitized path — never the
// value itself and never raw config text.
type Source struct {
	Kind SourceKind `json:"kind"`
	Ref  string     `json:"ref"`
}

// Observation is one layer of one field: what state it is in, what it holds,
// and where it came from.
type Observation struct {
	State  State  `json:"state"`
	Value  Value  `json:"value"`
	Source Source `json:"source"`
}

// Present builds an observation whose value is real. The registry may still
// downgrade it to StateRedacted if the value had to be masked.
func Present(value Value, source Source) Observation {
	return Observation{State: StatePresent, Value: value, Source: source}
}

// Unset builds an observation for a layer nothing set. The value is carried
// anyway — the zero a caller would get — so "unset and false" is legible.
func Unset(value Value, source Source) Observation {
	return Observation{State: StateUnset, Value: value, Source: source}
}

// Unavailable builds an observation the owner could not answer right now.
func Unavailable() Observation {
	return Observation{State: StateUnavailable, Value: NoValue(), Source: Source{Kind: SourceUnknown}}
}

// NotApplicable builds an observation for a layer that does not exist for
// this field.
func NotApplicable(kind SourceKind) Observation {
	return Observation{State: StateNotApplicable, Value: NoValue(), Source: Source{Kind: kind}}
}

// Field is one observable parameter across the three layers the epic
// distinguishes: Configured is what a source asked for, Loaded is what the
// owner took in, Effective is what is acting right now. They differ far more
// often than they look like they should, and the difference is usually the
// answer to the question being asked.
type Field struct {
	Key        string      `json:"key"`
	Configured Observation `json:"configured"`
	Loaded     Observation `json:"loaded"`
	Effective  Observation `json:"effective"`
	Apply      Apply       `json:"apply"`
	Scope      Scope       `json:"scope"`
	ObservedAt time.Time   `json:"observed_at"`
	Revision   string      `json:"revision"`
}

// Status is a collector's static self-description. It must be cheap and it
// must not touch the owner: the catalog is answered from Status alone, so
// listing what can be observed never observes anything.
type Status struct {
	Availability Availability `json:"availability"`
	Reason       string       `json:"reason"`
	Keys         []string     `json:"keys"`
}

// Collector observes one category. Implementations stay dumb: they read
// their owner through accessors and return fields. They do not sanitize,
// bound, sort, stamp timestamps or handle limits — the Registry does all of
// that, so a new collector cannot get it wrong.
//
// Collect must be side-effect free: no process spawning, no network, no
// reload, no hooks, no filesystem scans.
type Collector interface {
	// Category names the area this collector observes.
	Category() Category
	// Status describes availability and the field keys this collector
	// declares. It is called for every catalog request, so it must be cheap
	// and must not reach into the owner.
	Status() Status
	// Collect observes the owner now and returns its fields.
	Collect(ctx context.Context) ([]Field, error)
}
