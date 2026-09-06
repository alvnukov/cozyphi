package diag

import "context"

// Context field keys. They are declared statically so the catalog can list
// them and Explain can validate a key without observing anything.
//
// The names are grouped by what they answer rather than by where they come
// from: what the window is, what is in it, what compaction will do about it,
// and what the system prompt was assembled from.
const (
	// KeyContextWindow is the token window this session budgets against.
	KeyContextWindow = "window"
	// KeyContextCeiling is the spawn-time cap a parent put on that window.
	KeyContextCeiling = "window.ceiling"
	// KeyContextOverride is the session-only narrowing the user asked for.
	KeyContextOverride = "window.override"

	// KeyContextTokens is the best known token count occupying the window.
	KeyContextTokens = "usage.tokens"
	// KeyContextTokenSource says how that count was arrived at.
	KeyContextTokenSource = "usage.token_source"
	// KeyContextBytes is the serialized size of the model view.
	KeyContextBytes = "usage.bytes"
	// KeyContextMessages is how many messages that view holds.
	KeyContextMessages = "usage.messages"
	// KeyContextMicroResults is how many old tool results are stubbed.
	KeyContextMicroResults = "usage.micro_elided_results"
	// KeyContextMicroBytes is what those stubs removed.
	KeyContextMicroBytes = "usage.micro_elided_bytes"

	// KeyContextCompactionEnabled is whether compaction runs at all.
	KeyContextCompactionEnabled = "compaction.enabled"
	// KeyContextCompactThreshold is where the compact advice starts.
	KeyContextCompactThreshold = "compaction.threshold"
	// KeyContextCompactHeadroom is the room kept below the window.
	KeyContextCompactHeadroom = "compaction.headroom"
	// KeyContextKeepRecent is the verbatim tail budget.
	KeyContextKeepRecent = "compaction.keep_recent"
	// KeyContextCompactRecommended mirrors the advice decision.
	KeyContextCompactRecommended = "compaction.recommended"
	// KeyContextCompactPending is a compaction already asked for.
	KeyContextCompactPending = "compaction.pending"
	// KeyContextCompactStrikes counts rounds run over the threshold.
	KeyContextCompactStrikes = "compaction.strikes"
	// KeyContextCompactStopped is the latched refusal to continue.
	KeyContextCompactStopped = "compaction.stopped"
	// KeyContextCompactCount is how many compactions this context carries.
	KeyContextCompactCount = "compaction.count"
	// KeyContextLastCompactionKind tells a summary from a user trim.
	KeyContextLastCompactionKind = "compaction.last.kind"
	// KeyContextLastTokensBefore and KeyContextLastTokensAfter are what the
	// latest compaction measured on either side of itself.
	KeyContextLastTokensBefore = "compaction.last.tokens_before"
	KeyContextLastTokensAfter  = "compaction.last.tokens_after"
	// KeyContextLastSummarized and KeyContextLastKept are what it did with
	// the history.
	KeyContextLastSummarized = "compaction.last.messages_summarized"
	KeyContextLastKept       = "compaction.last.messages_kept"

	// KeyContextPromptBytes is the size of the assembled system prompt.
	KeyContextPromptBytes = "sources.prompt_bytes"
	// KeyContextInstructions is how many project-instruction files it carries.
	KeyContextInstructions = "sources.instructions"
	// KeyContextInstructionScopes says where each of them was found.
	KeyContextInstructionScopes = "sources.instructions.scopes"
	// KeyContextSkills is how many skills the catalog block names.
	KeyContextSkills = "sources.skills"
	// KeyContextMemory is how many memories the store held for that render.
	KeyContextMemory = "sources.memory"
	// KeyContextMemoryStanding is how many of them ride every request.
	KeyContextMemoryStanding = "sources.memory.standing"
	// KeyContextMemoryRunes is what the memory block costs the prompt.
	KeyContextMemoryRunes = "sources.memory.runes"
)

