package diag

import (
	"context"
	"strconv"
	"time"
)

// Watch field keys. Watches take their own namespace inside the diagnostics
// category: a key is a stable address for Explain, and what else this
// category grows to cover grows independently of them.
const (
	KeyWatchesState    = "watches.state"
	KeyWatchesLimits   = "watches.limits"
	KeyWatchesCount    = "watches.count"
	KeyWatchesShapes   = "watches.shapes"
	KeyWatchesCadence  = "watches.cadence"
	KeyWatchesEvents   = "watches.events"
	KeyWatchesOutcomes = "watches.outcomes"
)

// WatchLifecycle is where this session's watches stand. The values separate
// the things "no watch reported anything" could otherwise mean: a process
// shape that has no watch manager at all, a manager with nothing running,
// one with something running, and one that cannot start another.
type WatchLifecycle string

// WatchLifecycle values.
const (
	// WatchesSupported means this build carries watches. It is the
	// configured layer's answer and says nothing about whether this process
	// has a manager.
	WatchesSupported WatchLifecycle = "supported"
	// WatchesManaged means a watch manager is in force for this session.
	WatchesManaged WatchLifecycle = "managed"
	// WatchesNoManager means no watch manager is in force. A headless run
	// has none: nothing can start a watch there and no event can reach a
	// turn.
	WatchesNoManager WatchLifecycle = "no_manager"
	// WatchesIdle means a manager is in force with nothing running.
	WatchesIdle WatchLifecycle = "idle"
	// WatchesRunning means at least one watch is running with room for
	// another.
	WatchesRunning WatchLifecycle = "running"
	// WatchesFull means every live slot is taken, so the next start is
	// refused rather than queued.
	WatchesFull WatchLifecycle = "full"
)

// WatchShape is which of the three kinds of watch one is. The shape falls
// out of the two fields that decide it — whether it has a command, and
// whether it has an interval — and it is the one thing about a watch that
// can be reported without repeating what it was pointed at.
type WatchShape string

// WatchShape values, in the order the package doc introduces them.
const (
	// WatchStream is a command with no interval: every matching line is an
	// event, or one event when the command exits.
	WatchStream WatchShape = "stream"
	// WatchPoll is a command with an interval: it runs on each tick and
	// emits when the output changes.
	WatchPoll WatchShape = "poll"
	// WatchTimer is an interval with no command: the label comes back on
	// each tick.
	WatchTimer WatchShape = "timer"
)

// WatchTrigger is what counts as an event from a streaming command. A poll
// and a timer are not triggered by a line at all, so neither carries one.
type WatchTrigger string

// WatchTrigger values.
const (
	// WatchOnLine means every matching line of the command's output is an
	// event. It is also what a streaming command with nothing said about it
	// does.
	WatchOnLine WatchTrigger = "line"
	// WatchOnExit means the command produces exactly one event, when it
	// finishes. A watch like that being quiet is the command still running,
	// not the watch being broken.
	WatchOnExit WatchTrigger = "exit"
)

// WatchOutcome is what became of one watch. It is a category rather than a
// message: a watch that ended badly records an error that quotes the command
// it ran, and none of that may leave the owner.
type WatchOutcome string

// WatchOutcome values.
const (
	// WatchRunning means the watch has not finished.
	WatchRunning WatchOutcome = "running"
	// WatchEnded means it finished or was stopped, with nothing wrong.
	WatchEnded WatchOutcome = "ended"
	// WatchFailed means its command failed. Why is not reported.
	WatchFailed WatchOutcome = "failed"
	// WatchFlooded means it stopped itself after crossing the event budget
	// every watch of this session draws on together.
	WatchFlooded WatchOutcome = "flooded"
)

// WatchFacts is what the harness may say about one watch. It is an allowlist
// by construction: a shape, a trigger, an interval, a count and an outcome —
// with no member for the label, the command, the match expression, the text
// of an event or the text of an error to land in.
type WatchFacts struct {
	// Shape is which of the three kinds of watch this one is.
	Shape WatchShape
	// Trigger is what counts as an event from a streaming command. Empty for
	// a watch that is not one.
	Trigger WatchTrigger
	// Every is the interval a poll or a timer ticks on. Zero for a stream.
	Every time.Duration
	// Events is how many events this watch has produced. How many, never
	// what: an event carries a line of command output.
	Events int
	// Live is whether it is still running.
	Live bool
	// Outcome is what became of it.
	Outcome WatchOutcome
}

