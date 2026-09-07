package diag

import "strconv"

// VoiceMissing names what an unresolved transcription backend lacks. It is a
// closed vocabulary rather than the owner's hint sentence: the hint is prose
// meant for a person and can carry a path, while this is the machine-readable
// half and can carry nothing else.
type VoiceMissing string

// VoiceMissing values.
const (
	// VoiceMissingNone means nothing is missing.
	VoiceMissingNone VoiceMissing = ""
	// VoiceMissingBinary means the transcription binary is not on PATH.
	VoiceMissingBinary VoiceMissing = "binary"
	// VoiceMissingModel means the binary is installed but no model is.
	VoiceMissingModel VoiceMissing = "model"
	// VoiceMissingConfiguredModel means a model is pinned and nothing on
	// this machine matches it; another one is never silently used instead.
	VoiceMissingConfiguredModel VoiceMissing = "configured_model"
	// VoiceMissingOther means the backend did not resolve for a reason that
	// is not a missing artifact — a malformed command line, say.
	VoiceMissingOther VoiceMissing = "other"
)

// VoiceCaptureKind is how microphone audio is asked for. It says which of the
// two arrangements is in force and nothing about either: never the argv,
// never the device.
type VoiceCaptureKind string

// VoiceCaptureKind values.
const (
	// VoiceCapturePreset means the built-in recorder for this OS.
	VoiceCapturePreset VoiceCaptureKind = "preset"
	// VoiceCaptureCommand means a command line somebody supplied.
	VoiceCaptureCommand VoiceCaptureKind = "custom_command"
)

// VoiceTuning is the part of the voice section that shapes a segment without
// naming anything: the language asked for, how long a segment may run and
// what ends it, how long a transcription may take, whether a vocabulary hint
// travels with a request, and which dialect an endpoint speaks. The glossary
// somebody wrote is counted here and never carried: its terms are theirs.
type VoiceTuning struct {
	// Language is the language code a segment is transcribed in, or auto.
	Language string
	// MaxSeconds is the longest one segment may run.
	MaxSeconds int
	// SegmentSilenceMS is how much silence ends a segment.
	SegmentSilenceMS int
	// AutoPauseSeconds is how long an idle session waits before it lets the
	// microphone go.
	AutoPauseSeconds int
	// TimeoutSeconds is how long one transcription may take.
	TimeoutSeconds int
	// Hints is the hint mode voice.hints names: glossary or off.
	Hints string
	// GlossaryTerms is how many terms the glossary holds. The count only:
	// the terms are written by a person and never travel.
	GlossaryTerms int
	// Provider is the endpoint dialect voice.stt.provider names, or empty
	// when none is named.
	Provider string
}

// VoiceConfigFacts is the voice section as its owner decoded it. It is an
// allowlist by construction: there is no member for the capture argv, the
// transcription command line, the audio device, the endpoint or the key
// itself to land in, and the credential travels as a bool.
type VoiceConfigFacts struct {
	// Known is false when nobody published the section.
	Known bool
	// Enabled is whether speech input is switched on at all.
	Enabled bool
	// Backend is the configured transcription path: auto, command or http.
	Backend string
	// Capture is which of the two capture arrangements was asked for.
	Capture VoiceCaptureKind
	// Model is the model name voice.stt.model pins. Empty means nothing
	// pins one and the newest installed one is used.
	Model string
	// Credential is whether an API key is configured. Presence only: no
	// value, no hash, no suffix, no length.
	Credential bool
	// Tuning is the rest of the section: ceilings, language, hint mode and
	// provider, with the glossary as a count.
	Tuning VoiceTuning
}

