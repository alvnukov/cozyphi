package diag

import "time"

// Response limits. They exist so one snapshot can never become the turn's
// whole context budget, and so the answer degrades by dropping fields with
// an explicit flag rather than by silently producing something enormous.
const (
	// DefaultMaxFieldsPerCategory caps how many fields one category emits.
	DefaultMaxFieldsPerCategory = 64
	// DefaultMaxValueBytes caps one string — a value or a source ref.
	DefaultMaxValueBytes = 512
	// DefaultMaxListItems caps one list value's items.
	DefaultMaxListItems = 32
	// DefaultMaxTotalBytes caps the whole answer's payload.
	DefaultMaxTotalBytes = 16384
)

// Time limits. Every collector is a read of state this process already
// holds, so answering is microseconds of work — but a read still waits on
// whatever mutex its owner is holding, and an owner busy elsewhere would
// otherwise hold the turn open for as long as it liked. The budgets turn
// that into an unavailable category with a reason.
//
// They are generous on purpose: crossing one means an owner is stuck, not
// that the machine is slow, so a budget short enough to fire under load
// would report a fault that is not there.
const (
	// DefaultMaxCategoryDuration bounds one category's observation.
	DefaultMaxCategoryDuration = 2 * time.Second
	// DefaultMaxTotalDuration bounds the whole answer. The overview reads
	// every category in turn, so without it eleven stuck owners would cost
	// eleven times the per-category budget.
	DefaultMaxTotalDuration = 5 * time.Second
)

// truncationMarker ends a string that had to be cut.
const truncationMarker = "…"

// fieldOverheadBytes approximates the JSON scaffolding around one field —
// its keys, the three layer objects, the timestamp. The budget is charged
// with it so the byte cap tracks the rendered size rather than the payload
// alone. It is a bound, not an exact measure.
const fieldOverheadBytes = 200

// Limits bounds one response, in size and in time. Zero or negative members
// fall back to the defaults, so a partly-filled Limits is safe rather than
// unbounded.
type Limits struct {
	MaxFieldsPerCategory int
	MaxValueBytes        int
	MaxListItems         int
	MaxTotalBytes        int
	MaxCategoryDuration  time.Duration
	MaxTotalDuration     time.Duration
}

// DefaultLimits returns the limits every caller should start from.
func DefaultLimits() Limits {
	return Limits{
		MaxFieldsPerCategory: DefaultMaxFieldsPerCategory,
		MaxValueBytes:        DefaultMaxValueBytes,
		MaxListItems:         DefaultMaxListItems,
		MaxTotalBytes:        DefaultMaxTotalBytes,
		MaxCategoryDuration:  DefaultMaxCategoryDuration,
		MaxTotalDuration:     DefaultMaxTotalDuration,
	}
}

// normalized replaces every unset or nonsensical member with its default. A
// caller that forgot a field gets a bounded response, never an unbounded one.
func (l Limits) normalized() Limits {
	defaults := DefaultLimits()
	if l.MaxFieldsPerCategory <= 0 {
		l.MaxFieldsPerCategory = defaults.MaxFieldsPerCategory
	}
	if l.MaxValueBytes <= 0 {
		l.MaxValueBytes = defaults.MaxValueBytes
	}
	if l.MaxListItems <= 0 {
		l.MaxListItems = defaults.MaxListItems
	}
	if l.MaxTotalBytes <= 0 {
		l.MaxTotalBytes = defaults.MaxTotalBytes
	}
	if l.MaxCategoryDuration <= 0 {
		l.MaxCategoryDuration = defaults.MaxCategoryDuration
	}
	if l.MaxTotalDuration <= 0 {
		l.MaxTotalDuration = defaults.MaxTotalDuration
	}
	// A per-category budget longer than the whole answer's is not a budget:
	// the first stuck owner would spend everything and the ones after it
	// would be reported unreached for a reason that was really the first
	// one's. The narrower bound wins.
	l.MaxCategoryDuration = min(l.MaxCategoryDuration, l.MaxTotalDuration)
	return l
}
