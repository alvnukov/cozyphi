package diag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Snapshot modes. Overview is every category reduced to what is in effect;
// detail is one category with every layer and its provenance.
const (
	ModeOverview = "overview"
	ModeDetail   = "detail"
)

// reasonNotImplemented is what a category with no collector says. It names
// the gap instead of pretending the process has nothing there.
const reasonNotImplemented = "no collector is wired for this category yet; it reports nothing rather than guessing"

// Notes the registry attaches when an answer is not whole.
const (
	noteTruncated = "output hit a size limit and fields were dropped; " +
		"narrow it with category, then with key"
	notePartial = "one or more categories could not be observed; each says why in its reason"
	// noteFieldCut is what a single field says when it did not fit whole: a
	// string was cut to the value cap, or list items were dropped to keep the
	// field inside the answer cap. Unlike the other two it advises nothing,
	// because explain is already the narrowest question there is and there is
	// nowhere left to send the reader.
	noteFieldCut = "some values were cut to fit size limits; the field is complete in shape but not in content"
)

// Reasons a category has nothing to show because time ran out rather than
// because anything is wrong with it. They are three strings because they
// call for three different things: one slow owner is worth asking about on
// its own, an answer that ran out while reading one is worth narrowing, and
// a category nobody got to has said nothing about itself either way.
const (
	reasonCategoryBudget = "this category's owner did not answer within its own time budget, so nothing " +
		"was read from it; it is busy rather than broken. Ask for this category on its own"
	reasonAnswerBudget = "the answer's time budget ran out while this category was being read, so " +
		"nothing was read from it. Ask for this category on its own"
	reasonUnreached = "the answer's time budget was spent before this category was reached, " +
		"because an owner ahead of it did not answer in time. Ask for this category on its own"
)

// Catalog is the static answer to "what can be observed". It lists every
// category, always, so a gap in the harness is visible rather than absent.
type Catalog struct {
	Categories []CatalogEntry `json:"categories"`
}

// CatalogEntry is one category's static description.
type CatalogEntry struct {
	Category     Category     `json:"category"`
	Availability Availability `json:"availability"`
	Reason       string       `json:"reason"`
	Keys         []string     `json:"keys"`
}

// Snapshot is what is observed now. Each category carries its own
// ObservedAt: owners are read one after another, so a snapshot spanning
// several of them is explicitly not a globally atomic instant.
type Snapshot struct {
	Category   Category           `json:"category"`
	Mode       string             `json:"mode"`
	Categories []CategorySnapshot `json:"categories"`
	Truncated  bool               `json:"truncated"`
	Partial    bool               `json:"partial"`
	Note       string             `json:"note"`
}

// CategorySnapshot is one category's observation. In ModeOverview only
// Overview is filled; in ModeDetail only Fields is. The other stays an empty
// list rather than disappearing, so the shape never changes between modes.
type CategorySnapshot struct {
	Category     Category        `json:"category"`
	Availability Availability    `json:"availability"`
	Reason       string          `json:"reason"`
	ObservedAt   time.Time       `json:"observed_at"`
	Overview     []OverviewField `json:"overview"`
	Fields       []Field         `json:"fields"`
	Truncated    bool            `json:"truncated"`
}

// OverviewField is one field reduced to what is acting right now: the state
// the effective layer is in and the value it holds. No layers, no provenance,
// no timestamp. It is what an overview is for — the shape of the process in
// one screen, with detail one call away.
//
// Provenance is left out deliberately, and not only because explain is where
// a source belongs. A source ref is a path or a config key, and a hundred of
// them is a large part of an answer that has to fit one budget; leaving them
// out is what makes the row cheap enough for every category to have rows at
// all. The reader who wants to know where a value came from asks about that
// field by name, and gets all three layers with it.
type OverviewField struct {
	Key   string `json:"key"`
	State State  `json:"state"`
	Value Value  `json:"value"`
}

// Explanation is one field with everything known about it. It carries the
// same two closing members a Snapshot does — a flag and a note — because a
// field can be cut just as an answer can, and a reader shown a list with
// items missing and no word about it would read it as the whole list.
type Explanation struct {
	Category     Category     `json:"category"`
	Availability Availability `json:"availability"`
	Reason       string       `json:"reason"`
	ObservedAt   time.Time    `json:"observed_at"`
	Field        Field        `json:"field"`
	Truncated    bool         `json:"truncated"`
	Note         string       `json:"note"`
}