// VoiceRuntimeFacts is the live speech-input session's account of itself:
// what resolved on this machine and what the session is doing with it. The
// resolved model travels as a file name and never as a path, and the failure
// that stopped a segment does not travel at all.
type VoiceRuntimeFacts struct {
	// Known is false when no voice session is attached to this surface.
	// A session is built even when voice is switched off, so that /voice can
	// say so — absence here means the wiring, not the setting.
	Known bool
	// Enabled is the session's own copy of voice.enabled. It is read from
	// the session rather than from the configuration because a model
	// installed while cozyphi runs reconfigures the session in place.
	Enabled bool
	// Backend is the transcription path that resolved on this machine, or
	// empty when none did.
	Backend string
	// Missing is why nothing resolved.
	Missing VoiceMissing
	// Model is the file name of the resolved model. Never the path it was
	// found at.
	Model string
	// Remote is whether the resolved backend transcribes over an endpoint
	// rather than on this machine. It is what decides whether a model file
	// and a credential are questions worth asking at all.
	Remote bool
	// Credential is whether the session holds an API key. Presence only.
	Credential bool
	// CaptureReady is whether a capture command resolved on this machine.
	CaptureReady bool
	// State is what the session is doing: idle, listening, paused or
	// finishing.
	State string
	// Pending is how many segments are queued or being transcribed.
	Pending int
	// GateBusy is whether the process-wide microphone admission is held —
	// possibly by another session's recording, which is exactly the case
	// worth seeing from here.
	GateBusy bool
	// Tuning is the session's own copy of the ceilings, language, hint mode
	// and provider it runs under, taken from the same reconfigurable copy as
	// Enabled.
	Tuning VoiceTuning
}

// voiceStateListening is the one session state this package has to recognize
// on its own: it is what tells this session's recording apart from the one it
// merely found the microphone held by.
const voiceStateListening = "listening"