// Token source vocabulary. It is the whole answer to "is this a measurement",
// and it is three values rather than a boolean because the usual case is
// neither: a provider count taken one request ago, moved by an estimate of
// everything appended since.
const (
	// TokenSourceProvider is a count the provider reported for exactly this
	// context. It is a measurement.
	TokenSourceProvider = "provider"
	// TokenSourceCalibrated is that count plus the estimated change since it
	// was taken. Part measured, part estimated.
	TokenSourceCalibrated = "calibrated"
	// TokenSourceEstimate is the serialized-bytes heuristic alone — one token
	// per four bytes, used before any provider count has been seen. It is not
	// a measurement and must never be read as one.
	TokenSourceEstimate = "estimate"
)

// Compaction kinds. A trim is the user cutting history by hand; its counters
// read differently from a summary's, because nothing was summarized.
const (
	CompactionSummary = "summary"
	CompactionTrim    = "trim"
)

// contextKeys is the declared key set, in the order Collect returns them.
var contextKeys = []string{
	KeyContextWindow,
	KeyContextCeiling,
	KeyContextOverride,
	KeyContextTokens,
	KeyContextTokenSource,
	KeyContextBytes,
	KeyContextMessages,
	KeyContextMicroResults,
	KeyContextMicroBytes,
	KeyContextCompactionEnabled,
	KeyContextCompactThreshold,
	KeyContextCompactHeadroom,
	KeyContextKeepRecent,
	KeyContextCompactRecommended,
	KeyContextCompactPending,
	KeyContextCompactStrikes,
	KeyContextCompactStopped,
	KeyContextCompactCount,
	KeyContextLastCompactionKind,
	KeyContextLastTokensBefore,
	KeyContextLastTokensAfter,
	KeyContextLastSummarized,
	KeyContextLastKept,
	KeyContextPromptBytes,
	KeyContextInstructions,
	KeyContextInstructionScopes,
	KeyContextSkills,
	KeyContextMemory,
	KeyContextMemoryStanding,
	KeyContextMemoryRunes,
}

// contextReason states what this category answers and what it deliberately
// leaves out.
const contextReason = "the window this session budgets against, what occupies it now, what " +
	"compaction will do about it, and what the system prompt was assembled from — numbers, " +
	"counts and provenance only. No message, no summary, no prompt text, no instruction file, " +
	"no skill body and no memory is readable here, and the token count says whether it was " +
	"measured or estimated instead of presenting a heuristic as a measurement. Nothing here " +
	"compacts, trims, loads a memory, re-reads an instruction file or a skill, or moves the " +
	"token calibration: the source metadata comes from the load that already happened"

// ContextWindow is the window arithmetic: what the model offers and what each
// narrowing left of it. The three inputs are kept apart because a window
// smaller than the model's is a question with three different answers — the
// model is small, a parent capped this engine, or the user asked for less.
type ContextWindow struct {
	// Model is the window the model entry declares; 0 = unknown.
	Model int
	// Ceiling is the spawn-time cap a parent put on this engine; 0 = none.
	Ceiling int
	// Override is the session-only narrowing the user asked for; 0 = none.
	Override int
	// Effective is what the engine budgets against after both narrowings.
	Effective int
}

// ContextUsage is what occupies the window right now. Every member is a
// number about the model view; none of them is any part of it.
type ContextUsage struct {
	// Tokens is the best known count occupying the window.
	Tokens int
	// TokenSource is how Tokens was arrived at: provider, calibrated or
	// estimate. Empty means the owner did not say, and the field reports
	// that rather than picking the flattering one.
	TokenSource string
	// Bytes is the serialized size of the model view, and Messages how many
	// messages it holds.
	Bytes    int
	Messages int
	// MicroElidedResults is how many old tool results the provider view
	// currently stubs, and MicroElidedBytes what those stubs removed. They
	// are the difference between what the session holds and what the next
	// request carries.
	MicroElidedResults int
	MicroElidedBytes   int
}