// Registry holds the wired collectors and owns every rule that must hold for
// all of them: the catalog order, sanitization, bounds, detachment, the
// clock, and the promise that one broken collector costs one category rather
// than the whole answer.
type Registry struct {
	now        func() time.Time
	limits     Limits
	home       string
	collectors map[Category]Collector
	sink       func(AuditEvent)
}

// NewRegistry wires collectors into a registry. A nil now falls back to
// time.Now. Registering two collectors for the same category is last-wins:
// construction cannot fail, so a duplicate replaces its predecessor rather
// than producing a registry no one checked the error of. A collector whose
// category is not in the catalog is dropped — the catalog is the contract.
func NewRegistry(now func() time.Time, limits Limits, collectors ...Collector) *Registry {
	if now == nil {
		now = time.Now
	}
	registry := &Registry{
		now:        now,
		limits:     limits.normalized(),
		collectors: make(map[Category]Collector, len(collectors)),
	}
	// Read once, at construction: Collect must not touch the environment.
	if home, err := os.UserHomeDir(); err == nil {
		home = filepath.Clean(home)
		if home != "" && home != string(filepath.Separator) {
			registry.home = home
		}
	}
	for _, collector := range collectors {
		if collector == nil {
			continue
		}
		category := collector.Category()
		if !category.Known() {
			continue
		}
		registry.collectors[category] = collector
	}
	return registry
}

// WithAudit attaches the sink every request reports itself to, and returns
// the registry so wiring stays one expression. The registry itself writes
// nothing anywhere: what a record is worth, and whether one is kept at all,
// is the caller's decision, and a registry with no sink is the default.
//
// Call it while wiring, before the registry is shared. It is not safe to
// swap a sink under a registry that is already answering questions.
func (r *Registry) WithAudit(sink func(AuditEvent)) *Registry {
	if r == nil {
		return nil
	}
	r.sink = sink
	return r
}

// Catalog lists every category with its availability, reason and declared
// keys. It is static: no collector is asked to observe anything, so listing
// what is knowable costs nothing and has no side effects.
func (r *Registry) Catalog() Catalog {
	bounds := newBounder(r.bounds(), r.homeDir())
	entries := make([]CatalogEntry, 0, len(catalogOrder))
	for _, category := range catalogOrder {
		entries = append(entries, r.catalogEntry(category, bounds))
	}
	r.audit(AuditEvent{Action: AuditCatalog, Result: AuditAnswered, Categories: len(entries)})
	return Catalog{Categories: entries}
}

func (r *Registry) catalogEntry(category Category, bounds *bounder) CatalogEntry {
	collector := r.collector(category)
	if collector == nil {
		return CatalogEntry{
			Category:     category,
			Availability: AvailabilityNotImplemented,
			Reason:       reasonNotImplemented,
			Keys:         []string{},
		}
	}
	status := collector.Status()
	reason, _, _ := bounds.text(status.Reason)
	keys := make([]string, 0, len(status.Keys))
	for _, key := range status.Keys {
		text, _, _ := bounds.text(key)
		keys = append(keys, text)
	}
	return CatalogEntry{
		Category:     category,
		Availability: status.Availability,
		Reason:       reason,
		Keys:         keys,
	}
}