// Sources for the voice session's layers.
var (
	sourceVoiceEnabled = Source{
		Kind: SourceConfigFile,
		Ref:  "whether voice.enabled switches speech input on",
	}
	sourceVoiceReadiness = Source{
		Kind: SourceComputed,
		Ref: "what the session came up as on this machine: switched off, resolved and ready, or on but " +
			"missing a piece it needs",
	}
	sourceVoiceLive = Source{
		Kind: SourceSession,
		Ref:  "what the session is doing right now",
	}
	sourceVoiceBackendConfigured = Source{
		Kind: SourceConfigFile,
		Ref:  "the voice.stt.backend the configuration asked for; auto picks one of the others at load time",
	}
	sourceVoiceBackendUnconfigured = Source{
		Kind: SourceConfigFile,
		Ref:  "voice.stt.backend names no transcription path",
	}
	sourceVoiceBackendResolved = Source{
		Kind: SourceComputed,
		Ref:  "the transcription path that resolved on this machine",
	}
	sourceVoiceBackendUsed = Source{
		Kind: SourceComputed,
		Ref:  "the path the next segment would take",
	}
	sourceVoiceBackendUnfed = Source{
		Kind: SourceComputed,
		Ref: "a transcription path resolved but no capture did, so no audio ever reaches it and no segment " +
			"is ever transcribed",
	}
	sourceVoiceMissingBinary = Source{
		Kind: SourceComputed,
		Ref:  "no transcription path resolved: the binary it needs is not on PATH",
	}
	sourceVoiceMissingModel = Source{
		Kind: SourceComputed,
		Ref:  "no transcription path resolved: the binary is installed but no model is",
	}
	sourceVoiceMissingConfiguredModel = Source{
		Kind: SourceComputed,
		Ref: "no transcription path resolved: voice.stt.model pins a model nothing here matches, and " +
			"another one is never used in its place",
	}
	sourceVoiceMissingOther = Source{
		Kind: SourceComputed,
		Ref: "no transcription path resolved, for a reason that is not a missing artifact; the sentence " +
			"that says which is written for a person and stays where it is",
	}
	sourceVoiceCaptureConfigured = Source{
		Kind: SourceConfigFile,
		Ref: "which capture arrangement voice.capture.command asks for — the preset for this OS or a " +
			"command line somebody supplied; neither the command line nor the device travels",
	}
	sourceVoiceCaptureResolved = Source{
		Kind: SourceComputed,
		Ref:  "whether a capture command resolved on this machine",
	}
	sourceVoiceCaptureLive = Source{
		Kind: SourceComputed,
		Ref: "what the microphone is doing: one recording is admitted across the whole process, so a " +
			"session that is not recording can still find it held elsewhere",
	}
	sourceVoiceModelPinned = Source{
		Kind: SourceConfigFile,
		Ref:  "the model voice.stt.model pins",
	}
	sourceVoiceModelUnpinned = Source{
		Kind: SourceConfigFile,
		Ref:  "voice.stt.model pins nothing, so the newest model installed here is used",
	}
	sourceVoiceModelResolved = Source{
		Kind: SourceComputed,
		Ref: "the file name of the model the next segment would be transcribed with; installing one while " +
			"cozyphi runs re-resolves it without a restart, so this is both what was loaded and what acts",
	}
	sourceVoiceModelRemote = Source{
		Kind: SourceComputed,
		Ref:  "the resolved backend transcribes over an endpoint, which carries its own model",
	}
	sourceVoiceModelSelfContained = Source{
		Kind: SourceComputed,
		Ref: "the transcription command substitutes no model, so it is a wrapper that knows its own " +
			"weights and none was looked for",
	}
	sourceVoiceModelUnresolved = Source{
		Kind: SourceComputed,
		Ref:  "no transcription path resolved, so no model was settled on either",
	}
	sourceVoiceKeyConfigured = Source{
		Kind: SourceConfigFile,
		Ref: "whether voice.stt.api_key holds a credential. Presence only: the value is never read into " +
			"this view, and neither is a hash, a suffix or a length of it",
	}
	sourceVoiceKeyNeeded = Source{
		Kind: SourceComputed,
		Ref:  "whether the resolved backend sends anywhere that would ask for a credential",
	}
	sourceVoiceKeyLocal = Source{
		Kind: SourceComputed,
		Ref:  "transcription runs locally, so nothing is ever sent and no credential is ever carried",
	}
	sourceVoiceKeySent = Source{
		Kind: SourceSession,
		Ref: "whether the next request would carry a credential. Presence only, from the session's own " +
			"copy, which a model installed mid-session may have replaced",
	}
	sourceVoiceUnattached = Source{
		Kind: SourceSession,
		Ref:  "no voice session is attached to this surface, so nothing resolved and nothing is listening",
	}
	sourceVoiceLanguage = Source{
		Kind: SourceConfigFile,
		Ref:  "the language code voice.language asks a segment to be transcribed in; auto lets the backend decide",
	}
	sourceVoiceLanguageUnset = Source{
		Kind: SourceConfigFile,
		Ref:  "voice.language names nothing, so the backend decides the language of each segment",
	}
	sourceVoiceLanguageSession = Source{
		Kind: SourceSession,
		Ref: "the language the session's own copy of the configuration asks for, which a model installed " +
			"mid-session may have replaced",
	}
	sourceVoiceLimits = Source{
		Kind: SourceConfigFile,
		Ref: "the ceilings the voice section sets: voice.max_seconds bounds one segment, " +
			"voice.segment_silence_ms is the silence that ends it, voice.auto_pause_seconds is how long an " +
			"idle session keeps the microphone, and voice.stt.timeout_seconds bounds one transcription",
	}
	sourceVoiceLimitsSession = Source{
		Kind: SourceSession,
		Ref:  "the same ceilings from the session's own copy of the configuration, which is what a segment runs under",
	}
	sourceVoiceHints = Source{
		Kind: SourceConfigFile,
		Ref: "voice.hints is the mode and voice.glossary is counted. The count only: the terms are written " +
			"by a person and never travel into this view",
	}
	sourceVoiceHintsSession = Source{
		Kind: SourceSession,
		Ref:  "the same mode and count from the session's own copy of the configuration",
	}
	sourceVoiceHintTravels = Source{
		Kind: SourceComputed,
		Ref: "whether a vocabulary hint would travel with the next segment: the mode is glossary and the " +
			"glossary is not empty. What the hint says is not reported",
	}
	sourceVoiceHintUnresolved = Source{
		Kind: SourceComputed,
		Ref:  "no backend resolved, so no segment goes anywhere and no hint travels",
	}
	sourceVoiceProvider = Source{
		Kind: SourceConfigFile,
		Ref:  "the endpoint dialect voice.stt.provider names. The dialect only: the endpoint itself is not reported",
	}
	sourceVoiceProviderUnnamed = Source{
		Kind: SourceConfigFile,
		Ref:  "voice.stt.provider names nothing, so an endpoint is spoken to in the default dialect",
	}
	sourceVoiceProviderSession = Source{
		Kind: SourceSession,
		Ref:  "the dialect from the session's own copy of the configuration",
	}
	sourceVoiceProviderLocal = Source{
		Kind: SourceComputed,
		Ref:  "transcription runs locally or not at all, so no endpoint dialect is spoken",
	}
)

