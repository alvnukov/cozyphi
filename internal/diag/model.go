package diag

import "context"

// Model field keys. They are declared statically so the catalog can list
// them and Explain can validate a key without observing anything.
const (
	KeyModelName            = "name"
	KeyModelRequestName     = "request_name"
	KeyModelProtocol        = "protocol"
	KeyModelEffort          = "effort"
	KeyModelRequestEffort   = "effort.request"
	KeyModelEffortLevels    = "effort.levels"
	KeyModelContextWindow   = "context_window"
	KeyModelMaxOutputTokens = "max_output_tokens"
	KeyModelVariants        = "variants"
	KeyModelOptions         = "options"
	KeyModelThinking        = "thinking"
	KeyModelPlanPinned      = "pinned_by_plan"
	KeyModelSourceOrder     = "source_order"
)

// modelKeys is the declared key set, in the order Collect returns them.
var modelKeys = []string{
	KeyModelName,
	KeyModelRequestName,
	KeyModelProtocol,
	KeyModelEffort,
	KeyModelRequestEffort,
	KeyModelEffortLevels,
	KeyModelContextWindow,
	KeyModelMaxOutputTokens,
	KeyModelVariants,
	KeyModelOptions,
	KeyModelThinking,
	KeyModelPlanPinned,
	KeyModelSourceOrder,
}

// modelSourceOrder is the precedence a model selection follows, weakest
// first. It is exported as a field rather than left to the reader, because
// three layers that disagree are only readable next to the order that
// produced them.
var modelSourceOrder = []string{
	string(SourceDefault),
	string(SourceConfigFile),
	string(SourceEnv),
	string(SourceSession),
	string(SourcePlan),
}

// modelReason states what this category deliberately leaves out. A model that
// reads the catalog learns the exclusions before spending a call discovering
// them, and the list is the security contract, not a temporary gap.
const modelReason = "the model selection and the engine's model state; " +
	"api keys, base URLs, provider identity and the skill path are not exported here, " +
	"and no credential is ever reported as a value, a hash or a suffix"

// Source refs for the layers whose origin is a config key rather than the
// selection itself. They name the key that sets the value, so an explanation
// says where to go and change it.
var (
	sourceModelSelection = Source{Kind: SourceComputed, Ref: "models[].api_name, else the model name"}
	sourceModelProtocol  = Source{
		Kind: SourceConfigFile,
		Ref:  "models[].protocol, sniffed from models[].base_url when the key is absent",
	}
	sourceModelEffort        = Source{Kind: SourceConfigFile, Ref: "models[].reasoning_effort"}
	sourceModelRequestEffort = Source{
		Kind: SourceComputed,
		Ref:  "the selected variant's reasoning_effort, else the selected effort level",
	}
	sourceModelEffortLevels = Source{
		Kind: SourceComputed,
		Ref:  "the provider catalog or the variants of an imported model",
	}
	sourceModelWindow    = Source{Kind: SourceConfigFile, Ref: "models[].context_window"}
	sourceModelMaxOutput = Source{Kind: SourceConfigFile, Ref: "models[].max_output_tokens"}
	sourceModelVariants  = Source{Kind: SourceComputed, Ref: "the variants of an imported model"}
	sourceModelOptions   = Source{
		Kind: SourceComputed,
		Ref:  "the model's options with the selected variant's fragment overlaid",
	}
	sourceModelThinking = Source{Kind: SourceComputed, Ref: "derived from the request model name"}
	sourceModelPin      = Source{Kind: SourcePlan, Ref: "plan step model pin"}
	sourceModelBudget   = Source{
		Kind: SourceComputed,
		Ref:  "the model window narrowed by the spawn ceiling and the session context override",
	}
	sourceModelRound = Source{
		Kind: SourceComputed,
		Ref:  "the round in flight kept the model it started on",
	}
	sourceModelAfterLoad = Source{
		Kind: SourceSession,
		Ref:  "a session-time choice: /model, a remembered pick, a resumed session or a connected provider",
	}
)