// Snapshot observes the process. An empty category is the overview: every
// catalog category, each field reduced to its effective observation. A named
// category is the detail view: that category alone, with all three layers,
// provenance, apply semantics, scope and revision.
//
// Owners are read one after another, each under its own slice of the
// answer's time budget, and each category carries the moment it was read.
// A snapshot spanning several owners is therefore explicitly not one
// instant: it is a sequence of instants, and the timestamps say so.
//
// The result is detached — no slice in it aliases anything a collector owns.
func (r *Registry) Snapshot(ctx context.Context, category Category) (Snapshot, error) {
	event := AuditEvent{Action: AuditSnapshot, Category: category}
	started := time.Now()
	if err := ctx.Err(); err != nil {
		r.audit(event.ended(AuditCanceled, started))
		return Snapshot{}, err
	}
	targets := catalogOrder
	mode := ModeOverview
	if category != "" {
		if !category.Known() {
			r.audit(event.ended(AuditRefused, started))
			return Snapshot{}, unknownCategory(category)
		}
		targets = []Category{category}
		mode = ModeDetail
	}
	event.Mode = mode

	// One deadline for the whole answer, taken from the caller's context so
	// it can only ever narrow it: a turn that is already ending does not get
	// extended by asking a question.
	deadline, cancel := context.WithTimeout(ctx, r.bounds().MaxTotalDuration)
	defer cancel()

	bounds := newBounder(r.bounds(), r.homeDir())
	budget := newPurse(r.bounds().MaxTotalBytes)
	snapshot := Snapshot{
		Category:   category,
		Mode:       mode,
		Categories: make([]CategorySnapshot, 0, len(targets)),
	}
	// The answer's own scaffolding is charged before the first category's
	// turn. The note is part of it and is written last, when there is nothing
	// left to take it out of, so it is paid for up front at its widest.
	budget.spend(answerEnvelope(snapshot))
	for i, target := range targets {
		// The caller stopping is not the same as the budget running out.
		// One is a turn that ended and wants nothing back; the other is an
		// answer worth returning with a hole in it.
		if err := ctx.Err(); err != nil {
			r.audit(event.ended(AuditCanceled, started))
			return Snapshot{}, err
		}
		if deadline.Err() != nil {
			snapshot.Categories = append(snapshot.Categories, r.unreached(targets[i:])...)
			snapshot.Partial = true
			break
		}
		// This category's turn at the byte budget, sized against the
		// categories still to come so its place in the catalog decides
		// nothing about how much it may say.
		budget.open(len(targets) - i)
		entry := r.observe(deadline, target, mode == ModeDetail, bounds, budget)
		if entry.Availability == AvailabilityUnavailable {
			snapshot.Partial = true
		}
		if entry.Truncated {
			snapshot.Truncated = true
		}
		snapshot.Categories = append(snapshot.Categories, entry)
	}
	// Checked once more at the end: a caller that stopped waiting during the
	// last category would otherwise be handed an answer nobody is there for.
	if err := ctx.Err(); err != nil {
		r.audit(event.ended(AuditCanceled, started))
		return Snapshot{}, err
	}
	snapshot.Note = note(snapshot.Truncated, snapshot.Partial)

	event.Partial = snapshot.Partial
	event.Truncated = snapshot.Truncated
	event.Categories = len(snapshot.Categories)
	event.Fields = snapshot.count()
	r.audit(event.ended(AuditAnswered, started))
	return snapshot, nil
}

// unreached is what the categories after a spent budget report. They are
// still listed, and listed in catalog order: a category that vanished from
// an answer would read as a category that does not exist.
func (r *Registry) unreached(targets []Category) []CategorySnapshot {
	out := make([]CategorySnapshot, 0, len(targets))
	for _, target := range targets {
		out = append(out, CategorySnapshot{
			Category:     target,
			Availability: AvailabilityUnavailable,
			Reason:       reasonUnreached,
			ObservedAt:   r.clock(),
			Overview:     []OverviewField{},
			Fields:       []Field{},
		})
	}
	return out
}

// count is how many fields the answer carries, in whichever shape it took.
func (s Snapshot) count() int {
	total := 0
	for _, entry := range s.Categories {
		total += len(entry.Fields) + len(entry.Overview)
	}
	return total
}

// ended stamps how a request finished. The elapsed time is measured on the
// wall clock rather than the registry's, because it is about the wait the
// caller actually had, not about the instant an owner was read at.
func (e AuditEvent) ended(result AuditResult, started time.Time) AuditEvent {
	e.Result = result
	e.Elapsed = time.Since(started)
	return e
}

