package diag

import (
	"strconv"
	"time"
)

// Process field keys. They sit beside the watch keys in the diagnostics
// category: watches are what this session observes for the user, and these
// are what the process observes about itself.
const (
	KeyLoggingState       = "logging.state"
	KeyLoggingDestination = "logging.destination"
	KeyLoggingSubsystems  = "logging.subsystems"
	KeyTelemetryState     = "telemetry.state"
	KeyTelemetryExport    = "telemetry.export"
	KeyProfilingState     = "profiling.state"
	KeyHarnessLimits      = "harness.limits"
	KeyHeadlessOutput     = "headless.output"
	KeyHeadlessRounds     = "headless.rounds"
	KeyHeadlessTimeout    = "headless.timeout"
)

// LoggingFacts is what the harness may say about debug logging: whether the
// switch is set, whether it has been read yet, and the name — the name only —
// of the file lines land in.
//
// Nothing here is read out of the log. The switch and the destination are
// what the owner holds; what was written is the owner's business and stays
// there, because a debug line quotes whatever the process was doing.
type LoggingFacts struct {
	// Known is false when nobody published an observation — the layer is not
	// wired, rather than wired and off.
	Known bool
	// Requested is what the environment says right now.
	Requested bool
	// Latched is whether the switch has been read yet. It is read once and
	// remembered, so an environment changed afterwards is configured one way
	// and acting another — which is the whole reason this field has layers.
	Latched bool
	// Enabled is what the switch latched to. Meaningless until Latched.
	Enabled bool
	// Destination is the base name of the file lines would land in. The
	// name only: the directory it sits in is somebody's home directory or a
	// path a script chose, and neither is needed to answer the question.
	Destination string
	// FromEnv is whether the destination was named by the environment rather
	// than being the compiled-in default.
	FromEnv bool
	// OpenName is the base name of the file actually open, empty when none
	// is. A log that is on but has had nothing to say has no file open yet.
	OpenName string
	// MCPLogFromEnv is whether the environment redirects the MCP server
	// logs. Presence only: the directory it names is a path somebody chose,
	// and it is never reported.
	MCPLogFromEnv bool
	// PlanGateLogFromEnv is whether the environment redirects the plan-gate
	// log, on the same terms.
	PlanGateLogFromEnv bool
	// Revision fingerprints the state this observation describes.
	Revision string
}

// Subsystem log names, as logging.subsystems lists them.
const (
	LogSubsystemMCP      = "mcp"
	LogSubsystemPlanGate = "plan_gate"
)

// HeadlessFacts is what the harness may say about the ceilings a headless
// run was started under: how it prints, how many tool rounds a turn may
// take, and how long the whole run may last. They are flags of cozyphi run
// and nothing else — a terminal session has no such flags, and says so.
type HeadlessFacts struct {
	// Known is false when nobody published an observation.
	Known bool
	// Run is whether this process is a headless run at all. A terminal
	// surface publishes Known and not Run, so the ceilings read as not
	// applicable rather than as zeros somebody set.
	Run bool
	// JSONL is whether the run prints one event per line rather than text.
	JSONL bool
	// MaxRounds is the tool-round ceiling asked for, zero when none was.
	MaxRounds int
	// Timeout is the deadline asked for, zero when none was.
	Timeout time.Duration
	// Revision fingerprints the state this observation describes.
	Revision string
}

// TelemetryState is where plan telemetry stands. The values separate a build
// that carries the mechanism from a session that holds one and from a
// session that does not — three answers "no telemetry" would otherwise blur.
type TelemetryState string

// TelemetryState values.
const (
	// TelemetrySupported means this build carries plan telemetry. It is the
	// configured layer's answer and says nothing about this session.
	TelemetrySupported TelemetryState = "supported"
	// TelemetryTracked means a tracker is in force for this session.
	TelemetryTracked TelemetryState = "tracked"
	// TelemetryUntracked means none is. Records are dropped and the counters
	// read as zero, which is telemetry switched off rather than broken.
	TelemetryUntracked TelemetryState = "untracked"
)