// WatchState is one observation of this session's watches: whether a manager
// is in force at all, the budgets every watch is held to, and one entry per
// watch this session started.
//
// Watches are process-scoped and nothing is persisted, so this describes
// what is in memory and there is nothing else to describe. A manager belongs
// to one session, so no other session's watches are reachable from here.
type WatchState struct {
	// Known is false when nobody published an observation — the layer is not
	// wired, rather than wired and empty.
	Known bool
	// Managed is whether a watch manager is in force for this session.
	Managed bool
	// MaxLive bounds how many watches run at once.
	MaxLive int
	// MinInterval floors a polling watch.
	MinInterval time.Duration
	// FloodLimit and FloodWindow are the event budget every watch of this
	// session draws on together.
	FloodLimit  int
	FloodWindow time.Duration
	// MaxPerDelivery bounds how many events reach the model at once.
	MaxPerDelivery int
	// EventTextLimit truncates one event.
	EventTextLimit int
	// Watches is one entry per watch this session started, live or not.
	Watches []WatchFacts
	// Revision fingerprints the state this observation describes, so two
	// snapshots taken across a start are visibly of two different states.
	Revision string
}

// Sources for the layers whose origin is the view's own reading rather than
// an owner's record.
var (
	sourceWatchesBuild = Source{
		Kind: SourceBuild,
		Ref:  "this build carries watches; nothing in the configuration switches them on or off",
	}
	sourceWatchesManager = Source{
		Kind: SourceSession,
		Ref:  "whether this session holds a watch manager; a headless run holds none",
	}
	sourceWatchesLive = Source{
		Kind: SourceSession,
		Ref:  "what this session's watches are doing; asking starts, stops and waits for none of them",
	}
	sourceWatchesNoManager = Source{
		Kind: SourceSession,
		Ref: "no watch manager is in force here — a headless run has none, so nothing can start a watch " +
			"and no event can reach a turn",
	}
	sourceWatchesBudgets = Source{
		Kind: SourceBuild,
		Ref:  "the budgets every watch is held to, compiled in rather than configured",
	}
	sourceWatchesBudgetsFixed = Source{
		Kind: SourceComputed,
		Ref:  "nothing loads these: no configuration file, environment entry or session setting changes one",
	}
	sourceWatchesSlotsFree = Source{
		Kind: SourceComputed,
		Ref:  "how many more watches this session could start before the live cap refuses the next one",
	}
	sourceWatchesUnplanned = Source{
		Kind: SourceComputed,
		Ref:  "nothing configures how many watches exist; one exists because a turn started it",
	}
	sourceWatchesStarted = Source{
		Kind: SourceSession,
		Ref:  "every watch this session started, live or not; a watch is process-scoped and nothing is persisted",
	}
	sourceWatchesStillLive = Source{
		Kind: SourceSession,
		Ref:  "how many of them are still running",
	}
	sourceWatchesShapeVocabulary = Source{
		Kind: SourceBuild,
		Ref: "the shapes a watch can take, decided by whether it has a command and whether it has " +
			"an interval, with the streaming one split by what triggers it",
	}
	sourceWatchesShapesStarted = Source{
		Kind: SourceSession,
		Ref: "this session's watches by shape. The shape only — no label, command, match expression or " +
			"working directory",
	}
	sourceWatchesShapesLive = Source{
		Kind: SourceSession,
		Ref:  "the same count over the ones still running",
	}
	sourceWatchesIntervalFloor = Source{
		Kind: SourceBuild,
		Ref:  "the shortest interval a watch may ask for; a tighter one is refused rather than rounded up",
	}
	sourceWatchesTightest = Source{
		Kind: SourceSession,
		Ref:  "the shortest interval among this session's watches — how often the busiest of them ticks",
	}
	sourceWatchesTightestLive = Source{
		Kind: SourceSession,
		Ref:  "the same among the ones still running: the cadence this session is still paying for",
	}
	sourceWatchesNoInterval = Source{
		Kind: SourceComputed,
		Ref: "no watch here ticks on an interval; a streaming command reports when its output does, " +
			"not on a clock",
	}
	sourceWatchesEventsUnplanned = Source{
		Kind: SourceComputed,
		Ref:  "nothing configures what a watch reports; an event happens because the thing being watched did",
	}
	sourceWatchesEventsAll = Source{
		Kind: SourceSession,
		Ref: "how many events this session's watches have produced. How many, never what: an event carries " +
			"a line of command output",
	}
	sourceWatchesEventsLive = Source{
		Kind: SourceSession,
		Ref:  "how many of them came from watches still running",
	}
	sourceWatchesOutcomeVocabulary = Source{
		Kind: SourceBuild,
		Ref:  "what a watch can come to",
	}
	sourceWatchesOutcomes = Source{
		Kind: SourceSession,
		Ref:  "this session's watches by outcome",
	}
	sourceWatchesBadEndings = Source{
		Kind: SourceComputed,
		Ref: "how many of them stopped because something went wrong rather than because they finished or " +
			"were stopped. Why is never reported: a watch's error quotes the command it ran",
	}
)