// ContextCompactionLast is the outcome of the latest compaction, in counters.
type ContextCompactionLast struct {
	// Known is whether the members below describe a real compaction. Zero
	// from a compaction that measured nothing and zero because none ever ran
	// are different answers.
	Known bool
	// FromTrim is true when the latest one was the user cutting history by
	// hand rather than a generated summary.
	FromTrim bool
	// TokensBefore and TokensAfter are what it measured on either side.
	TokensBefore int
	TokensAfter  int
	// MessagesSummarized and MessagesKept are what it did with the history.
	MessagesSummarized int
	MessagesKept       int
}

// ContextCompaction is the compaction policy this session runs under and the
// pressure it is under now. The configured budgets and the numbers they work
// out to for this window are separate members on purpose: a threshold of 0
// because compaction is off and one of 0 because the window is unknown are
// not the same situation.
type ContextCompaction struct {
	// Enabled is whether compaction runs at all.
	Enabled bool
	// ReminderTokens is the advice threshold the settings carry; 0 when none
	// was set and the window-derived one is used instead.
	ReminderTokens int
	// Threshold is where the advice actually starts for this window; 0 when
	// compaction is off or the window is unknown.
	Threshold int
	// Headroom is the room kept below the window: compaction fires at the
	// window minus this.
	Headroom int
	// KeepRecent is the tail budget neither the summary cut nor the
	// provider-view projection touches.
	KeepRecent int
	// Recommended mirrors the advice decision for the usage above.
	Recommended bool
	// Pending is a compaction the model already asked for, waiting for the
	// end of the current tool round.
	Pending bool
	// Strikes counts tool rounds that ran over the threshold without a
	// compaction landing, and Stopped is the latched refusal to continue
	// until one does.
	Strikes int
	Stopped bool
	// Count is how many compactions the current context path carries.
	Count int
	// Last is the latest one's counters.
	Last ContextCompactionLast
}

// ContextSources is what the system prompt was assembled from — how many of
// each kind of input, from which scope, at what size. It is deliberately not
// a list of files: a count and a scope say what shape the prompt has, where a
// path would say what the user's machine looks like and a name would start
// describing the contents.
//
// Every member comes from the render that already happened. Nothing here is
// re-read to answer a question about it, which is why Loaded exists: before
// the first render there is no record, and saying so beats reporting zeroes.
type ContextSources struct {
	// Loaded is whether a prompt has been assembled yet.
	Loaded bool
	// PromptBytes is the size of the finished system prompt.
	PromptBytes int
	// Instructions is how many project-instruction files it carries, and
	// InstructionScopes where each was found, in load order.
	Instructions      int
	InstructionScopes []string
	// SkillDir is whether a skill directory was configured for that render;
	// without one no catalog was read, which is a different answer from a
	// directory that held nothing.
	SkillDir bool
	// Skills is how many skills the catalog block names.
	Skills int
	// MemoryStore is whether a memory store is attached at all. The three
	// counts below describe the block the same render built: how many
	// memories were stored, how many ride every request in full, and what
	// the block costs the prompt.
	MemoryStore    bool
	MemoryFacts    int
	MemoryStanding int
	MemoryRunes    int
}

// ContextState is one observation of the context layer, read from the owner
// in a single pass so the window, the usage and the compaction policy in one
// snapshot describe one moment rather than three.
type ContextState struct {
	// Known is false when no engine has published a context yet. Every layer
	// fed from it then reports unavailable rather than zeroes that would read
	// as an empty context.
	Known      bool
	Window     ContextWindow
	Usage      ContextUsage
	Compaction ContextCompaction
	Sources    ContextSources
	// Revision fingerprints the generation this observation belongs to: the
	// prompt render, the window and the compactions behind it. Usage drifts
	// between two observations of one generation; the revision does not.
	Revision string
}