// observe collects one category and renders it under the response bounds,
// charging the byte budget for what it actually emits.
//
// The category's scaffolding is charged before any row is offered a place
// inside it, so rows compete for the room that will really be left rather
// than for room the name, reason and timestamp are about to take. Each row is
// then charged for the value that is appended, not for an estimate of it:
// what is measured and what is emitted are the same bytes.
func (r *Registry) observe(
	ctx context.Context, category Category, detail bool, bounds *bounder, budget *purse,
) CategorySnapshot {
	entry, fields := r.read(ctx, category, bounds)
	budget.spend(envelope(entry))
	if entry.Availability != AvailabilityAvailable {
		return entry
	}

	limits := r.bounds()
	for i, raw := range fields {
		if i >= limits.MaxFieldsPerCategory {
			entry.Truncated = true
			break
		}
		field, truncated := bounds.field(raw)
		field.ObservedAt = entry.ObservedAt
		if truncated {
			entry.Truncated = true
		}
		if detail {
			if !budget.afford(rowCost(field)) {
				entry.Truncated = true
				break
			}
			entry.Fields = append(entry.Fields, field)
			continue
		}
		row := OverviewField{
			Key:   field.Key,
			State: field.Effective.State,
			Value: field.Effective.Value,
		}
		if !budget.afford(rowCost(row)) {
			entry.Truncated = true
			break
		}
		entry.Overview = append(entry.Overview, row)
	}
	return entry
}

// read observes one category's owner and returns the entry it belongs in
// together with the fields it produced. A collector's error becomes this
// category's unavailable reason and never escapes as a failure of the whole
// snapshot: a snapshot that reported nothing because one owner was busy would
// be the worst possible answer.
//
// The category gets its own deadline, and it is the same mechanism the
// caller's cancellation travels on: nothing is started in the background and
// abandoned here, so a budget that fires ends the work rather than leaving
// it running behind an answer that has already been returned.
//
// It is separate from observe so that every way a category can end up with
// nothing to say — no collector, an unavailable owner, a failed read — leaves
// through one place with its reason already final, which is what lets the
// envelope be charged exactly once and never for a reason that later grew.
func (r *Registry) read(ctx context.Context, category Category, bounds *bounder) (CategorySnapshot, []Field) {
	entry := CategorySnapshot{
		Category:   category,
		ObservedAt: r.clock(),
		Overview:   []OverviewField{},
		Fields:     []Field{},
	}
	collector := r.collector(category)
	if collector == nil {
		entry.Availability = AvailabilityNotImplemented
		entry.Reason = reasonNotImplemented
		return entry, nil
	}
	status := collector.Status()
	entry.Availability = status.Availability
	entry.Reason, _, _ = bounds.text(status.Reason)
	if status.Availability != AvailabilityAvailable {
		return entry, nil
	}

	own, cancel := context.WithTimeout(ctx, r.bounds().MaxCategoryDuration)
	defer cancel()
	fields, err := collector.Collect(own)
	if err != nil {
		entry.Availability = AvailabilityUnavailable
		entry.Reason = observeFailure(ctx, own, err, bounds)
		return entry, nil
	}
	return entry, fields
}