// voiceState is speech input as a whole: switched on or not, what came up on
// this machine, and what it is doing at this moment.
func (s UISurfaceFacts) voiceState(config UIConfigFacts) Field {
	field := s.field(KeyVoiceState, ApplyImmediate, ScopeSession)
	if config.Known && config.Voice.Known {
		field.Configured = Present(BoolValue(config.Voice.Enabled), sourceVoiceEnabled)
	}
	field.Loaded = s.voice(func() Observation {
		return Present(StringValue(s.voiceReadiness()), sourceVoiceReadiness)
	})
	field.Effective = s.voice(func() Observation {
		return Present(StringValue(s.Voice.State), sourceVoiceLive)
	})
	return field
}

// voiceBackend is the transcription path: what was asked for, what resolved
// here, and what a segment would actually take — which is nothing at all when
// no capture resolved to feed it.
func (s UISurfaceFacts) voiceBackend(config UIConfigFacts) Field {
	field := s.field(KeyVoiceBackend, ApplyRestart, ScopeSession)
	switch {
	case !config.Known || !config.Voice.Known:
	case config.Voice.Backend == "":
		field.Configured = Unset(NoValue(), sourceVoiceBackendUnconfigured)
	default:
		field.Configured = Present(StringValue(config.Voice.Backend), sourceVoiceBackendConfigured)
	}
	field.Loaded = s.voice(func() Observation {
		if s.Voice.Backend == "" {
			return Unset(NoValue(), voiceMissingSource(s.Voice.Missing))
		}
		return Present(StringValue(s.Voice.Backend), sourceVoiceBackendResolved)
	})
	field.Effective = s.voice(func() Observation {
		switch {
		case s.Voice.Backend == "":
			return Unset(NoValue(), voiceMissingSource(s.Voice.Missing))
		case !s.Voice.CaptureReady:
			return Unset(NoValue(), sourceVoiceBackendUnfed)
		default:
			return Present(StringValue(s.Voice.Backend), sourceVoiceBackendUsed)
		}
	})
	return field
}

// voiceCapture is the microphone: which arrangement was asked for, whether it
// resolved, and who holds the one recording this process admits.
func (s UISurfaceFacts) voiceCapture(config UIConfigFacts) Field {
	field := s.field(KeyVoiceCapture, ApplyRestart, ScopeSession)
	if config.Known && config.Voice.Known {
		field.Configured = Present(StringValue(string(config.Voice.Capture)), sourceVoiceCaptureConfigured)
	}
	field.Loaded = s.voice(func() Observation {
		return Present(BoolValue(s.Voice.CaptureReady), sourceVoiceCaptureResolved)
	})
	field.Effective = s.voice(func() Observation {
		return Present(StringValue(string(s.captureActivity())), sourceVoiceCaptureLive)
	})
	return field
}

// voiceModel is the local model a segment is transcribed with. It travels as
// a file name: where it was found is a path, and paths do not belong here.
func (s UISurfaceFacts) voiceModel(config UIConfigFacts) Field {
	field := s.field(KeyVoiceModel, ApplyImmediate, ScopeSession)
	switch {
	case !config.Known || !config.Voice.Known:
	case config.Voice.Model == "":
		field.Configured = Unset(NoValue(), sourceVoiceModelUnpinned)
	default:
		field.Configured = Present(StringValue(config.Voice.Model), sourceVoiceModelPinned)
	}
	field.Loaded = s.voice(func() Observation {
		switch {
		case s.Voice.Backend == "":
			return Unset(NoValue(), sourceVoiceModelUnresolved)
		case s.Voice.Remote:
			return notApplicable(sourceVoiceModelRemote)
		case s.Voice.Model == "":
			return notApplicable(sourceVoiceModelSelfContained)
		default:
			return Present(StringValue(s.Voice.Model), sourceVoiceModelResolved)
		}
	})
	field.Effective = field.Loaded
	return field
}

// voiceCredential is whether speech input carries a credential anywhere. It
// is three booleans: one somebody set, one the resolved path would ask for,
// and one the next request would actually carry. No value of any of them ever
// reaches this view.
func (s UISurfaceFacts) voiceCredential(config UIConfigFacts) Field {
	field := s.field(KeyVoiceCredential, ApplyRestart, ScopeSession)
	if config.Known && config.Voice.Known {
		field.Configured = Present(BoolValue(config.Voice.Credential), sourceVoiceKeyConfigured)
	}
	field.Loaded = s.voice(func() Observation {
		return Present(BoolValue(s.Voice.Backend != "" && s.Voice.Remote), sourceVoiceKeyNeeded)
	})
	field.Effective = s.voice(func() Observation {
		if s.Voice.Backend == "" || !s.Voice.Remote {
			return notApplicable(sourceVoiceKeyLocal)
		}
		return Present(BoolValue(s.Voice.Credential), sourceVoiceKeySent)
	})
	return field
}