// Sources for the layers of this category. They are written out rather than
// composed at the call site so every field's provenance is one named thing a
// reader can compare against another field's.
var (
	sourceContextModelWindow = Source{
		Kind: SourceConfigFile,
		Ref:  "the context window the model entry declares",
	}
	sourceContextNoModelWindow = Source{
		Kind: SourceUnknown,
		Ref:  "the model entry declares no context window, so there is no budget to measure against",
	}
	sourceContextEffectiveWindow = Source{
		Kind: SourceComputed,
		Ref:  "the model's window after the spawn ceiling and the session override, each only narrowing",
	}
	sourceContextCeiling = Source{
		Kind: SourceSession,
		Ref:  "the cap the parent set when it spawned this engine",
	}
	sourceContextNoCeiling = Source{
		Kind: SourceSession,
		Ref:  "no parent capped this engine's window",
	}
	sourceContextOverride = Source{
		Kind: SourceSession,
		Ref:  "the window narrowing asked for in this session; it is never persisted",
	}
	sourceContextNoOverride = Source{
		Kind: SourceSession,
		Ref:  "no session override; the model's own window stands",
	}
	sourceContextProviderTokens = Source{
		Kind: SourceComputed,
		Ref:  "counted by the provider for exactly this context — a measurement",
	}
	sourceContextCalibratedTokens = Source{
		Kind: SourceComputed,
		Ref: "the provider's count for the last request plus the estimated change since it was " +
			"taken: part measured, part estimated",
	}
	sourceContextEstimatedTokens = Source{
		Kind: SourceComputed,
		Ref: "a serialized-bytes heuristic, one token per four bytes — no provider count has been " +
			"seen yet, so this is not a measurement",
	}
	sourceContextUnknownTokens = Source{
		Kind: SourceUnknown,
		Ref:  "the owner did not say how this count was arrived at, so it must not be read as measured",
	}
	sourceContextModelView = Source{
		Kind: SourceComputed,
		Ref:  "the model view the next request would carry, serialized",
	}
	sourceContextMicro = Source{
		Kind: SourceComputed,
		Ref: "provider-view microcompaction: old tool results stubbed so the cached prompt prefix " +
			"survives, the session's own record untouched",
	}
	sourceContextSettings = Source{
		Kind: SourceDefault,
		Ref:  "the compaction settings this session runs under",
	}
	sourceContextReminderSet = Source{
		Kind: SourceConfigFile,
		Ref:  "the reminder threshold set for this session",
	}
	sourceContextReminderUnset = Source{
		Kind: SourceDefault,
		Ref:  "no reminder threshold was set; the window-derived one is used",
	}
	sourceContextThreshold = Source{
		Kind: SourceComputed,
		Ref:  "where the compact advice starts for this window",
	}
	sourceContextNoThreshold = Source{
		Kind: SourceComputed,
		Ref:  "compaction is off or the window is unknown, so no threshold acts",
	}
	sourceContextPressure = Source{
		Kind: SourceComputed,
		Ref:  "the pressure ladder, recomputed for this observation",
	}
	sourceContextPending = Source{
		Kind: SourceSession,
		Ref: "a compaction the model asked for, waiting for the end of the current tool round; " +
			"reading it here neither schedules nor cancels one",
	}
	sourceContextCompactions = Source{
		Kind: SourceSession,
		Ref:  "the compactions the current context path carries",
	}
	sourceContextLastCompaction = Source{
		Kind: SourceSession,
		Ref:  "recorded by the compaction itself when it landed",
	}
	sourceContextNoCompaction = Source{
		Kind: SourceSession,
		Ref:  "this context carries no compaction yet",
	}
	sourceContextPrompt = Source{
		Kind: SourceComputed,
		Ref:  "measured where the prompt was assembled, not by assembling it again",
	}
	sourceContextInstructions = Source{
		Kind: SourceConfigFile,
		Ref: "the instruction files discovery found in the agent directory and in the ancestors " +
			"of the working directory",
	}
	sourceContextInstructionScopes = Source{
		Kind: SourceComputed,
		Ref:  "where each loaded file was found, in load order; the paths stay with the owner",
	}
	sourceContextSkills = Source{
		Kind: SourceConfigFile,
		Ref:  "skills parsed from the configured skill directory when the prompt was built",
	}
	sourceContextNoSkillDir = Source{
		Kind: SourceConfigFile,
		Ref:  "no skill directory is configured for this session, so no catalog was read",
	}
	sourceContextMemory = Source{
		Kind: SourceConfigFile,
		Ref:  "the memories the store held when the prompt was built",
	}
	sourceContextNoMemory = Source{
		Kind: SourceSession,
		Ref:  "no memory store is attached to this session",
	}
	sourceContextInPrompt = Source{
		Kind: SourceComputed,
		Ref:  "carried by the system prompt the next request sends",
	}
)