// TelemetryFacts is what the harness may say about plan telemetry. It is
// about the mechanism, not the numbers: how many counters exist, whether a
// tracker is in force, and where the counters can be read. What they count
// is the plan category's answer and is not repeated here.
type TelemetryFacts struct {
	// Known is false when nobody published an observation.
	Known bool
	// Tracked is whether a tracker is in force for this session.
	Tracked bool
	// Counters is how wide the telemetry surface is. The schema is fixed and
	// numeric-only, so this is also the leak contract: no string, map or
	// slice field exists for plan text to be recorded through.
	Counters int
	// Readable names where the counters can be read inside this process.
	// It is not a list of exporters: nothing exports them.
	Readable []string
	// Revision fingerprints the state this observation describes.
	Revision string
}

// ProfilingExposure is how far the profiling endpoint reaches. It is derived
// from the address without reporting it: how exposed the endpoint is is the
// question worth asking, and the host and port are not needed to answer it.
type ProfilingExposure string

// ProfilingExposure values.
const (
	// ProfilingLoopback means the endpoint is bound to this machine only.
	ProfilingLoopback ProfilingExposure = "loopback"
	// ProfilingEveryInterface means it is bound to every interface, so
	// anything that can reach this host can read the process's profiles.
	ProfilingEveryInterface ProfilingExposure = "every_interface"
	// ProfilingNamedHost means it is bound to one named address that is not
	// loopback.
	ProfilingNamedHost ProfilingExposure = "named_host"
)

// ProfilingLifecycle is what became of the endpoint.
type ProfilingLifecycle string

// ProfilingLifecycle values.
const (
	// ProfilingOff means nothing asked for an endpoint.
	ProfilingOff ProfilingLifecycle = "off"
	// ProfilingServing means the listener came up and is serving.
	ProfilingServing ProfilingLifecycle = "serving"
	// ProfilingStopped means it was asked for and is not serving — the
	// address was taken or malformed, or the listener ended. Why is not
	// reported: the error quotes the address.
	ProfilingStopped ProfilingLifecycle = "stopped"
)

// ProfilingFacts is what the harness may say about the pprof endpoint:
// whether one was asked for, how exposed it is, and whether it is up.
//
// The address never travels — not the host, not the port, not the URL a
// startup line printed. Neither does a profile: answering this reads no
// handler and fetches nothing.
type ProfilingFacts struct {
	// Known is false when nobody published an observation.
	Known bool
	// Requested is whether the environment named an address.
	Requested bool
	// Exposure is how far that address reaches. Empty when none was named.
	Exposure ProfilingExposure
	// Lifecycle is what became of the endpoint.
	Lifecycle ProfilingLifecycle
	// Revision fingerprints the state this observation describes.
	Revision string
}