// ModelFacts is everything the harness may say about one model
// configuration. It is an allowlist by construction: these members exist
// because they are safe to publish, and a model's api key, base URL,
// authenticator and skill path have no member here to land in. The
// projection from the owner's own struct lives with the owner, so this
// package never holds a type that carries a credential and no edit made here
// can start leaking one.
type ModelFacts struct {
	// Known is false when the owner could not be read at all — no config
	// loaded, no engine built. Every layer fed from it then reports
	// unavailable rather than a zero that would read as a real answer.
	Known bool
	// Name is the user-facing selector; RequestName is what goes on the wire.
	Name        string
	RequestName string
	// Protocol is the wire protocol of the endpoint, not the endpoint.
	Protocol string
	// Effort is the selected reasoning level; RequestEffort is what the
	// request actually carries once a variant's own value overlays it.
	Effort        string
	RequestEffort string
	// EffortLevels are the levels selectable at runtime. Empty means the
	// model offers no choice — not that the choice is unknown.
	EffortLevels    []string
	ContextWindow   int
	MaxOutputTokens int
	// Variants names the model's option fragments; Options names the tuning
	// options that are set. Both are names only: an option's value is
	// whatever a config file put there, so no value from it is exported.
	Variants []string
	Options  []string
	Thinking bool
}

// ModelActing is the selection the round in flight is running on. It is a
// name and an effort and nothing else, because that is all the engine keeps
// for a round once it has started: the rest of the round's model is not
// observable, and this DTO does not pretend otherwise.
type ModelActing struct {
	Name   string
	Effort string
}

// ModelState is one coherent read of the engine's model state: what it holds
// for the next round, what the round in flight runs on, the window it
// budgets against, whether a plan step pinned the model, and the generation
// of all of it. It is one value rather than five accessors so the layers of
// a single observation cannot disagree with each other.
type ModelState struct {
	// Known is false when there is no engine to read.
	Known    bool
	Loaded   ModelFacts
	Acting   ModelActing
	Window   int
	Pinned   bool
	Revision string
}

// ModelDeps binds the model collector to its two owners. The loader answers
// what was configured; the engine answers what is loaded and what is acting.
// They stay separate on purpose: the effective layer must never be recomputed
// by re-applying the configuration, because that is exactly how a saved
// default gets presented as the model that is running.
type ModelDeps struct {
	// Configured is what the loader resolved at startup — config file plus
	// environment, as it stands now. Reading it never re-reads the file.
	Configured func() ModelFacts
	// ConfiguredSource is where that selection came from.
	ConfiguredSource func() Source
	// State is the engine's own answer, read in one pass.
	State func() ModelState
}

// ModelSelectionSource names where the loader's default model selection came
// from. It takes the two facts the loader actually records — whether the
// environment pinned the entry, and whether the config file declared a
// default — because those are the only origins that exist: anything else is
// the fallback to the first entry. It never guesses.
func ModelSelectionSource(envOverride, defaultDeclared bool) Source {
	switch {
	case envOverride:
		return Source{Kind: SourceEnv, Ref: "COZYPHI_MODEL"}
	case defaultDeclared:
		return Source{Kind: SourceConfigFile, Ref: "models[].default"}
	default:
		return Source{Kind: SourceDefault, Ref: "the first models[] entry"}
	}
}

// modelCollector observes the model: what was selected, what the engine took
// in, and what is answering right now.
type modelCollector struct {
	deps ModelDeps
}

// NewModelCollector builds the model collector.
func NewModelCollector(deps ModelDeps) Collector {
	return &modelCollector{deps: deps}
}

func (*modelCollector) Category() Category { return CategoryModel }

// Status is answered from the declared key set alone: listing the catalog
// reads neither the loader nor the engine.
func (*modelCollector) Status() Status {
	keys := make([]string, len(modelKeys))
	copy(keys, modelKeys)
	return Status{Availability: AvailabilityAvailable, Reason: modelReason, Keys: keys}
}