// ContextDeps binds the context collector to its one owner. The engine holds
// the window, the usage projection, the compaction policy and the record of
// the last prompt render together, and it is asked for all of them at once so
// the fields of one snapshot describe one moment.
type ContextDeps struct {
	// State observes the engine's context layer through the owner's own
	// projection. It is read, never exercised: no accessor here may compact,
	// trim, summarize, load a memory, search one, re-read an instruction file
	// or a skill directory, or move the token calibration.
	State func() ContextState
}

// contextCollector observes the window, what is in it, and what it was built
// from.
type contextCollector struct {
	deps ContextDeps
}

// NewContextCollector builds the context collector.
func NewContextCollector(deps ContextDeps) Collector {
	return &contextCollector{deps: deps}
}

func (*contextCollector) Category() Category { return CategoryContext }

// Status is answered from the declared key set alone: listing what can be
// observed builds no context projection and reads nothing from disk.
func (*contextCollector) Status() Status {
	keys := make([]string, len(contextKeys))
	copy(keys, contextKeys)
	return Status{Availability: AvailabilityAvailable, Reason: contextReason, Keys: keys}
}

// Collect reads the owner once and derives every field from that one read.
func (c *contextCollector) Collect(_ context.Context) ([]Field, error) {
	state := callContextState(c.deps.State)
	return []Field{
		state.window(),
		state.ceiling(),
		state.override(),
		state.tokens(),
		state.tokenSource(),
		state.bytes(),
		state.messages(),
		state.microResults(),
		state.microBytes(),
		state.compactionEnabled(),
		state.compactThreshold(),
		state.compactHeadroom(),
		state.keepRecent(),
		state.compactRecommended(),
		state.compactPending(),
		state.compactStrikes(),
		state.compactStopped(),
		state.compactCount(),
		state.lastCompactionKind(),
		state.lastTokensBefore(),
		state.lastTokensAfter(),
		state.lastSummarized(),
		state.lastKept(),
		state.promptBytes(),
		state.instructions(),
		state.instructionScopes(),
		state.skills(),
		state.memoryFacts(),
		state.memoryStanding(),
		state.memoryRunes(),
	}, nil
}

// window is the one field with a real configured-to-effective story: the
// model entry declares a window, and this session budgets against whatever
// the narrowings left of it. The two narrowings are their own fields, so the
// difference between the layers is always attributable.
func (s ContextState) window() Field {
	field := s.field(KeyContextWindow, ApplyNewSession, ScopeSession)
	if !s.Known {
		return field
	}
	field.Configured = Unset(IntValue(0), sourceContextNoModelWindow)
	if s.Window.Model > 0 {
		field.Configured = Present(IntValue(int64(s.Window.Model)), sourceContextModelWindow)
	}
	field.Effective = Unset(IntValue(0), sourceContextNoModelWindow)
	if s.Window.Effective > 0 {
		field.Effective = Present(IntValue(int64(s.Window.Effective)), sourceContextEffectiveWindow)
	}
	return field
}

// ceiling is a limit a parent set, so it cannot change without a new session.
func (s ContextState) ceiling() Field {
	return s.limit(KeyContextCeiling, s.Window.Ceiling,
		sourceContextCeiling, sourceContextNoCeiling, ApplyNewSession)
}

// override is a limit this session set, and setting it acts at once.
func (s ContextState) override() Field {
	return s.limit(KeyContextOverride, s.Window.Override,
		sourceContextOverride, sourceContextNoOverride, ApplyImmediate)
}