// voiceLanguage is the language a segment is transcribed in: what the file
// asks for and what the session's own copy asks for.
func (s UISurfaceFacts) voiceLanguage(config UIConfigFacts) Field {
	field := s.field(KeyVoiceLanguage, ApplyRestart, ScopeSession)
	if config.Known && config.Voice.Known {
		field.Configured = voiceLanguage(config.Voice.Tuning, sourceVoiceLanguage)
	}
	field.Loaded = s.voice(func() Observation {
		return voiceLanguage(s.Voice.Tuning, sourceVoiceLanguageSession)
	})
	field.Effective = field.Loaded
	return field
}

// voiceLanguage is one language observation: a name, or the fact that none
// was given and the backend decides.
func voiceLanguage(tuning VoiceTuning, source Source) Observation {
	if tuning.Language == "" {
		return Unset(NoValue(), sourceVoiceLanguageUnset)
	}
	return Present(StringValue(tuning.Language), source)
}

// voiceLimits is the ceilings a segment runs under. They are numbers and
// only numbers; four of them, in one order, so two answers read the same way.
func (s UISurfaceFacts) voiceLimits(config UIConfigFacts) Field {
	field := s.field(KeyVoiceLimits, ApplyRestart, ScopeSession)
	if config.Known && config.Voice.Known {
		field.Configured = Present(ListValue(voiceLimitLabels(config.Voice.Tuning)), sourceVoiceLimits)
	}
	field.Loaded = s.voice(func() Observation {
		return Present(ListValue(voiceLimitLabels(s.Voice.Tuning)), sourceVoiceLimitsSession)
	})
	field.Effective = field.Loaded
	return field
}

// voiceLimitLabels renders the ceilings as "name=value", the way the view's
// own limits are rendered.
func voiceLimitLabels(tuning VoiceTuning) []string {
	return []string{
		"max_seconds=" + strconv.Itoa(tuning.MaxSeconds),
		"segment_silence_ms=" + strconv.Itoa(tuning.SegmentSilenceMS),
		"auto_pause_seconds=" + strconv.Itoa(tuning.AutoPauseSeconds),
		"timeout_seconds=" + strconv.Itoa(tuning.TimeoutSeconds),
	}
}

// voiceHints is the vocabulary hint: the mode somebody chose and how many
// terms they wrote, and whether a hint would actually travel with a segment.
// The terms themselves are the one thing here a person wrote, and they stay
// where they were written.
func (s UISurfaceFacts) voiceHints(config UIConfigFacts) Field {
	field := s.field(KeyVoiceHints, ApplyRestart, ScopeSession)
	if config.Known && config.Voice.Known {
		field.Configured = Present(ListValue(voiceHintLabels(config.Voice.Tuning)), sourceVoiceHints)
	}
	field.Loaded = s.voice(func() Observation {
		return Present(ListValue(voiceHintLabels(s.Voice.Tuning)), sourceVoiceHintsSession)
	})
	field.Effective = s.voice(func() Observation {
		if s.Voice.Backend == "" {
			return Unset(NoValue(), sourceVoiceHintUnresolved)
		}
		return Present(BoolValue(voiceHintTravels(s.Voice.Tuning)), sourceVoiceHintTravels)
	})
	return field
}

// voiceHintLabels renders the hint mode and the glossary as a count.
func voiceHintLabels(tuning VoiceTuning) []string {
	return []string{
		"mode=" + tuning.Hints,
		"glossary_terms=" + strconv.Itoa(tuning.GlossaryTerms),
	}
}

// voiceHintTravels is the owner's rule restated: a hint travels when the mode
// is glossary and there is a glossary to send.
func voiceHintTravels(tuning VoiceTuning) bool {
	return tuning.Hints == voiceHintsGlossary && tuning.GlossaryTerms > 0
}

// voiceHintsGlossary is the one hint mode that sends anything.
const voiceHintsGlossary = "glossary"

