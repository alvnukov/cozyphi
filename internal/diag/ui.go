package diag

import (
	"context"
)

// UI field keys. Each subject takes its own namespace inside the ui category
// for the same reason the stores do inside storage: a key is a stable address
// for Explain, and the four subjects of a surface grow apart.
//
//nolint:gosec // G101: field addresses, not credentials; no value is one.
const (
	KeySurfaceKind = "surface.kind"

	KeyThemeName    = "theme.name"
	KeyThemeBuiltin = "theme.builtin"

	KeyKeymapMode        = "keymap.mode"
	KeyKeybindsCommands  = "keybinds.commands"
	KeyKeybindsOverrides = "keybinds.overrides"

	KeyNotifyMode     = "notify.mode"
	KeyNotifySound    = "notify.sound"
	KeyNotifyDelivery = "notify.delivery"
	KeyNotifyFocus    = "notify.focus"

	KeyVoiceState      = "voice.state"
	KeyVoiceBackend    = "voice.backend"
	KeyVoiceCapture    = "voice.capture"
	KeyVoiceModel      = "voice.model"
	KeyVoiceCredential = "voice.credential"
)

// uiKeys is the declared key set, in the order Collect returns them: the
// shape of the surface first, because it decides what the rest can mean,
// then what it looks like, what it answers to, how it reaches the user
// outside the terminal, and what it hears.
var uiKeys = []string{
	KeySurfaceKind,
	KeyThemeName,
	KeyThemeBuiltin,
	KeyKeymapMode,
	KeyKeybindsCommands,
	KeyKeybindsOverrides,
	KeyNotifyMode,
	KeyNotifySound,
	KeyNotifyDelivery,
	KeyNotifyFocus,
	KeyVoiceState,
	KeyVoiceBackend,
	KeyVoiceCapture,
	KeyVoiceModel,
	KeyVoiceCredential,
}

// uiReason states what this category answers and what it deliberately leaves
// out. The exclusions are as much a part of the contract as the keys: this is
// the one category whose subjects can act on the world — a notification is
// sent, a microphone is opened, a screen is repainted — and none of that
// happens because somebody asked a question about it. It is written to fit
// the response budget whole: a reason the bounder cuts would lose the
// exclusions, which are the half worth reading.
const uiReason = "the surface this session runs behind: whether anything renders, the palette in " +
	"force and the ones built in, the dialect the key table was compiled for and the commands the " +
	"configuration rebinds, whether a desktop notification would arrive, and whether speech input " +
	"is resolved and listening. States and names only: no chord spelling, no command line, no audio " +
	"device, no endpoint, no notification text, no credential by value. Answering renders nothing, " +
	"sends nothing, opens no microphone and changes nothing"

// UIShape is what a process renders through. It is the first thing the
// category answers, because every runtime layer below it means something
// different depending on it: a headless run does not have a dimmed theme, it
// has no theme at all.
type UIShape string

// UIShape values.
const (
	// UIShapeTerminal means a terminal UI is attached and painting.
	UIShapeTerminal UIShape = "terminal"
	// UIShapeNone means nothing renders: the process was started to run a
	// prompt and exit, so every layer a surface would own is absent rather
	// than unreported.
	UIShapeNone UIShape = "none"
)

// UIDeps binds the ui collector to the two owners of what a surface is. They
// are deliberately separate: what a source asked for survives a process that
// renders nothing, and what a surface does with it only exists while one is
// attached.
type UIDeps struct {
	// Config observes what the configuration and the stored preferences
	// asked the surface to be. It is answered in every process shape, so a
	// headless run still reports a notification mode somebody set. No
	// accessor here may read the preferences file or reload the config: the
	// values are the ones their owners already hold.
	Config func() UIConfigFacts
	// Surface observes the surface's own detached account of itself, handed
	// over on the goroutine that owns it rather than read from a background
	// one. No accessor here may render a frame, send a notification, open a
	// microphone, or change a palette, a keybinding or a preference.
	Surface func() UISurfaceFacts
}

// UIConfigFacts is what a source asked the surface to be. It is an allowlist
// by construction: there is no member for a chord spelling, a command line,
// an audio device, an endpoint or a credential to land in.
type UIConfigFacts struct {
	// Known is false when nobody published the configuration — the layer is
	// not wired, rather than wired and empty.
	Known bool
	// KeymapRead is whether the stored UI preferences were read at all. They
	// are read once, where a session is built; a read that failed leaves the
	// dialect unknown rather than reported as the default.
	KeymapRead bool
	// Keymap is the editing dialect the preferences persist. Empty means
	// nothing was persisted and the composer starts in its own default.
	Keymap string
	// Keybinds is what the configuration's keybinds section names.
	Keybinds KeybindConfigFacts
	// Notifications is the notifications section, decoded by its owner.
	Notifications NotifyConfigFacts
	// Voice is the voice section, decoded by its owner.
	Voice VoiceConfigFacts
}

