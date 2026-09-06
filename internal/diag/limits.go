package diag

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

// truncationMarker ends a string that had to be cut.
const truncationMarker = "…"

// fieldOverheadBytes approximates the JSON scaffolding around one field —
// its keys, the three layer objects, the timestamp. The budget is charged
// with it so the byte cap tracks the rendered size rather than the payload
// alone. It is a bound, not an exact measure.
const fieldOverheadBytes = 200

// Limits bounds one response. Zero or negative members fall back to the
// defaults, so a partly-filled Limits is safe rather than unbounded.
type Limits struct {
	MaxFieldsPerCategory int
	MaxValueBytes        int
	MaxListItems         int
	MaxTotalBytes        int
}

// DefaultLimits returns the limits every caller should start from.
func DefaultLimits() Limits {
	return Limits{
		MaxFieldsPerCategory: DefaultMaxFieldsPerCategory,
		MaxValueBytes:        DefaultMaxValueBytes,
		MaxListItems:         DefaultMaxListItems,
		MaxTotalBytes:        DefaultMaxTotalBytes,
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
	return l
}
