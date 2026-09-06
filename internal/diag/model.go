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
	KeyModelProvider        = "provider"
	KeyModelCredential      = "credential"
	KeyModelCredentialKind  = KeyModelCredential + ".kind"
	KeyModelCatalog         = "catalog.providers"
	KeyModelConnected       = "catalog.connected"
	KeyModelImport          = "import.opencode"
	KeyModelImportModels    = "import.opencode.models"
)

// modelKeys is the declared key set, in the order Collect returns them.
var modelKeys = []string{
	KeyModelName,
	KeyModelRequestName,
	KeyModelProvider,
	KeyModelProtocol,
	KeyModelCredential,
	KeyModelCredentialKind,
	KeyModelEffort,
	KeyModelRequestEffort,
	KeyModelEffortLevels,
	KeyModelContextWindow,
	KeyModelMaxOutputTokens,
	KeyModelVariants,
	KeyModelOptions,
	KeyModelThinking,
	KeyModelPlanPinned,
	KeyModelCatalog,
	KeyModelConnected,
	KeyModelImport,
	KeyModelImportModels,
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
const modelReason = "the model selection, the engine's model state, the provider catalog and " +
	"the read-only opencode import; a credential is reported as presence and kind only, " +
	"so api keys, tokens, base URLs, credential-store contents, import file contents, " +
	"load-error text and the skill path have no field here, and no credential is ever " +
	"reported as a value, a hash or a suffix"

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
	sourceModelProvider = Source{
		Kind: SourceComputed,
		Ref:  "the provider entry the model belongs to; empty for a model declared in models[]",
	}
	sourceModelCredential = Source{
		Kind: SourceComputed,
		Ref:  "presence only: the entry carries an api key or a request authenticator",
	}
	sourceModelCredentialKind = Source{
		Kind: SourceComputed,
		Ref:  "how the entry authenticates, never what it authenticates with",
	}
	sourceModelCatalogCache = Source{
		Kind: SourceConfigFile,
		Ref:  "the saved last-known-good provider catalog, read once at startup",
	}
	sourceModelCatalogBuiltin = Source{
		Kind: SourceBuild,
		Ref:  "the built-in provider table; no saved catalog was read",
	}
	sourceModelConnected = Source{
		Kind: SourceConfigFile,
		Ref:  "the credential store, read once at startup; provider names only",
	}
	sourceModelImportSetting = Source{Kind: SourceConfigFile, Ref: "opencode.enabled"}
	sourceModelImport        = Source{
		Kind: SourceComputed,
		Ref:  "the read-only opencode import as it resolved at startup",
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
	// Provider is the id of the provider entry this model came from — a
	// connected provider or an imported one. It is empty for a model declared
	// in models[], which belongs to no provider.
	Provider string
	// Credential says only that something to authenticate with is attached to
	// the entry. It is the whole of what the harness may say about a
	// credential's content: there is no member here for a key, a token, a
	// suffix or a hash of one.
	Credential bool
	// CredentialKind is how the entry authenticates: "api_key" for a stored
	// key that rides the request, "authenticator" for a token minted per
	// request. Empty means the entry carries neither.
	CredentialKind string
}

// ProviderFacts is what the harness may say about the provider catalog and
// the credential store: how many providers are known, which of them a
// credential exists for, and which of the two sources the catalog came from.
// Provider names are validated ids, so they are safe to publish; nothing
// stored beside them — endpoints, keys, tokens, account ids, expiry — has a
// member here to land in.
type ProviderFacts struct {
	// Known is false when there is no provider manager to read.
	Known bool
	// Catalog is how many providers the manager holds.
	Catalog int
	// Cached reports whether a saved catalog was read when the manager was
	// opened, as opposed to the built-in table standing alone. It is recorded
	// at open time, so answering it never touches the file again.
	Cached bool
	// Connected are the ids a credential is stored for, sorted. Presence is
	// the entire answer: this list says a credential exists, never what it is.
	Connected []string
	// Revision is the credential store's generation, so two observations can
	// be compared by generation rather than by value.
	Revision string
}

// ImportState is the state of a read-only import. The four values exist
// because they are four different answers to "why is nothing imported", and
// collapsing any two of them is how an import that failed gets mistaken for
// one that was switched off.
type ImportState string

// ImportState values.
const (
	// ImportDisabled means the setting turns the import off.
	ImportDisabled ImportState = "disabled"
	// ImportNotLoaded means the import is on but never ran — the sources it
	// resolves against failed before it was reached.
	ImportNotLoaded ImportState = "not_loaded"
	// ImportFailed means the import ran and failed. Why it failed is not
	// reported: the error names files and can quote what it could not parse.
	ImportFailed ImportState = "failed"
	// ImportLoaded means the import ran and produced a source.
	ImportLoaded ImportState = "loaded"
)

// ImportFacts is what the harness may say about one read-only import: what
// state it is in and how much it contributed. The files it read, the
// providers it names, the keys it carries and the text of any failure all
// stay with the owner.
type ImportFacts struct {
	// Known is false when nothing published the import's state.
	Known bool
	State ImportState
	// Models is how many models the import contributed. It is meaningful only
	// in ImportLoaded; every other state reports no count rather than a zero
	// that would read as "it produced none".
	Models int
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
	// Providers is the provider manager's own answer about its catalog and
	// its credential store. It is read, never refreshed: no accessor here may
	// fetch a catalog, re-read a store or authenticate anything.
	Providers func() ProviderFacts
	// Import is the read-only import's state as it resolved at startup. The
	// import runs once, so this reports what happened then rather than trying
	// it again.
	Import func() ImportFacts
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
		m.capability(KeyModelProvider, ApplyNextTurn, sourceModelProvider, factProvider),
		m.capability(KeyModelProtocol, ApplyRestart, sourceModelProtocol, factProtocol),
		m.capability(KeyModelCredential, ApplyRestart, sourceModelCredential, factCredential),
		m.capability(KeyModelCredentialKind, ApplyRestart, sourceModelCredentialKind, factCredentialKind),
		m.effort(),
		m.selected(KeyModelRequestEffort, ApplyNextTurn, sourceModelRequestEffort, factRequestEffort),
		m.capability(KeyModelEffortLevels, ApplyRestart, sourceModelEffortLevels, factEffortLevels),
		m.contextWindow(),
		m.capability(KeyModelMaxOutputTokens, ApplyRestart, sourceModelMaxOutput, factMaxOutput),
		m.capability(KeyModelVariants, ApplyRestart, sourceModelVariants, factVariants),
		m.selected(KeyModelOptions, ApplyRestart, sourceModelOptions, factOptions),
		m.capability(KeyModelThinking, ApplyNextTurn, sourceModelThinking, factThinking),
		m.planPinned(),
		m.catalogProviders(),
		m.catalogConnected(),
		m.importState(),
		m.importModels(),
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
	// providers and imported are the other two owners: the provider manager
	// and the read-only import. They answer process-wide questions, so their
	// fields carry their own revision and scope rather than the engine's.
	providers ProviderFacts
	imported  ImportFacts
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
		providers:  callProviderFacts(c.deps.Providers),
		imported:   callImportFacts(c.deps.Import),
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

// catalogProviders is how many providers the manager holds. Nobody
// configures a catalog — it is fetched and cached — so the configured layer
// does not exist for it; the source says which of the two origins the loaded
// one came from, the saved file or the built-in table alone.
func (m modelLayers) catalogProviders() Field {
	source := sourceModelCatalogBuiltin
	if m.providers.Cached {
		source = sourceModelCatalogCache
	}
	observation := Unavailable()
	if m.providers.Known {
		observation = modelObservation(
			true, m.providers.Catalog > 0, IntValue(int64(m.providers.Catalog)), source)
	}
	return Field{
		Key:        KeyModelCatalog,
		Configured: NotApplicable(SourceConfigFile),
		Loaded:     observation,
		Effective:  observation,
		// A refresh or a /connect installs a new catalog in the running
		// manager, and the next observation sees it.
		Apply:    ApplyImmediate,
		Scope:    ScopeProcess,
		Revision: m.providers.Revision,
	}
}

// catalogConnected names the providers a credential is stored for. The names
// are the whole answer: that a credential exists is a fact about the store,
// and nothing about its content — kind, endpoint, account, expiry, let alone
// the secret — is reachable from this field. An empty list is a real answer,
// reported as unset: the store was read and holds nothing.
func (m modelLayers) catalogConnected() Field {
	observation := Unavailable()
	if m.providers.Known {
		observation = modelObservation(true, len(m.providers.Connected) > 0,
			ListValue(m.providers.Connected), sourceModelConnected)
	}
	return Field{
		Key:        KeyModelConnected,
		Configured: NotApplicable(SourceConfigFile),
		Loaded:     observation,
		Effective:  observation,
		Apply:      ApplyImmediate,
		Scope:      ScopeProcess,
		Revision:   m.providers.Revision,
	}
}

// importState separates what the setting asked for from what the import did.
// The four loaded states are four different answers, and telling them apart
// is the point: an import switched off, one that never ran because the
// catalog it resolves against failed first, one that ran and failed, and one
// that ran. Why a failure failed is deliberately absent — the error names
// the files it read and can quote what it could not parse.
func (m modelLayers) importState() Field {
	configured := Unavailable()
	observation := Unavailable()
	if m.imported.Known {
		configured = Present(StringValue(importSetting(m.imported.State)), sourceModelImportSetting)
		observation = Present(StringValue(string(m.imported.State)), sourceModelImport)
	}
	return Field{
		Key:        KeyModelImport,
		Configured: configured,
		Loaded:     observation,
		Effective:  observation,
		// The import is read once, while the process starts.
		Apply:    ApplyRestart,
		Scope:    ScopeProcess,
		Revision: m.state.Revision,
	}
}

// importModels is how many models the import contributed, and it is reported
// only where that number means something. A disabled import has no count to
// have; one that failed or never ran has a count nobody knows, and a zero
// there would read as "it imported nothing".
func (m modelLayers) importModels() Field {
	observation := Unavailable()
	switch {
	case !m.imported.Known:
	case m.imported.State == ImportDisabled:
		observation = NotApplicable(SourceConfigFile)
	case m.imported.State == ImportLoaded:
		observation = modelObservation(
			true, m.imported.Models > 0, IntValue(int64(m.imported.Models)), sourceModelImport)
	}
	return Field{
		Key:        KeyModelImportModels,
		Configured: NotApplicable(SourceConfigFile),
		Loaded:     observation,
		Effective:  observation,
		Apply:      ApplyRestart,
		Scope:      ScopeProcess,
		Revision:   m.state.Revision,
	}
}

// importSetting reports the setting behind a state. Only ImportDisabled comes
// from the setting being off; every other state is the setting being on and
// the import then doing something.
func importSetting(state ImportState) string {
	if state == ImportDisabled {
		return "disabled"
	}
	return "enabled"
}

// sourceOrder publishes the override order the other nineteen fields are read
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

func factProvider(f ModelFacts) (Value, bool) { return StringValue(f.Provider), f.Provider != "" }

// factCredential is present for every entry that could be read, false
// included: an entry with no credential is running unauthenticated, which is
// an answer, not a missing one.
func factCredential(f ModelFacts) (Value, bool) { return BoolValue(f.Credential), f.Known }

func factCredentialKind(f ModelFacts) (Value, bool) {
	return StringValue(f.CredentialKind), f.CredentialKind != ""
}

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

func callProviderFacts(accessor func() ProviderFacts) ProviderFacts {
	if accessor == nil {
		return ProviderFacts{}
	}
	return accessor()
}

func callImportFacts(accessor func() ImportFacts) ImportFacts {
	if accessor == nil {
		return ImportFacts{}
	}
	return accessor()
}

func callSource(accessor func() Source) Source {
	if accessor == nil {
		return Source{Kind: SourceUnknown}
	}
	return accessor()
}