// limit builds one configured narrowing. Zero is not missing data — it is the
// answer "nothing narrows the window here" — so it is reported as unset with
// the reason attached rather than as an absent field.
func (s ContextState) limit(key string, value int, set, unset Source, apply Apply) Field {
	field := s.field(key, apply, ScopeSession)
	if !s.Known {
		return field
	}
	observation := Unset(IntValue(0), unset)
	if value > 0 {
		observation = Present(IntValue(int64(value)), set)
	}
	field.Configured = observation
	field.Effective = observation
	return field
}

// tokens is the number every budget decision is made against, and its source
// says what kind of number it is. Nothing configures usage, so the configured
// layer does not exist rather than reporting a limit that is not one.
func (s ContextState) tokens() Field {
	field := s.field(KeyContextTokens, ApplyImmediate, ScopeTurn)
	if !s.Known {
		return field
	}
	field.Effective = Present(IntValue(int64(s.Usage.Tokens)), tokenSourceRef(s.Usage.TokenSource))
	return field
}

// tokenSource is the honesty field: it names whether the count above was
// measured, part-measured or estimated. An owner that did not say leaves it
// unset with an explicit unknown origin, because an unnamed estimate read as
// a measurement is the failure this whole category exists to prevent.
func (s ContextState) tokenSource() Field {
	field := s.field(KeyContextTokenSource, ApplyImmediate, ScopeTurn)
	if !s.Known {
		return field
	}
	field.Effective = Unset(StringValue(""), sourceContextUnknownTokens)
	if s.Usage.TokenSource != "" {
		field.Effective = Present(StringValue(s.Usage.TokenSource), tokenSourceRef(s.Usage.TokenSource))
	}
	return field
}

func (s ContextState) bytes() Field {
	return s.measure(KeyContextBytes, s.Usage.Bytes, sourceContextModelView)
}

func (s ContextState) messages() Field {
	return s.measure(KeyContextMessages, s.Usage.Messages, sourceContextModelView)
}

func (s ContextState) microResults() Field {
	return s.measure(KeyContextMicroResults, s.Usage.MicroElidedResults, sourceContextMicro)
}

func (s ContextState) microBytes() Field {
	return s.measure(KeyContextMicroBytes, s.Usage.MicroElidedBytes, sourceContextMicro)
}

// measure builds one observed number about the current context. Nothing
// configures or loads it: it is counted from what is there, so only the
// effective layer exists.
func (s ContextState) measure(key string, value int, source Source) Field {
	field := s.field(key, ApplyImmediate, ScopeTurn)
	if !s.Known {
		return field
	}
	field.Effective = Present(IntValue(int64(value)), source)
	return field
}

func (s ContextState) compactionEnabled() Field {
	field := s.field(KeyContextCompactionEnabled, ApplyNextTurn, ScopeSession)
	if !s.Known {
		return field
	}
	observation := Present(BoolValue(s.Compaction.Enabled), sourceContextSettings)
	field.Configured = observation
	field.Effective = observation
	return field
}

// compactThreshold is the field the ticket's separation is sharpest on: the
// configured layer is a number a user chose or the absence of one, and the
// effective layer is what that works out to against this window. They differ
// whenever nobody set one, which is the usual case.
func (s ContextState) compactThreshold() Field {
	field := s.field(KeyContextCompactThreshold, ApplyNextTurn, ScopeSession)
	if !s.Known {
		return field
	}
	field.Configured = Unset(IntValue(0), sourceContextReminderUnset)
	if s.Compaction.ReminderTokens > 0 {
		field.Configured = Present(IntValue(int64(s.Compaction.ReminderTokens)), sourceContextReminderSet)
	}
	field.Effective = Unset(IntValue(0), sourceContextNoThreshold)
	if s.Compaction.Threshold > 0 {
		field.Effective = Present(IntValue(int64(s.Compaction.Threshold)), sourceContextThreshold)
	}
	return field
}

func (s ContextState) compactHeadroom() Field {
	return s.budget(KeyContextCompactHeadroom, s.Compaction.Headroom)
}

func (s ContextState) keepRecent() Field {
	return s.budget(KeyContextKeepRecent, s.Compaction.KeepRecent)
}