// Sources for the process's own diagnostics. Each says what was read and, as
// often as not, what deliberately was not.
var (
	sourceLoggingSwitch = Source{
		Kind: SourceEnv,
		Ref:  "COZYPHI_DEBUG, read now: whether debug logging is asked for at this moment",
	}
	sourceLoggingLatched = Source{
		Kind: SourceComputed,
		Ref: "the answer the switch was read into on the first line anything tried to write; it is read " +
			"once and remembered, so changing the environment afterwards changes nothing",
	}
	sourceLoggingUnlatched = Source{
		Kind: SourceComputed,
		Ref:  "nothing has tried to write a line yet, so the switch has not been read and nothing is latched",
	}
	sourceLoggingActing = Source{
		Kind: SourceComputed,
		Ref: "whether a line written now would land: the latched answer if there is one, and otherwise " +
			"what the environment says, because writing it is what would latch it",
	}
	sourceLoggingDestinationEnv = Source{
		Kind: SourceEnv,
		Ref: "COZYPHI_DEBUG_FILE names the file. The name only — the directory it sits in is not reported, " +
			"and neither is anything in it",
	}
	sourceLoggingDestinationDefault = Source{
		Kind: SourceDefault,
		Ref:  "nothing names a file, so lines land on the default name in the working directory",
	}
	sourceLoggingSubsystemsEnv = Source{
		Kind: SourceEnv,
		Ref: "COZYPHI_MCP_LOG_DIR and COZYPHI_PLAN_GATE_LOG_DIR, read now: which subsystem logs the " +
			"environment redirects. Presence only — the directory each names is not reported, and " +
			"neither is anything in it",
	}
	sourceLoggingSubsystemsDefault = Source{
		Kind: SourceDefault,
		Ref: "nothing redirects a subsystem log, so the MCP server logs and the plan-gate log land " +
			"under their default names in the cozyphi home",
	}
	sourceLoggingSubsystemsActing = Source{
		Kind: SourceComputed,
		Ref: "the redirect each subsystem would honor: its directory is read from the environment " +
			"when the log is opened, so what the environment says now is what a log opened now obeys",
	}
	sourceLoggingOpen = Source{
		Kind: SourceComputed,
		Ref:  "the name of the file actually open. It is opened on the first line written, not at startup",
	}
	sourceLoggingNoFile = Source{
		Kind: SourceComputed,
		Ref: "no file is open: with the switch off nothing is ever written, and with it on the file is " +
			"opened by the first line that has something to say",
	}
	sourceTelemetryBuild = Source{
		Kind: SourceBuild,
		Ref: "this build carries plan telemetry; nothing in the configuration switches it on or off, and " +
			"the schema is fixed at build time",
	}
	sourceTelemetryTracker = Source{
		Kind: SourceSession,
		Ref:  "whether a tracker is in force for this session; a session without one records nothing",
	}
	sourceTelemetryWidth = Source{
		Kind: SourceComputed,
		Ref: "how many counters are actually counting: the whole surface when a tracker is in force and " +
			"none without one. Counters and bounded durations only — the schema is numeric by " +
			"construction, so no plan text, evidence or label can be recorded through it",
	}
	sourceTelemetryUnconfigured = Source{
		Kind: SourceComputed,
		Ref: "nothing configures a destination because there is none to configure: this build has no " +
			"metrics endpoint, no collector address and no telemetry file",
	}
	sourceTelemetryReadable = Source{
		Kind: SourceComputed,
		Ref:  "where the counters can be read inside this process",
	}
	sourceTelemetryUnreadable = Source{
		Kind: SourceComputed,
		Ref:  "nothing is counting, so there is nothing to read anywhere",
	}
	sourceTelemetryOffProcess = Source{
		Kind: SourceComputed,
		Ref: "whether anything leaves this process. Nothing does: the counters live in memory, are never " +
			"persisted and are never sent anywhere",
	}
	sourceProfilingSwitch = Source{
		Kind: SourceEnv,
		Ref: "COZYPHI_PPROF, read now: whether a profiling endpoint is asked for. The address it names " +
			"never travels — not the host, not the port, not the URL the startup line printed",
	}
	sourceProfilingExposure = Source{
		Kind: SourceComputed,
		Ref:  "how far the address reaches, derived from it without reporting it",
	}
	sourceProfilingNoEndpoint = Source{
		Kind: SourceComputed,
		Ref: "this process started no endpoint, so there is no address and nothing is listening. The " +
			"environment is read once, at start: exporting one into a running process asks for nothing",
	}
	sourceProfilingLifecycle = Source{
		Kind: SourceComputed,
		Ref: "what became of the endpoint, as this process recorded it at start. Nothing here connects " +
			"to it, fetches a profile or reads a handler — and an endpoint being up grants nothing: " +
			"the read-only harness view comes from --developer-mode and from nothing else",
	}
	sourceHeadlessNotRun = Source{
		Kind: SourceComputed,
		Ref: "this process runs a terminal surface; the headless ceilings are flags of cozyphi run " +
			"and bind nothing here",
	}
	sourceHeadlessOutputFlag = Source{
		Kind: SourceCLIFlag,
		Ref:  "--jsonl: the run prints one event per line",
	}
	sourceHeadlessOutputDefault = Source{
		Kind: SourceDefault,
		Ref:  "no --jsonl, so the run prints text",
	}
	sourceHeadlessRoundsFlag = Source{
		Kind: SourceCLIFlag,
		Ref:  "--max-rounds: the tool-round ceiling one turn may reach",
	}
	sourceHeadlessRoundsDefault = Source{
		Kind: SourceDefault,
		Ref: "no --max-rounds, so the engine's compiled-in round budget applies; it is not read back " +
			"through this view",
	}
	sourceHeadlessTimeoutFlag = Source{
		Kind: SourceCLIFlag,
		Ref:  "--timeout: the deadline the whole run is held to",
	}
	sourceHeadlessTimeoutDefault = Source{
		Kind: SourceDefault,
		Ref:  "no --timeout, so the run has no deadline of its own",
	}
	sourceHeadlessActing = Source{
		Kind: SourceSession,
		Ref: "what this run was started under, as the flags were parsed; nothing can change them " +
			"once the run is under way",
	}
	sourceHarnessLimitDefaults = Source{
		Kind: SourceBuild,
		Ref:  "the limits every caller starts from, compiled in rather than configured",
	}
	sourceHarnessLimitsAsked = Source{
		Kind: SourceSession,
		Ref:  "the limits this view was built with",
	}
	sourceHarnessLimitsActing = Source{
		Kind: SourceComputed,
		Ref: "the limits actually bounding an answer: an unset or nonsensical one falls back to its " +
			"default, and a per-category time budget wider than the whole answer's is narrowed to it",
	}
)