// Collect reads both owners once and derives every field from that one read.
// Nothing here reloads configuration, opens a connection or asks the engine
// to change anything.
func (c *modelCollector) Collect(_ context.Context) ([]Field, error) {
	m := c.layers()
	return []Field{
		m.name(),
		m.capability(KeyModelRequestName, ApplyNextTurn, sourceModelSelection, factRequestName),
		m.capability(KeyModelProtocol, ApplyRestart, sourceModelProtocol, factProtocol),
		m.effort(),
		m.selected(KeyModelRequestEffort, ApplyNextTurn, sourceModelRequestEffort, factRequestEffort),
		m.capability(KeyModelEffortLevels, ApplyRestart, sourceModelEffortLevels, factEffortLevels),
		m.contextWindow(),
		m.capability(KeyModelMaxOutputTokens, ApplyRestart, sourceModelMaxOutput, factMaxOutput),
		m.capability(KeyModelVariants, ApplyRestart, sourceModelVariants, factVariants),
		m.selected(KeyModelOptions, ApplyRestart, sourceModelOptions, factOptions),
		m.capability(KeyModelThinking, ApplyNextTurn, sourceModelThinking, factThinking),
		m.planPinned(),
		m.sourceOrder(),
	}, nil
}

// modelLayers is one observation of both owners, taken once and then used for
// every field, so the thirteen fields of a snapshot describe the same instant
// rather than thirteen slightly different ones.
type modelLayers struct {
	configured ModelFacts
	selection  Source
	state      ModelState
	// sameModel is true when the engine holds the model entry the loader
	// resolved. The capabilities of a model belong to its entry, so this is
	// what decides whether the configured source still explains them.
	sameModel bool
	// sameSelection additionally requires the effort to match: an effort
	// changed after the load is a session-time choice even when the model
	// itself is the configured one.
	sameSelection bool
	// settled is true when the round in flight runs the model the engine
	// holds. While it is false the two disagree, and the fields say so
	// instead of reporting the next round's answer as the current one.
	settled bool
}

func (c *modelCollector) layers() modelLayers {
	configured := callModelFacts(c.deps.Configured)
	state := callModelState(c.deps.State)
	m := modelLayers{
		configured: configured,
		selection:  callSource(c.deps.ConfiguredSource),
		state:      state,
	}
	m.sameModel = configured.Known && state.Known && configured.Name == state.Loaded.Name
	m.sameSelection = m.sameModel && configured.Effort == state.Loaded.Effort
	m.settled = state.Acting.Name == state.Loaded.Name && state.Acting.Effort == state.Loaded.Effort
	return m
}

// loadedSource says where the model the engine holds came from. It does not
// guess: a plan pin and an unchanged loader selection are both observable
// facts, and everything else happened after the load — a remembered pick, a
// resumed session, a connected provider or /model — so the ref names that set
// honestly instead of picking one of its members.
func (m modelLayers) loadedSource(configured Source, same bool) Source {
	switch {
	case m.state.Pinned:
		return sourceModelPin
	case same:
		return configured
	default:
		return sourceModelAfterLoad
	}
}

// scope is how far a model value reaches. A plan pin narrows everything about
// the model to the step that pinned it, because closing the plan or reaching
// a step without a pin hands the session model back.
func (m modelLayers) scope() Scope {
	if m.state.Known && m.state.Pinned {
		return ScopeStep
	}
	return ScopeSession
}

// capability builds a field describing the model entry itself: its wire name,
// protocol, limits and options. The three layers all read the same member of
// ModelFacts, from the loader, from the engine, and — while the round in
// flight is running the model the engine holds — from the engine again.
func (m modelLayers) capability(key string, apply Apply, source Source, pick modelFact) Field {
	return m.field(key, apply, source, pick, m.sameModel)
}

// selected builds a field that follows the selected effort as well as the
// model, so a session that changed only the effort is attributed to the
// session rather than to the config entry that still names the model.
func (m modelLayers) selected(key string, apply Apply, source Source, pick modelFact) Field {
	return m.field(key, apply, source, pick, m.sameSelection)
}