// budget builds one compaction budget: a configured token count that acts as
// it is, with no window arithmetic between the layers.
func (s ContextState) budget(key string, value int) Field {
	field := s.field(key, ApplyNextTurn, ScopeSession)
	if !s.Known {
		return field
	}
	observation := Present(IntValue(int64(value)), sourceContextSettings)
	field.Configured = observation
	field.Effective = observation
	return field
}

func (s ContextState) compactRecommended() Field {
	field := s.field(KeyContextCompactRecommended, ApplyImmediate, ScopeTurn)
	if !s.Known {
		return field
	}
	field.Effective = Present(BoolValue(s.Compaction.Recommended), sourceContextPressure)
	return field
}

func (s ContextState) compactPending() Field {
	field := s.field(KeyContextCompactPending, ApplyNextTurn, ScopeTurn)
	if !s.Known {
		return field
	}
	field.Effective = Present(BoolValue(s.Compaction.Pending), sourceContextPending)
	return field
}

func (s ContextState) compactStrikes() Field {
	field := s.field(KeyContextCompactStrikes, ApplyImmediate, ScopeTurn)
	if !s.Known {
		return field
	}
	field.Effective = Present(IntValue(int64(s.Compaction.Strikes)), sourceContextPressure)
	return field
}

func (s ContextState) compactStopped() Field {
	field := s.field(KeyContextCompactStopped, ApplyImmediate, ScopeSession)
	if !s.Known {
		return field
	}
	field.Effective = Present(BoolValue(s.Compaction.Stopped), sourceContextPressure)
	return field
}

func (s ContextState) compactCount() Field {
	field := s.field(KeyContextCompactCount, ApplyImmediate, ScopeSession)
	if !s.Known {
		return field
	}
	field.Effective = Present(IntValue(int64(s.Compaction.Count)), sourceContextCompactions)
	return field
}

// lastCompactionKind tells a generated summary from a trim the user asked
// for. The counters below mean different things in the two cases, so the
// kind is reported next to them rather than left to be inferred.
func (s ContextState) lastCompactionKind() Field {
	field := s.field(KeyContextLastCompactionKind, ApplyImmediate, ScopeSession)
	if !s.Known {
		return field
	}
	field.Effective = Unset(StringValue(""), sourceContextNoCompaction)
	if s.Compaction.Last.Known {
		kind := CompactionSummary
		if s.Compaction.Last.FromTrim {
			kind = CompactionTrim
		}
		field.Effective = Present(StringValue(kind), sourceContextLastCompaction)
	}
	return field
}

func (s ContextState) lastTokensBefore() Field {
	return s.lastCount(KeyContextLastTokensBefore, s.Compaction.Last.TokensBefore)
}

func (s ContextState) lastTokensAfter() Field {
	return s.lastCount(KeyContextLastTokensAfter, s.Compaction.Last.TokensAfter)
}

func (s ContextState) lastSummarized() Field {
	return s.lastCount(KeyContextLastSummarized, s.Compaction.Last.MessagesSummarized)
}

func (s ContextState) lastKept() Field {
	return s.lastCount(KeyContextLastKept, s.Compaction.Last.MessagesKept)
}

// lastCount builds one counter of the latest compaction. Without a compaction
// the layer is unset with the reason, so a zero that means "none ever ran"
// cannot be read as one that means "it freed nothing".
func (s ContextState) lastCount(key string, value int) Field {
	field := s.field(key, ApplyImmediate, ScopeSession)
	if !s.Known {
		return field
	}
	field.Effective = Unset(IntValue(0), sourceContextNoCompaction)
	if s.Compaction.Last.Known {
		field.Effective = Present(IntValue(int64(value)), sourceContextLastCompaction)
	}
	return field
}

// promptBytes is what the assembled system prompt costs. It is measured at
// assembly and reported from the record: building it again to measure it
// would re-read every instruction file and the whole skill catalog, which is
// exactly what observing must not do.
func (s ContextState) promptBytes() Field {
	field := s.field(KeyContextPromptBytes, ApplyNextTurn, ScopeSession)
	if !s.Known || !s.Sources.Loaded {
		return field
	}
	field.Effective = Present(IntValue(int64(s.Sources.PromptBytes)), sourceContextPrompt)
	return field
}