// loggingState separates the switch from the answer it was read into. They
// agree at startup and stop agreeing the moment somebody changes the
// environment of a running process, and that gap is the usual explanation
// for a log that is "on" and empty.
func (s LoggingFacts) loggingState() Field {
	field := diagField(KeyLoggingState, ScopeProcess, s.Revision)
	if !s.Known {
		return field
	}
	field.Configured = Present(BoolValue(s.Requested), sourceLoggingSwitch)
	if s.Latched {
		field.Loaded = Present(BoolValue(s.Enabled), sourceLoggingLatched)
	} else {
		field.Loaded = Unset(BoolValue(false), sourceLoggingUnlatched)
	}
	field.Effective = Present(BoolValue(s.acting()), sourceLoggingActing)
	return field
}

// loggingDestination is where lines land. The file is opened by the first
// line written rather than at startup, so a log that is on and has had
// nothing to say has a destination and no open file — which is not the same
// as a destination that could not be opened.
func (s LoggingFacts) loggingDestination() Field {
	field := diagField(KeyLoggingDestination, ScopeProcess, s.Revision)
	if !s.Known {
		return field
	}
	source := sourceLoggingDestinationDefault
	if s.FromEnv {
		source = sourceLoggingDestinationEnv
	}
	field.Configured = Present(StringValue(s.Destination), source)
	if s.OpenName == "" {
		field.Loaded = notApplicable(sourceLoggingNoFile)
		field.Effective = notApplicable(sourceLoggingNoFile)
		return field
	}
	field.Loaded = Present(StringValue(s.OpenName), sourceLoggingOpen)
	field.Effective = Present(StringValue(s.OpenName), sourceLoggingOpen)
	return field
}

// acting is whether a line written now would land: the latched answer if
// there is one, and otherwise what the environment says, because writing the
// line is what would latch it.
func (s LoggingFacts) acting() bool {
	if s.Latched {
		return s.Enabled
	}
	return s.Requested
}

// telemetryState is whether the plan is being counted. The build carrying
// the mechanism, this session holding a tracker and a record made now being
// counted are three different questions, and only the last one is the answer
// to "why are the counters zero".
func (s TelemetryFacts) telemetryState() Field {
	field := diagField(KeyTelemetryState, ScopeSession, s.Revision)
	if !s.Known {
		return field
	}
	tracked := TelemetryUntracked
	if s.Tracked {
		tracked = TelemetryTracked
	}
	field.Configured = Present(StringValue(string(TelemetrySupported)), sourceTelemetryBuild)
	field.Loaded = Present(StringValue(string(tracked)), sourceTelemetryTracker)
	counting := 0
	if s.Tracked {
		counting = s.Counters
	}
	field.Effective = Present(IntValue(int64(counting)), sourceTelemetryWidth)
	return field
}