func (m modelLayers) field(key string, apply Apply, source Source, pick modelFact, same bool) Field {
	configuredValue, configuredSet := pick(m.configured)
	loadedValue, loadedSet := pick(m.state.Loaded)
	loadedSource := m.loadedSource(source, same)
	field := Field{
		Key:        key,
		Configured: modelObservation(m.configured.Known, configuredSet, configuredValue, source),
		Loaded:     modelObservation(m.state.Known, loadedSet, loadedValue, loadedSource),
		Effective:  Unavailable(),
		Apply:      apply,
		Scope:      m.scope(),
		Revision:   m.state.Revision,
	}
	// A round in flight keeps the model it started on, and the engine
	// remembers only its name and effort. While that round runs a different
	// model, the rest of what it is running is genuinely not observable from
	// here, and the field says unavailable rather than describing the model
	// that will answer next.
	if m.state.Known && m.settled {
		field.Effective = modelObservation(true, loadedSet, loadedValue, loadedSource)
	}
	return field
}

// name is the model the session is on. Its effective layer comes from the
// round in flight, never from the configured default: a saved default
// presented as the model that is answering is precisely the confusion this
// category exists to remove, so with no engine to read it stays unavailable.
func (m modelLayers) name() Field {
	source := m.loadedSource(m.selection, m.sameModel)
	return Field{
		Key: KeyModelName,
		Configured: modelObservation(
			m.configured.Known, m.configured.Name != "", StringValue(m.configured.Name), m.selection),
		Loaded: modelObservation(
			m.state.Known, m.state.Loaded.Name != "", StringValue(m.state.Loaded.Name), source),
		Effective: m.acting(m.state.Acting.Name != "", StringValue(m.state.Acting.Name), source),
		Apply:     ApplyNextTurn,
		Scope:     m.scope(),
		Revision:  m.state.Revision,
	}
}

// effort is the reasoning depth in force. An effort the model does not offer
// never reaches the engine — the owner refuses the switch — so this field
// reports what was accepted and has no way to show a level that was asked for
// and rejected.
func (m modelLayers) effort() Field {
	source := m.loadedSource(sourceModelEffort, m.sameSelection)
	return Field{
		Key: KeyModelEffort,
		Configured: modelObservation(
			m.configured.Known, m.configured.Effort != "", StringValue(m.configured.Effort), sourceModelEffort),
		Loaded: modelObservation(
			m.state.Known, m.state.Loaded.Effort != "", StringValue(m.state.Loaded.Effort), source),
		Effective: m.acting(m.state.Acting.Effort != "", StringValue(m.state.Acting.Effort), source),
		Apply:     ApplyNextTurn,
		Scope:     m.scope(),
		Revision:  m.state.Revision,
	}
}

// contextWindow separates the model's own window from the number the engine
// budgets against. The effective layer is the second one, and it is marked
// computed even when the two agree, because a session override or a spawn
// ceiling can narrow it at any time without the model's window changing.
func (m modelLayers) contextWindow() Field {
	loadedSource := m.loadedSource(sourceModelWindow, m.sameModel)
	effective := Unavailable()
	if m.state.Known {
		effective = modelObservation(
			true, m.state.Window > 0, IntValue(int64(m.state.Window)), sourceModelBudget)
	}
	return Field{
		Key: KeyModelContextWindow,
		Configured: modelObservation(m.configured.Known, m.configured.ContextWindow > 0,
			IntValue(int64(m.configured.ContextWindow)), sourceModelWindow),
		Loaded: modelObservation(m.state.Known, m.state.Loaded.ContextWindow > 0,
			IntValue(int64(m.state.Loaded.ContextWindow)), loadedSource),
		Effective: effective,
		// The session override lands on the running engine at once; the
		// model's own window needs a config edit and a restart. Apply names
		// the cheapest path from a change to an effect.
		Apply:    ApplyImmediate,
		Scope:    m.scope(),
		Revision: m.state.Revision,
	}
}