// lifecycle separates the things "no watch reported anything" could mean: a
// process shape with no manager at all, a manager with nothing running, and
// a manager that cannot start another. The first is not a fault and the last
// is the answer a refused start is looking for.
func (s WatchState) lifecycle() Field {
	field := s.field(KeyWatchesState, ApplyRestart, ScopeSession)
	if !s.Known {
		return field
	}
	managed := WatchesManaged
	if !s.Managed {
		managed = WatchesNoManager
	}
	field.Configured = Present(StringValue(string(WatchesSupported)), sourceWatchesBuild)
	field.Loaded = Present(StringValue(string(managed)), sourceWatchesManager)
	field.Effective = Present(StringValue(string(s.liveState())), sourceWatchesLive)
	return field
}

// limits is what a watch is held to and what is left of it. The budgets are
// compiled in, so the loaded layer does not exist rather than repeating
// them; what a reader actually wants from this field is the last line — how
// many more watches this session could start.
func (s WatchState) limits() Field {
	field := s.field(KeyWatchesLimits, ApplyRestart, ScopeProcess)
	if !s.Known {
		return field
	}
	field.Configured = Present(ListValue(s.budgets()), sourceWatchesBudgets)
	field.Loaded = notApplicable(sourceWatchesBudgetsFixed)
	if !s.Managed {
		field.Effective = notApplicable(sourceWatchesNoManager)
		return field
	}
	field.Effective = Present(IntValue(int64(max(s.MaxLive-s.live(), 0))), sourceWatchesSlotsFree)
	return field
}

// count is how many watches this session started and how many are still
// running. The gap between them is every watch that finished, failed or
// stopped itself, which the outcomes field breaks down.
func (s WatchState) count() Field {
	field := s.field(KeyWatchesCount, ApplyImmediate, ScopeSession)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = notApplicable(sourceWatchesUnplanned)
	field.Loaded = Present(IntValue(int64(len(s.Watches))), sourceWatchesStarted)
	field.Effective = Present(IntValue(int64(s.live())), sourceWatchesStillLive)
	return field
}

// shapes is what kinds of watch this session started and what kinds are
// still running. A shape is the one thing about a watch that can be reported
// without repeating what it was pointed at: the label and the command are
// whatever a turn typed, and neither leaves here.
func (s WatchState) shapes() Field {
	field := s.field(KeyWatchesShapes, ApplyImmediate, ScopeSession)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = Present(ListValue(watchShapeLabels()), sourceWatchesShapeVocabulary)
	field.Loaded = Present(ListValue(s.countShapes(func(WatchFacts) bool { return true })), sourceWatchesShapesStarted)
	field.Effective = Present(
		ListValue(s.countShapes(func(w WatchFacts) bool { return w.Live })),
		sourceWatchesShapesLive,
	)
	return field
}

// cadence is how often this session wakes itself up. A poll and a timer both
// carry an interval, and the tightest one in force is what a session that
// feels busy is actually paying for. The floor below it is the one thing
// about an interval a turn cannot argue with.
func (s WatchState) cadence() Field {
	field := s.field(KeyWatchesCadence, ApplyImmediate, ScopeSession)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = Present(DurationValue(s.MinInterval), sourceWatchesIntervalFloor)
	field.Loaded = s.tightest(func(WatchFacts) bool { return true }, sourceWatchesTightest)
	field.Effective = s.tightest(func(w WatchFacts) bool { return w.Live }, sourceWatchesTightestLive)
	return field
}