// Explain answers one field with everything: all three layers, where each
// came from, what it would take to change it, and when it was observed. The
// key is validated against the collector's declared keys before anything is
// collected, so a typo costs an error rather than an observation.
func (r *Registry) Explain(ctx context.Context, category Category, key string) (Explanation, error) {
	event := AuditEvent{Action: AuditExplain, Category: category}
	started := time.Now()
	if err := ctx.Err(); err != nil {
		r.audit(event.ended(AuditCanceled, started))
		return Explanation{}, err
	}
	bounds := newBounder(r.bounds(), r.homeDir())
	if !category.Known() {
		r.audit(event.ended(AuditRefused, started))
		return Explanation{}, unknownCategory(category)
	}
	safeKey, _, _ := bounds.text(strings.TrimSpace(key))
	event.Key = safeKey
	collector := r.collector(category)
	if collector == nil {
		r.audit(event.ended(AuditRefused, started))
		return Explanation{}, fmt.Errorf(
			"diag: category %q is not implemented yet (%s); action=catalog lists what is observable",
			category, reasonNotImplemented)
	}
	status := collector.Status()
	if status.Availability != AvailabilityAvailable {
		reason, _, _ := bounds.text(status.Reason)
		r.audit(event.ended(AuditRefused, started))
		return Explanation{}, fmt.Errorf("diag: category %q is %s: %s", category, status.Availability, reason)
	}
	if !slices.Contains(status.Keys, safeKey) {
		r.audit(event.ended(AuditRefused, started))
		return Explanation{}, fmt.Errorf(
			"diag: category %q has no field %q; it declares: %s",
			category, safeKey, strings.Join(status.Keys, ", "))
	}

	// One category, so the whole answer's budget and the category's are the
	// same wait; the narrower of the two bounds it.
	own, cancel := context.WithTimeout(ctx, r.bounds().MaxCategoryDuration)
	defer cancel()
	fields, err := collector.Collect(own)
	if err != nil {
		r.audit(event.ended(explainResult(ctx), started))
		return Explanation{}, fmt.Errorf(
			"diag: category %q could not be observed: %s", category, observeFailure(ctx, own, err, bounds))
	}
	observedAt := r.clock()
	reason, _, _ := bounds.text(status.Reason)
	for _, raw := range fields {
		if raw.Key != safeKey {
			continue
		}
		field, truncated := bounds.field(raw)
		field.ObservedAt = observedAt
		event.Fields = 1
		event.Categories = 1
		event.Truncated = truncated
		r.audit(event.ended(AuditAnswered, started))
		explanation := Explanation{
			Category:     category,
			Availability: AvailabilityAvailable,
			Reason:       reason,
			ObservedAt:   observedAt,
			Field:        field,
			Truncated:    truncated,
		}
		if truncated {
			explanation.Note = noteFieldCut
		}
		return explanation, nil
	}
	r.audit(event.ended(AuditFailed, started))
	return Explanation{}, fmt.Errorf(
		"diag: category %q declares field %q but did not observe it", category, safeKey)
}

// explainResult tells a caller that stopped waiting apart from everything
// else. Nothing went wrong in that case, so it is not recorded as a failure;
// a collector that errored and one that ran out of time both are.
func explainResult(outer context.Context) AuditResult {
	if outer.Err() != nil {
		return AuditCanceled
	}
	return AuditFailed
}

// observeFailure says why a category has nothing to show. A collector that
// returned an error gets its message sanitized and reported; one that ran
// out of time gets the budget reason instead, because "context deadline
// exceeded" tells a reader nothing about which budget or what to do next.
func observeFailure(outer, own context.Context, err error, bounds *bounder) string {
	switch {
	case own.Err() == nil:
		return collectorReason(err, bounds)
	case outer.Err() != nil:
		return reasonAnswerBudget
	default:
		return reasonCategoryBudget
	}
}

// collector returns the wired collector for a category, nil-safe on the
// registry so a missing capability degrades instead of panicking.
func (r *Registry) collector(category Category) Collector {
	if r == nil {
		return nil
	}
	return r.collectors[category]
}

func (r *Registry) bounds() Limits {
	if r == nil {
		return DefaultLimits()
	}
	return r.limits
}

func (r *Registry) homeDir() string {
	if r == nil {
		return ""
	}
	return r.home
}

func (r *Registry) clock() time.Time {
	if r == nil || r.now == nil {
		return time.Now()
	}
	return r.now()
}

// collectorReason turns a collector's error into a safe category reason.
// Error text is arbitrary and may quote whatever the owner was holding, so
// it goes through the same masking and bounding as any value.
func collectorReason(err error, bounds *bounder) string {
	if err == nil {
		return ""
	}
	text, _, _ := bounds.text(err.Error())
	if text == "" {
		return "the collector failed without a message"
	}
	return text
}

func unknownCategory(category Category) error {
	names := make([]string, 0, len(catalogOrder))
	for _, known := range catalogOrder {
		names = append(names, string(known))
	}
	safe := stripControl(string(category))
	if len(safe) > 64 {
		safe = cutBytes(safe, 64)
	}
	return fmt.Errorf("diag: unknown category %q; known categories: %s", safe, strings.Join(names, ", "))
}

func note(truncated, partial bool) string {
	var notes []string
	if truncated {
		notes = append(notes, noteTruncated)
	}
	if partial {
		notes = append(notes, notePartial)
	}
	return strings.Join(notes, "; ")
}