// planPinned reports whether a plan step is holding the model. Nothing in a
// config file can pin a step's model, so the configured layer does not exist
// for it rather than reporting a false that would look like a setting.
func (m modelLayers) planPinned() Field {
	observation := Unavailable()
	if m.state.Known {
		observation = modelObservation(true, m.state.Pinned, BoolValue(m.state.Pinned), sourceModelPin)
	}
	return Field{
		Key:        KeyModelPlanPinned,
		Configured: NotApplicable(SourcePlan),
		Loaded:     observation,
		Effective:  observation,
		Apply:      ApplyNextTurn,
		Scope:      ScopeStep,
		Revision:   m.state.Revision,
	}
}

// sourceOrder publishes the override order the other twelve fields are read
// against. It is compiled into the build, so it has no configured or loaded
// layer to report.
func (m modelLayers) sourceOrder() Field {
	return Field{
		Key:        KeyModelSourceOrder,
		Configured: NotApplicable(SourceBuild),
		Loaded:     NotApplicable(SourceBuild),
		Effective: Present(ListValue(modelSourceOrder),
			Source{Kind: SourceBuild, Ref: "model selection precedence, weakest first"}),
		Apply:    ApplyRestart,
		Scope:    ScopeSession,
		Revision: m.state.Revision,
	}
}

// acting builds an effective layer from the round in flight. With no engine
// there is nothing acting, and the layer says so.
func (m modelLayers) acting(set bool, value Value, source Source) Observation {
	if !m.state.Known {
		return Unavailable()
	}
	if !m.settled {
		source = sourceModelRound
	}
	return modelObservation(true, set, value, source)
}

// modelObservation is the one place the three empty answers are told apart:
// an owner that could not be read is unavailable, a layer nothing set is
// unset (carrying the zero a caller would get), and everything else is
// present.
func modelObservation(known, set bool, value Value, source Source) Observation {
	switch {
	case !known:
		return Unavailable()
	case !set:
		return Unset(value, source)
	default:
		return Present(value, source)
	}
}

// modelFact reads one member out of the facts and says whether anything set
// it. The bool is what keeps a zero window or an empty effort list legible as
// "nothing set this" instead of "the value is 0".
type modelFact func(ModelFacts) (Value, bool)

func factRequestName(f ModelFacts) (Value, bool) {
	return StringValue(f.RequestName), f.RequestName != ""
}

func factProtocol(f ModelFacts) (Value, bool) { return StringValue(f.Protocol), f.Protocol != "" }

func factRequestEffort(f ModelFacts) (Value, bool) {
	return StringValue(f.RequestEffort), f.RequestEffort != ""
}

func factEffortLevels(f ModelFacts) (Value, bool) {
	return ListValue(f.EffortLevels), len(f.EffortLevels) > 0
}

func factMaxOutput(f ModelFacts) (Value, bool) {
	return IntValue(int64(f.MaxOutputTokens)), f.MaxOutputTokens > 0
}

func factVariants(f ModelFacts) (Value, bool) { return ListValue(f.Variants), len(f.Variants) > 0 }

func factOptions(f ModelFacts) (Value, bool) { return ListValue(f.Options), len(f.Options) > 0 }

// factThinking is derived rather than set, so what makes it present is having
// had an input to derive it from: a model with no name yields a false that
// means "nothing to derive this from", not "this model does not think".
func factThinking(f ModelFacts) (Value, bool) {
	return BoolValue(f.Thinking), f.RequestName != ""
}

// callModelFacts and its siblings read optional accessors. A nil accessor is
// a wiring gap, and the layers it feeds report unavailable rather than
// crashing the snapshot.
func callModelFacts(accessor func() ModelFacts) ModelFacts {
	if accessor == nil {
		return ModelFacts{}
	}
	return accessor()
}

func callModelState(accessor func() ModelState) ModelState {
	if accessor == nil {
		return ModelState{}
	}
	return accessor()
}

func callSource(accessor func() Source) Source {
	if accessor == nil {
		return Source{Kind: SourceUnknown}
	}
	return accessor()
}