// events is how much this session's watches have had to say and how much of
// that is still arriving. Counts only: an event carries a line of command
// output, and the text of one never leaves the manager.
func (s WatchState) events() Field {
	field := s.field(KeyWatchesEvents, ApplyImmediate, ScopeSession)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = notApplicable(sourceWatchesEventsUnplanned)
	field.Loaded = Present(
		IntValue(int64(s.countEvents(func(WatchFacts) bool { return true }))),
		sourceWatchesEventsAll,
	)
	field.Effective = Present(
		IntValue(int64(s.countEvents(func(w WatchFacts) bool { return w.Live }))),
		sourceWatchesEventsLive,
	)
	return field
}

// outcomes is what became of this session's watches, and how many of them
// came to a bad end. A watch that failed and a watch that stopped itself for
// flooding are different problems with different fixes, and neither is the
// same as one that simply finished.
func (s WatchState) outcomes() Field {
	field := s.field(KeyWatchesOutcomes, ApplyImmediate, ScopeSession)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = Present(ListValue(watchOutcomeVocabulary()), sourceWatchesOutcomeVocabulary)
	field.Loaded = Present(ListValue(s.countOutcomes()), sourceWatchesOutcomes)
	field.Effective = Present(IntValue(int64(s.badEndings())), sourceWatchesBadEndings)
	return field
}

// field is the shape every watch field starts from: all three layers
// unavailable, so a layer nobody wired degrades into an honest answer rather
// than into a zero that would read as "this session started no watch".
func (s WatchState) field(key string, apply Apply, scope Scope) Field {
	return Field{
		Key:        key,
		Configured: Unavailable(),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      apply,
		Scope:      scope,
		Revision:   s.Revision,
	}
}

// absent reports whether a question about this session's watches can be
// answered at all, and what to say when it cannot. A layer nobody wired
// knows nothing; a process shape with no watch manager is a different answer
// entirely, and reporting it as an empty list would read as a session that
// simply has not started one.
func (s WatchState) absent() (Observation, bool) {
	switch {
	case !s.Known:
		return Unavailable(), true
	case !s.Managed:
		return notApplicable(sourceWatchesNoManager), true
	default:
		return Observation{}, false
	}
}

// liveState is what this session's watches amount to right now, answered in
// the order the answers stop mattering: no manager beats everything, a full
// session beats the fact that something is running in it, and something
// running beats nothing.
func (s WatchState) liveState() WatchLifecycle {
	live := s.live()
	switch {
	case !s.Managed:
		return WatchesNoManager
	case s.MaxLive > 0 && live >= s.MaxLive:
		return WatchesFull
	case live > 0:
		return WatchesRunning
	default:
		return WatchesIdle
	}
}

// budgets renders the compiled-in limits as "name=value", in the order the
// package doc introduces them.
func (s WatchState) budgets() []string {
	return []string{
		"live=" + strconv.Itoa(s.MaxLive),
		"interval_floor=" + s.MinInterval.String(),
		"events=" + strconv.Itoa(s.FloodLimit) + "/" + s.FloodWindow.String(),
		"per_delivery=" + strconv.Itoa(s.MaxPerDelivery),
		"event_text=" + strconv.Itoa(s.EventTextLimit),
	}
}

// live is how many of this session's watches are still running.
func (s WatchState) live() int {
	n := 0
	for _, w := range s.Watches {
		if w.Live {
			n++
		}
	}
	return n
}

// countShapes tallies the matching watches by shape, in the vocabulary's own
// order so two snapshots of one state answer the same way.
func (s WatchState) countShapes(keep func(WatchFacts) bool) []string {
	counts := make(map[string]int, len(s.Watches))
	for _, w := range s.Watches {
		if keep(w) {
			counts[shapeLabel(w)]++
		}
	}
	out := make([]string, 0, len(counts))
	for _, label := range watchShapeLabels() {
		if counts[label] > 0 {
			out = append(out, label+"="+strconv.Itoa(counts[label]))
		}
	}
	return out
}

// countOutcomes tallies every watch by outcome, in the vocabulary's own
// order.
func (s WatchState) countOutcomes() []string {
	counts := make(map[WatchOutcome]int, len(s.Watches))
	for _, w := range s.Watches {
		counts[w.Outcome]++
	}
	out := make([]string, 0, len(counts))
	for _, outcome := range watchOutcomes() {
		if counts[outcome] > 0 {
			out = append(out, string(outcome)+"="+strconv.Itoa(counts[outcome]))
		}
	}
	return out
}