// telemetryExport is where the counters go. The answer is nowhere, and it is
// worth a field of its own: telemetry is the part of a harness a reader is
// entitled to assume phones home, and here it does not.
func (s TelemetryFacts) telemetryExport() Field {
	field := diagField(KeyTelemetryExport, ScopeProcess, s.Revision)
	if !s.Known {
		return field
	}
	field.Configured = notApplicable(sourceTelemetryUnconfigured)
	if s.Tracked && len(s.Readable) > 0 {
		field.Loaded = Present(ListValue(s.Readable), sourceTelemetryReadable)
	} else {
		field.Loaded = notApplicable(sourceTelemetryUnreadable)
	}
	field.Effective = Present(BoolValue(false), sourceTelemetryOffProcess)
	return field
}

// profilingState is whether this process is serving profiles. It is one
// field rather than three because the three answers are one story: something
// asked for an endpoint, the address reaches that far, and the listener is
// or is not up.
func (s ProfilingFacts) profilingState() Field {
	field := diagField(KeyProfilingState, ScopeProcess, s.Revision)
	if !s.Known {
		return field
	}
	field.Configured = Present(BoolValue(s.Requested), sourceProfilingSwitch)
	if s.Exposure == "" {
		field.Loaded = notApplicable(sourceProfilingNoEndpoint)
	} else {
		field.Loaded = Present(StringValue(string(s.Exposure)), sourceProfilingExposure)
	}
	field.Effective = Present(StringValue(string(s.Lifecycle)), sourceProfilingLifecycle)
	return field
}

// loggingSubsystems is which subsystem logs the environment redirects: the
// MCP server logs and the plan-gate log each honor a directory of their
// own. Which of them is redirected is the fact a person debugging a missing
// log needs; where to is a path they chose, and it stays with them.
//
// The three layers agree by construction. Each directory is read from the
// environment when its log is opened, not at startup, so there is no latched
// answer to disagree with the switch.
func (s LoggingFacts) loggingSubsystems() Field {
	field := diagField(KeyLoggingSubsystems, ScopeProcess, s.Revision)
	if !s.Known {
		return field
	}
	redirected := s.redirectedSubsystems()
	if len(redirected) == 0 {
		field.Configured = Unset(ListValue(nil), sourceLoggingSubsystemsDefault)
	} else {
		field.Configured = Present(ListValue(redirected), sourceLoggingSubsystemsEnv)
	}
	field.Loaded = Present(ListValue(redirected), sourceLoggingSubsystemsActing)
	field.Effective = field.Loaded
	return field
}

// redirectedSubsystems names the subsystem logs the environment redirects,
// in one order so two answers about one state read the same way.
func (s LoggingFacts) redirectedSubsystems() []string {
	var out []string
	if s.MCPLogFromEnv {
		out = append(out, LogSubsystemMCP)
	}
	if s.PlanGateLogFromEnv {
		out = append(out, LogSubsystemPlanGate)
	}
	return out
}

// headlessOutput is how a headless run prints: one event per line, or text.
func (s HeadlessFacts) output() Field {
	field := s.headlessField(KeyHeadlessOutput)
	if !s.Known || !s.Run {
		return field
	}
	if s.JSONL {
		field.Configured = Present(StringValue("jsonl"), sourceHeadlessOutputFlag)
	} else {
		field.Configured = Present(StringValue("text"), sourceHeadlessOutputDefault)
	}
	field.Loaded = Present(field.Configured.Value, sourceHeadlessActing)
	field.Effective = field.Loaded
	return field
}

// rounds is the tool-round ceiling a headless run was started under. When
// none was asked for the engine's own budget applies, and this view does not
// read the engine to repeat it: the answer is that nothing was asked.
func (s HeadlessFacts) rounds() Field {
	field := s.headlessField(KeyHeadlessRounds)
	if !s.Known || !s.Run {
		return field
	}
	if s.MaxRounds > 0 {
		field.Configured = Present(IntValue(int64(s.MaxRounds)), sourceHeadlessRoundsFlag)
		field.Loaded = Present(field.Configured.Value, sourceHeadlessActing)
	} else {
		field.Configured = Unset(NoValue(), sourceHeadlessRoundsDefault)
		field.Loaded = Unset(NoValue(), sourceHeadlessRoundsDefault)
	}
	field.Effective = field.Loaded
	return field
}

