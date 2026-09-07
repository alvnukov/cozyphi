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

// overviewOverheadBytes approximates the scaffolding around one overview
// row. It is a quarter of a detail field's because the row is a quarter of
// the structure: one layer instead of three, and no source, timestamp, apply
// semantics, scope or revision hung off it.
const overviewOverheadBytes = fieldOverheadBytes / 4

// purse is the answer's byte budget, handed out one category at a time.
//
// A single running total is spent in catalog order, so the categories at the
// front take all of it and the ones at the back come back empty — empty
// because of where they sit in the catalog and for no other reason. That is
// not a budget, it is a queue, and every category the epic adds makes the
// queue longer.
//
// So each category is handed its own share when its turn comes: what is left
// of the answer divided by the categories still to be read, this one
// included. The floor that gives is worth stating exactly — no category ever
// gets less than total/n bytes, wherever it sits — because a category can
// spend no more than its own share, so every category after it still finds
// at least its share of the remainder waiting.
//
// What a category does not spend stays in the purse and enlarges every share
// after it. That is what keeps an equal division from being a waste: a
// category with three fields does not lock away a share sized for one with
// thirty, and the last category is offered everything the others left.
type purse struct {
	left  int
	share int
}

func newPurse(total int) *purse { return &purse{left: total} }

// open starts one category's turn. remaining counts the categories still to
// be read, this one included, so the share is measured against what the
// answer actually has left rather than against the size it started at. A
// single category — the detail view — is offered the whole budget.
func (p *purse) open(remaining int) {
	p.share = p.left / max(remaining, 1)
}

// afford charges cost against this category's share and reports whether it
// fit. A category that has spent its share stops adding rows and says so,
// which is why there is no pagination protocol: the caller narrows by
// category, and then by key.
func (p *purse) afford(cost int) bool {
	if cost > p.share {
		return false
	}
	p.share -= cost
	p.left -= cost
	return true
}

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