// countEvents sums the events of the matching watches.
func (s WatchState) countEvents(keep func(WatchFacts) bool) int {
	n := 0
	for _, w := range s.Watches {
		if keep(w) {
			n += w.Events
		}
	}
	return n
}

// tightest is the shortest interval among the matching watches. A stream
// carries none at all, so a session running only streams reports no cadence
// rather than a zero that would read as "constantly".
func (s WatchState) tightest(keep func(WatchFacts) bool, source Source) Observation {
	var found time.Duration
	for _, w := range s.Watches {
		if !keep(w) || w.Every <= 0 {
			continue
		}
		if found == 0 || w.Every < found {
			found = w.Every
		}
	}
	if found == 0 {
		return notApplicable(sourceWatchesNoInterval)
	}
	return Present(DurationValue(found), source)
}

// badEndings is how many watches stopped because something went wrong.
func (s WatchState) badEndings() int {
	n := 0
	for _, w := range s.Watches {
		if w.Outcome == WatchFailed || w.Outcome == WatchFlooded {
			n++
		}
	}
	return n
}

// shapeLabel is how one watch is counted: its shape, and for a streaming
// command the trigger too. A stream that reports on exit produces exactly one
// event, when the command finishes — telling it apart from one that reports
// every line is the difference between a watch that is quiet and a watch that
// is broken.
func shapeLabel(w WatchFacts) string {
	if w.Shape != WatchStream {
		return string(w.Shape)
	}
	trigger := w.Trigger
	if trigger == "" {
		trigger = WatchOnLine
	}
	return string(w.Shape) + ":" + string(trigger)
}

// watchShapeLabels is the counting vocabulary in canonical order: the three
// shapes, with the streaming one split by what triggers it.
func watchShapeLabels() []string {
	return []string{
		string(WatchStream) + ":" + string(WatchOnLine),
		string(WatchStream) + ":" + string(WatchOnExit),
		string(WatchPoll),
		string(WatchTimer),
	}
}

// watchOutcomes is the outcome vocabulary in canonical order.
func watchOutcomes() []WatchOutcome {
	return []WatchOutcome{WatchRunning, WatchEnded, WatchFailed, WatchFlooded}
}

// watchOutcomeVocabulary is the outcome vocabulary as plain strings.
func watchOutcomeVocabulary() []string {
	outcomes := watchOutcomes()
	out := make([]string, 0, len(outcomes))
	for _, outcome := range outcomes {
		out = append(out, string(outcome))
	}
	return out
}

// diagnosticsReason states what this category answers and what it
// deliberately leaves out.
const diagnosticsReason = "what this session has watching the world for it and what the process " +
	"observes about itself. Watches: whether a manager " +
	"is in force at all, the budgets every watch is held to, how many watches this session started " +
	"and how many are still running, what shape each takes, how often the busiest of them ticks, how many " +
	"events they have produced and " +
	"what became of them. " +
	"The process itself: whether debug logging is switched on and what it latched to, the name of the " +
	"file lines land in, whether plan telemetry is being counted and where the counters can be read, " +
	"which subsystem logs the environment redirects, " +
	"whether a profiling endpoint was asked for and how far it reaches, the limits this view " +
	"answers under, and the output form, round ceiling and deadline a headless run was started " +
	"under. " +
	"Shapes, counts and outcomes only — no label, command, match expression, working directory, " +
	"event text or error text, no line of the log, no log directory, no profile and no address. " +
	"Nothing here starts a " +
	"watch, stops one, waits for one or reads its " +
	"log, nothing opens the log file or connects to the profiling endpoint, and a headless run reports " +
	"no manager rather than an empty list"

// DiagnosticDeps binds the diagnostics collector to the owners of what this
// session observes on its own. Each is optional: a process without one
// reports that layer unavailable rather than making the category fail.
type DiagnosticDeps struct {
	// Watches observes the watch manager through the owner's own projection.
	// It is read, never exercised: no accessor here may start a watch, stop
	// one, subscribe to one, wait for one or read its log.
	Watches func() WatchState
	// Logging observes the debug log's switch and destination. It reads the
	// owner's state and the environment; it never opens the file, writes a
	// line, or reads one back.
	Logging func() LoggingFacts
	// Telemetry observes whether the plan is being counted. It reports the
	// mechanism, not the numbers — those are the plan category's answer.
	Telemetry func() TelemetryFacts
	// Profiling observes the pprof endpoint as this process recorded it at
	// start. It never connects to the endpoint or fetches a profile.
	Profiling func() ProfilingFacts
	// Headless observes the ceilings a headless run was started under. A
	// terminal session publishes an observation that says it is not a run,
	// so the ceilings read as not applicable rather than as zeros.
	Headless func() HeadlessFacts
	// Response is the limits this view answers under, as the wiring asked
	// for them. It is a value rather than an accessor because they are fixed
	// when the registry is built and nothing can change them afterwards.
	Response Limits
}