// timeout is the deadline a headless run was started under, if any.
func (s HeadlessFacts) timeout() Field {
	field := s.headlessField(KeyHeadlessTimeout)
	if !s.Known || !s.Run {
		return field
	}
	if s.Timeout > 0 {
		field.Configured = Present(DurationValue(s.Timeout), sourceHeadlessTimeoutFlag)
		field.Loaded = Present(field.Configured.Value, sourceHeadlessActing)
	} else {
		field.Configured = Unset(NoValue(), sourceHeadlessTimeoutDefault)
		field.Loaded = Unset(NoValue(), sourceHeadlessTimeoutDefault)
	}
	field.Effective = field.Loaded
	return field
}

// headlessField is the shape the three ceilings start from: unavailable
// where nobody wired them, and not applicable — on every layer, because a
// terminal session has no configured half either — where a surface is
// running instead of a headless run.
func (s HeadlessFacts) headlessField(key string) Field {
	field := diagField(key, ScopeProcess, s.Revision)
	if s.Known && !s.Run {
		field.Configured = notApplicable(sourceHeadlessNotRun)
		field.Loaded = notApplicable(sourceHeadlessNotRun)
		field.Effective = notApplicable(sourceHeadlessNotRun)
	}
	return field
}

// HeadlessRevision fingerprints what a headless observation describes. The
// flags are fixed when the run starts, so it is only a way to tell one run's
// answer from another's.
func HeadlessRevision(facts HeadlessFacts) string {
	if !facts.Run {
		return "surface"
	}
	out := "j"
	if facts.JSONL {
		out += "1"
	} else {
		out += "0"
	}
	return out + ".r" + strconv.Itoa(facts.MaxRounds) + ".t" + facts.Timeout.String()
}

// harnessLimits is what bounds an answer from this view. It is reported by
// the view about itself on purpose: a truncated snapshot says it was
// truncated, and this is where a reader finds out at what.
func harnessLimits(asked Limits) Field {
	acting := asked.normalized()
	field := diagField(KeyHarnessLimits, ScopeSession, limitRevision(acting))
	field.Configured = Present(ListValue(limitLabels(DefaultLimits())), sourceHarnessLimitDefaults)
	field.Loaded = Present(ListValue(limitLabels(asked)), sourceHarnessLimitsAsked)
	field.Effective = Present(ListValue(limitLabels(acting)), sourceHarnessLimitsActing)
	return field
}

// limitRevision fingerprints the limits in force, so an answer taken under
// one set is visibly not an answer taken under another.
func limitRevision(l Limits) string {
	return "b" + strconv.Itoa(l.MaxTotalBytes) + ".f" + strconv.Itoa(l.MaxFieldsPerCategory) +
		".t" + l.MaxTotalDuration.String()
}

// limitLabels renders one set of limits as "name=value", in one order so two
// answers about one state read the same way.
func limitLabels(l Limits) []string {
	return []string{
		"fields_per_category=" + strconv.Itoa(l.MaxFieldsPerCategory),
		"value_bytes=" + strconv.Itoa(l.MaxValueBytes),
		"list_items=" + strconv.Itoa(l.MaxListItems),
		"total_bytes=" + strconv.Itoa(l.MaxTotalBytes),
		"category_time=" + l.MaxCategoryDuration.String(),
		"answer_time=" + l.MaxTotalDuration.String(),
	}
}

// diagField is the shape every process field starts from: all three layers
// unavailable, so a layer nobody wired degrades into an honest answer rather
// than into a zero that would read as a switch somebody turned off.
//
// Every one of them takes a restart to change. The switches here are read
// from the environment once, at start, and the limits are fixed when the
// view is built — so nothing in this category is a setting a running session
// can be talked into changing.
func diagField(key string, scope Scope, revision string) Field {
	return Field{
		Key:        key,
		Configured: Unavailable(),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      ApplyRestart,
		Scope:      scope,
		Revision:   revision,
	}
}