// voiceProvider is the dialect an endpoint is spoken to in. It means nothing
// when transcription runs locally, and the endpoint it would apply to is not
// reported either way.
func (s UISurfaceFacts) voiceProvider(config UIConfigFacts) Field {
	field := s.field(KeyVoiceProvider, ApplyRestart, ScopeSession)
	if config.Known && config.Voice.Known {
		field.Configured = voiceProvider(config.Voice.Tuning, sourceVoiceProvider)
	}
	field.Loaded = s.voice(func() Observation {
		return voiceProvider(s.Voice.Tuning, sourceVoiceProviderSession)
	})
	field.Effective = s.voice(func() Observation {
		if s.Voice.Backend == "" || !s.Voice.Remote {
			return notApplicable(sourceVoiceProviderLocal)
		}
		return voiceProvider(s.Voice.Tuning, sourceVoiceProviderSession)
	})
	return field
}

// voiceProvider is one dialect observation: a name, or the fact that none
// was given and the default dialect is spoken.
func voiceProvider(tuning VoiceTuning, source Source) Observation {
	if tuning.Provider == "" {
		return Unset(NoValue(), sourceVoiceProviderUnnamed)
	}
	return Present(StringValue(tuning.Provider), source)
}

// voice is runtime narrowed by one more absence: a surface may render and
// have no voice session attached at all, which is the wiring rather than the
// setting — a session is built even when speech input is switched off.
func (s UISurfaceFacts) voice(observe func() Observation) Observation {
	return s.runtime(func() Observation {
		if !s.Voice.Known {
			return Unset(NoValue(), sourceVoiceUnattached)
		}
		return observe()
	})
}

// VoiceReadiness is what a voice session came up as, once the configuration
// and what resolved on this machine are put together.
type VoiceReadiness string

// VoiceReadiness values.
const (
	// VoiceReadinessOff means speech input is switched off.
	VoiceReadinessOff VoiceReadiness = "off"
	// VoiceReadinessReady means both halves resolved and a segment could be
	// recorded and transcribed.
	VoiceReadinessReady VoiceReadiness = "ready"
	// VoiceReadinessUnresolved means speech input is on but a piece it needs
	// is missing here.
	VoiceReadinessUnresolved VoiceReadiness = "unresolved"
)

// voiceReadiness reports what the session came up as. A session exists even
// when voice is off, so "off" is a state of a real session rather than the
// absence of one.
func (s UISurfaceFacts) voiceReadiness() string {
	switch {
	case !s.Voice.Enabled:
		return string(VoiceReadinessOff)
	case s.Voice.Backend != "" && s.Voice.CaptureReady:
		return string(VoiceReadinessReady)
	default:
		return string(VoiceReadinessUnresolved)
	}
}

// VoiceCaptureActivity is what the one recording this process admits is doing.
type VoiceCaptureActivity string

// VoiceCaptureActivity values.
const (
	// VoiceCaptureIdle means the microphone is not held.
	VoiceCaptureIdle VoiceCaptureActivity = "idle"
	// VoiceCaptureRecording means this session holds it.
	VoiceCaptureRecording VoiceCaptureActivity = "recording"
	// VoiceCaptureBusyElsewhere means the admission is held by another
	// session, so starting here would be refused.
	VoiceCaptureBusyElsewhere VoiceCaptureActivity = "busy_elsewhere"
)

// captureActivity tells this session's own recording apart from another
// session's, which is the difference between a microphone that is busy and
// one that is busy with you.
func (s UISurfaceFacts) captureActivity() VoiceCaptureActivity {
	switch {
	case s.Voice.State == voiceStateListening:
		return VoiceCaptureRecording
	case s.Voice.GateBusy:
		return VoiceCaptureBusyElsewhere
	default:
		return VoiceCaptureIdle
	}
}

// voiceMissingSource maps the closed vocabulary onto a fixed sentence, so the
// reason a backend did not resolve is legible without the owner's hint — that
// one is prose for a person and may name a path.
func voiceMissingSource(missing VoiceMissing) Source {
	switch missing {
	case VoiceMissingBinary:
		return sourceVoiceMissingBinary
	case VoiceMissingModel:
		return sourceVoiceMissingModel
	case VoiceMissingConfiguredModel:
		return sourceVoiceMissingConfiguredModel
	default:
		return sourceVoiceMissingOther
	}
}