// diagnosticCollector observes what this session watches for itself.
type diagnosticCollector struct {
	deps DiagnosticDeps
}

// NewDiagnosticCollector builds the diagnostics collector.
func NewDiagnosticCollector(deps DiagnosticDeps) Collector {
	return &diagnosticCollector{deps: deps}
}

func (*diagnosticCollector) Category() Category { return CategoryDiagnostics }

// Status is answered from the declared key set alone: listing the catalog
// reaches no manager and reads no watch. The keys are static for the same
// reason — a key per watch would make the catalog depend on what a turn
// started, and would address one by a label this view does not report.
func (*diagnosticCollector) Status() Status {
	return Status{
		Availability: AvailabilityAvailable,
		Reason:       diagnosticsReason,
		Keys: []string{
			KeyWatchesState,
			KeyWatchesLimits,
			KeyWatchesCount,
			KeyWatchesShapes,
			KeyWatchesCadence,
			KeyWatchesEvents,
			KeyWatchesOutcomes,
			KeyLoggingState,
			KeyLoggingDestination,
			KeyLoggingSubsystems,
			KeyTelemetryState,
			KeyTelemetryExport,
			KeyProfilingState,
			KeyHarnessLimits,
			KeyHeadlessOutput,
			KeyHeadlessRounds,
			KeyHeadlessTimeout,
		},
	}
}

// Collect reads each owner once and derives its fields from that one read,
// so the fields about one owner describe one moment rather than several.
// The owners are read in the order the fields are listed, which is also the
// order they cost anything: three of them are a mutex and a map lookup.
func (c *diagnosticCollector) Collect(_ context.Context) ([]Field, error) {
	watches := callWatchState(c.deps.Watches)
	logging := callLoggingFacts(c.deps.Logging)
	telemetry := callTelemetryFacts(c.deps.Telemetry)
	profiling := callProfilingFacts(c.deps.Profiling)
	headless := callHeadlessFacts(c.deps.Headless)
	return []Field{
		watches.lifecycle(),
		watches.limits(),
		watches.count(),
		watches.shapes(),
		watches.cadence(),
		watches.events(),
		watches.outcomes(),
		logging.loggingState(),
		logging.loggingDestination(),
		logging.loggingSubsystems(),
		telemetry.telemetryState(),
		telemetry.telemetryExport(),
		profiling.profilingState(),
		harnessLimits(c.deps.Response),
		headless.output(),
		headless.rounds(),
		headless.timeout(),
	}, nil
}

// callWatchState reads the optional accessor. A nil accessor is a wiring
// gap, and every layer it feeds reports unavailable rather than crashing the
// snapshot.
func callWatchState(accessor func() WatchState) WatchState {
	if accessor == nil {
		return WatchState{}
	}
	return accessor()
}

// callLoggingFacts reads the optional accessor, on the same terms.
func callLoggingFacts(accessor func() LoggingFacts) LoggingFacts {
	if accessor == nil {
		return LoggingFacts{}
	}
	return accessor()
}

// callTelemetryFacts reads the optional accessor, on the same terms.
func callTelemetryFacts(accessor func() TelemetryFacts) TelemetryFacts {
	if accessor == nil {
		return TelemetryFacts{}
	}
	return accessor()
}

// callHeadlessFacts reads the optional accessor, on the same terms.
func callHeadlessFacts(accessor func() HeadlessFacts) HeadlessFacts {
	if accessor == nil {
		return HeadlessFacts{}
	}
	return accessor()
}

// callProfilingFacts reads the optional accessor, on the same terms.
func callProfilingFacts(accessor func() ProfilingFacts) ProfilingFacts {
	if accessor == nil {
		return ProfilingFacts{}
	}
	return accessor()
}