// instructions separates the two questions the layers exist for: how many
// files discovery took in, and how many of them the prompt the next request
// sends actually carries.
func (s ContextState) instructions() Field {
	field := s.field(KeyContextInstructions, ApplyNextTurn, ScopeWorkspace)
	if !s.Known || !s.Sources.Loaded {
		return field
	}
	count := int64(s.Sources.Instructions)
	field.Loaded = Present(IntValue(count), sourceContextInstructions)
	field.Effective = Present(IntValue(count), sourceContextInPrompt)
	return field
}

// instructionScopes says how far each loaded file's authority reaches — the
// global agent directory, an ancestor of the working directory, or the
// working directory itself. The scopes are the safe half of the file list;
// the paths never leave the owner.
func (s ContextState) instructionScopes() Field {
	field := s.field(KeyContextInstructionScopes, ApplyNextTurn, ScopeWorkspace)
	if !s.Known || !s.Sources.Loaded {
		return field
	}
	field.Effective = Present(ListValue(s.Sources.InstructionScopes), sourceContextInstructionScopes)
	return field
}

// skills counts what the catalog block names. A session with no skill
// directory reports unset with that reason rather than 0, because "none
// configured" and "the directory held none" are different answers.
func (s ContextState) skills() Field {
	field := s.field(KeyContextSkills, ApplyNextTurn, ScopeWorkspace)
	if !s.Known || !s.Sources.Loaded {
		return field
	}
	if !s.Sources.SkillDir {
		field.Loaded = Unset(IntValue(0), sourceContextNoSkillDir)
		field.Effective = Unset(IntValue(0), sourceContextNoSkillDir)
		return field
	}
	count := int64(s.Sources.Skills)
	field.Loaded = Present(IntValue(count), sourceContextSkills)
	field.Effective = Present(IntValue(count), sourceContextInPrompt)
	return field
}

func (s ContextState) memoryFacts() Field {
	return s.memoryCount(KeyContextMemory, s.Sources.MemoryFacts, sourceContextMemory)
}

func (s ContextState) memoryStanding() Field {
	return s.memoryCount(KeyContextMemoryStanding, s.Sources.MemoryStanding, sourceContextInPrompt)
}

func (s ContextState) memoryRunes() Field {
	return s.memoryCount(KeyContextMemoryRunes, s.Sources.MemoryRunes, sourceContextPrompt)
}

// memoryCount builds one measurement of what memory contributes. Every one of
// them comes from the block the last render built: asking the store again
// would re-scan its directory, and an observation is not a reason to load
// anything.
func (s ContextState) memoryCount(key string, value int, source Source) Field {
	field := s.field(key, ApplyNextTurn, ScopeWorkspace)
	if !s.Known || !s.Sources.Loaded {
		return field
	}
	if !s.Sources.MemoryStore {
		field.Effective = Unset(IntValue(0), sourceContextNoMemory)
		return field
	}
	field.Effective = Present(IntValue(int64(value)), source)
	return field
}

// field is the shape every context field starts from: all three layers
// unavailable, so a state nobody has published yet reports honestly and each
// builder only fills in the layers it can answer for.
func (s ContextState) field(key string, apply Apply, scope Scope) Field {
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

// tokenSourceRef attaches the provenance the token source names. An owner
// that named nothing gets the explicit unknown, never the benefit of the
// doubt.
func tokenSourceRef(source string) Source {
	switch source {
	case TokenSourceProvider:
		return sourceContextProviderTokens
	case TokenSourceCalibrated:
		return sourceContextCalibratedTokens
	case TokenSourceEstimate:
		return sourceContextEstimatedTokens
	default:
		return sourceContextUnknownTokens
	}
}

// callContextState reads the optional accessor. A nil accessor is a wiring
// gap, and every layer it feeds reports unavailable rather than crashing the
// snapshot.
func callContextState(accessor func() ContextState) ContextState {
	if accessor == nil {
		return ContextState{}
	}
	return accessor()
}