// UISurfaceFacts is the surface's own account of itself, taken on the
// goroutine that owns it and handed over detached. Nothing here points back
// at a widget: a snapshot describes the moment it was published and can never
// be dereferenced into live UI state from a tool goroutine.
type UISurfaceFacts struct {
	// Known is false when no surface has published anything. In a process
	// that renders nothing that is the wiring; in one that does, it means the
	// answer has not been handed over yet, and the two are told apart by
	// Shape below rather than merged into one silence.
	Known bool
	// Shape is what this process renders through.
	Shape UIShape
	// Theme is the palette in force and the ones this build carries.
	Theme ThemeFacts
	// Keys is the compiled binding table's own account of itself.
	Keys KeybindRuntimeFacts
	// Notifications is the live notifier's account of itself.
	Notifications NotifierFacts
	// Voice is the live speech-input session's account of itself.
	Voice VoiceRuntimeFacts
	// Revision fingerprints the state this observation describes, so two
	// snapshots taken across a theme switch or a recording are visibly of two
	// different states.
	Revision string
}

// Sources every subject of the surface shares.
var (
	sourceUINoSurface = Source{
		Kind: SourceUnknown,
		Ref: "no surface has published its state to this session yet, so what one would be doing is not " +
			"something this view may reach for on its own",
	}
	sourceUIHeadless = Source{
		Kind: SourceComputed,
		Ref: "this process renders nothing, so there is no surface to hold this — what was configured for " +
			"one is reported all the same",
	}
	sourceUIShapeFixed = Source{
		Kind: SourceComputed,
		Ref:  "nothing configures the shape of the surface: the entry point fixes it before any session exists",
	}
	sourceUIShapeBuilt = Source{
		Kind: SourceCLIFlag,
		Ref:  "what this process was started to render through",
	}
	sourceUIShapeLive = Source{
		Kind: SourceComputed,
		Ref:  "what is rendering now, which is what decides whether the runtime layers below exist at all",
	}
)

// uiCollector observes the surface this session runs behind.
type uiCollector struct {
	deps UIDeps
}

// NewUICollector builds the ui collector.
func NewUICollector(deps UIDeps) Collector {
	return &uiCollector{deps: deps}
}

func (*uiCollector) Category() Category { return CategoryUI }

// Status is answered from the declared key set alone: listing the catalog
// renders nothing, touches no notifier and opens no microphone.
func (*uiCollector) Status() Status {
	keys := make([]string, len(uiKeys))
	copy(keys, uiKeys)
	return Status{Availability: AvailabilityAvailable, Reason: uiReason, Keys: keys}
}

// Collect reads both owners once and derives every field from that one read,
// so the fields of one answer describe one moment rather than several.
func (c *uiCollector) Collect(_ context.Context) ([]Field, error) {
	config := callUIConfigFacts(c.deps.Config)
	surface := callUISurfaceFacts(c.deps.Surface)
	return []Field{
		surface.kind(),
		surface.themeName(),
		surface.themeBuiltin(),
		surface.keymapMode(config),
		surface.keybindCommands(config),
		surface.keybindOverrides(config),
		surface.notifyMode(config),
		surface.notifySound(config),
		surface.notifyDelivery(),
		surface.notifyFocus(),
		surface.voiceState(config),
		surface.voiceBackend(config),
		surface.voiceCapture(config),
		surface.voiceModel(config),
		surface.voiceCredential(config),
	}, nil
}

// kind is what this process renders through: what nothing configures, what
// the entry point built, and what is painting now.
func (s UISurfaceFacts) kind() Field {
	field := s.field(KeySurfaceKind, ApplyRestart, ScopeProcess)
	field.Configured = notApplicable(sourceUIShapeFixed)
	if !s.Known {
		return field
	}
	field.Loaded = Present(StringValue(string(s.Shape)), sourceUIShapeBuilt)
	field.Effective = Present(StringValue(string(s.Shape)), sourceUIShapeLive)
	return field
}

// runtime is what a layer the surface owns says when the surface itself is
// the answer. A process that renders nothing has no such layer at all; one
// that does but has published nothing has a layer nobody has reported. The
// two are different answers and are never merged into one silence.
func (s UISurfaceFacts) runtime(observe func() Observation) Observation {
	switch {
	case !s.Known:
		return unknown(sourceUINoSurface)
	case s.Shape != UIShapeTerminal:
		return notApplicable(sourceUIHeadless)
	default:
		return observe()
	}
}

// field is the shape every ui field starts from: all three layers
// unavailable, so a layer nobody wired degrades into an honest answer rather
// than into a zero that would read as a setting somebody switched off.
//
// Every one of them is published rather than live. The surface belongs to
// the render goroutine and hands its account over when it changes; a tool
// goroutine reading that account is what keeps this view off state another
// goroutine owns, and the price is that the account is as old as the last
// change rather than as old as the question. Freshness says so on each
// field instead of leaving a reader to assume otherwise.
func (s UISurfaceFacts) field(key string, apply Apply, scope Scope) Field {
	return Field{
		Key:        key,
		Configured: Unavailable(),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      apply,
		Scope:      scope,
		Freshness:  FreshnessPublished,
		Revision:   s.Revision,
	}
}

// callUIConfigFacts reads the optional accessor. A nil accessor is a wiring
// gap, and every layer it feeds reports unavailable rather than crashing the
// snapshot.
func callUIConfigFacts(accessor func() UIConfigFacts) UIConfigFacts {
	if accessor == nil {
		return UIConfigFacts{}
	}
	return accessor()
}

// callUISurfaceFacts reads the optional accessor, on the same terms.
func callUISurfaceFacts(accessor func() UISurfaceFacts) UISurfaceFacts {
	if accessor == nil {
		return UISurfaceFacts{}
	}
	return accessor()
}
