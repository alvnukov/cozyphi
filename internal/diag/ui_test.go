package diag_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// liveUIConfig is what a source asked this surface to be. It is deliberately
// a configuration somebody has been at: a dialect persisted, two commands
// rebound, notifications narrowed to the unfocused case with a sound named,
// and speech input switched on against an endpoint with a key.
func liveUIConfig() diag.UIConfigFacts {
	return diag.UIConfigFacts{
		Known:      true,
		KeymapRead: true,
		Keymap:     "readline",
		Keybinds: diag.KeybindConfigFacts{
			Known:     true,
			Commands:  []string{"palette", "help", "watches"},
			Overrides: []string{"help", "palette"},
		},
		Notifications: diag.NotifyConfigFacts{Known: true, Mode: "unfocused", Sound: "Ping"},
		Voice: diag.VoiceConfigFacts{
			Known: true, Enabled: true, Backend: "auto",
			Capture: diag.VoiceCapturePreset, Model: "large-v3", Credential: true,
		},
	}
}

// liveSurface is a terminal that has been used: the palette was switched, the
// dialect with it, the sender has not failed, the terminal has reported focus
// at least once, and a segment is being recorded right now.
func liveSurface() diag.UISurfaceFacts {
	return diag.UISurfaceFacts{
		Known: true,
		Shape: diag.UIShapeTerminal,
		Theme: diag.ThemeFacts{
			Known:   true,
			Builtin: []string{"opencode", "opencode-light", "Dark", "Darcula", "Pink", "Terminal"},
			Boot:    "opencode",
			Live:    "Pink",
		},
		Keys: diag.KeybindRuntimeFacts{
			Known: true, Profile: "readline", Editing: "readline",
			Accepted: []string{"help", "palette"}, Diverged: []string{"palette"},
		},
		Notifications: diag.NotifierFacts{
			Known: true, Mode: "unfocused", Sound: "Ping", FocusTrusted: true,
		},
		Voice: diag.VoiceRuntimeFacts{
			Known: true, Enabled: true, Backend: "command", Remote: false,
			Model: "ggml-large-v3.bin", Credential: true, CaptureReady: true,
			State: "listening", Pending: 1, GateBusy: true,
		},
		Revision: "tPink.kreadline.nunfocusedfalse.vlistening1",
	}
}

// uiFields asks one arrangement of the two owners what it can say, through
// the registry rather than the collector: the bounding, the sanitizing and
// the ordering are part of what these tests assert.
func uiFields(t *testing.T, config diag.UIConfigFacts, surface diag.UISurfaceFacts) []diag.Field {
	t.Helper()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), diag.NewUICollector(diag.UIDeps{
		Config:  func() diag.UIConfigFacts { return config },
		Surface: func() diag.UISurfaceFacts { return surface },
	}))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryUI)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	require.False(t, snapshot.Truncated, "the category fits the response budget on its own")
	return snapshot.Categories[0].Fields
}

// headlessSurface is what a run that renders nothing publishes: a surface
// that exists as an answer and not as a screen.
func headlessSurface() diag.UISurfaceFacts {
	return diag.UISurfaceFacts{Known: true, Shape: diag.UIShapeNone}
}

// The three layers each answer a different question about the same setting,
// and for a surface they answer it in three different vocabularies: what
// persists a palette, what the surface started under, what is painting now.
func TestTheSurfaceAnswersWhatPersistsWhatStartedAndWhatIsActing(t *testing.T) {
	fields := uiFields(t, liveUIConfig(), liveSurface())

	theme := fieldByKey(t, fields, diag.KeyThemeName)
	assert.Equal(t, diag.StateUnset, theme.Configured.State, "nothing persists a palette")
	assert.Equal(t, "opencode", theme.Loaded.Value.Str)
	assert.Equal(t, "Pink", theme.Effective.Value.Str)

	keymap := fieldByKey(t, fields, diag.KeyKeymapMode)
	assert.Equal(t, "readline", keymap.Configured.Value.Str)
	assert.Equal(t, "readline", keymap.Loaded.Value.Str)
	assert.Equal(t, "readline", keymap.Effective.Value.Str)

	overrides := fieldByKey(t, fields, diag.KeyKeybindsOverrides)
	assert.Equal(t, []string{"help", "palette"}, overrides.Configured.Value.List)
	assert.Equal(t, []string{"help", "palette"}, overrides.Loaded.Value.List)
	assert.Equal(t, []string{"palette"}, overrides.Effective.Value.List,
		"a rebound command can end up answering to the chord the dialect would have given it anyway")
}

// Every field says what it would take for a change to it to matter. A palette
// and a dialect land at once; a rebinding waits for the next process, because
// the table is compiled where the process starts.
func TestEverySurfaceFieldSaysWhatItWouldTakeToChangeIt(t *testing.T) {
	want := map[string]struct {
		apply diag.Apply
		scope diag.Scope
	}{
		diag.KeySurfaceKind:       {diag.ApplyRestart, diag.ScopeProcess},
		diag.KeyThemeName:         {diag.ApplyImmediate, diag.ScopeSession},
		diag.KeyThemeBuiltin:      {diag.ApplyRestart, diag.ScopeProcess},
		diag.KeyKeymapMode:        {diag.ApplyImmediate, diag.ScopeProcess},
		diag.KeyKeybindsCommands:  {diag.ApplyRestart, diag.ScopeProcess},
		diag.KeyKeybindsOverrides: {diag.ApplyRestart, diag.ScopeProcess},
		diag.KeyNotifyMode:        {diag.ApplyImmediate, diag.ScopeSession},
		diag.KeyNotifySound:       {diag.ApplyImmediate, diag.ScopeSession},
		diag.KeyNotifyDelivery:    {diag.ApplyImmediate, diag.ScopeSession},
		diag.KeyNotifyFocus:       {diag.ApplyImmediate, diag.ScopeSession},
		diag.KeyVoiceState:        {diag.ApplyImmediate, diag.ScopeSession},
		diag.KeyVoiceBackend:      {diag.ApplyRestart, diag.ScopeSession},
		diag.KeyVoiceCapture:      {diag.ApplyRestart, diag.ScopeSession},
		diag.KeyVoiceModel:        {diag.ApplyImmediate, diag.ScopeSession},
		diag.KeyVoiceCredential:   {diag.ApplyRestart, diag.ScopeSession},
	}
	fields := uiFields(t, liveUIConfig(), liveSurface())
	require.Len(t, fields, len(want))
	for _, field := range fields {
		expected, ok := want[field.Key]
		require.True(t, ok, "unexpected field %q", field.Key)
		assert.Equal(t, expected.apply, field.Apply, field.Key)
		assert.Equal(t, expected.scope, field.Scope, field.Key)
	}
}

// A run that renders nothing has no palette, no table, no notifier and no
// microphone — and says so as "does not exist here", not as "nobody has
// answered yet". The two are different states and mean different things.
func TestARunThatRendersNothingReportsRuntimeLayersAsNotApplicable(t *testing.T) {
	fields := uiFields(t, liveUIConfig(), headlessSurface())

	kind := fieldByKey(t, fields, diag.KeySurfaceKind)
	assert.Equal(t, string(diag.UIShapeNone), kind.Effective.Value.Str)

	for _, key := range []string{
		diag.KeyThemeName, diag.KeyThemeBuiltin, diag.KeyNotifyDelivery,
		diag.KeyNotifyFocus, diag.KeyVoiceState, diag.KeyVoiceCapture,
	} {
		field := fieldByKey(t, fields, key)
		assert.Equal(t, diag.StateNotApplicable, field.Loaded.State, key)
		assert.Equal(t, diag.StateNotApplicable, field.Effective.State, key)
	}
}

// The point of splitting the two owners: a headless run must not hide a
// setting somebody made. The configured layer of every field is the same
// answer in both shapes, and the vocabulary of rebindable commands is
// answered in both too, because what may be configured does not stop being
// true because nothing is painting.
func TestARunThatRendersNothingHidesNoSettingSomebodyMade(t *testing.T) {
	terminal := uiFields(t, liveUIConfig(), liveSurface())
	headless := uiFields(t, liveUIConfig(), headlessSurface())
	require.Len(t, headless, len(terminal))

	for i := range terminal {
		require.Equal(t, terminal[i].Key, headless[i].Key, "the field order is the same in both shapes")
		if terminal[i].Key == diag.KeySurfaceKind {
			continue
		}
		assert.Equal(t, terminal[i].Configured, headless[i].Configured, terminal[i].Key)
	}

	commands := fieldByKey(t, headless, diag.KeyKeybindsCommands)
	assert.Equal(t, diag.StatePresent, commands.Loaded.State)
	assert.Equal(t, []string{"palette", "help", "watches"}, commands.Loaded.Value.List)
}

// A surface that has published nothing has said nothing, and this view may
// not go and find out on its own: reading a live binding table or a live
// palette from a tool goroutine is the race the whole arrangement exists to
// avoid. It reports that, and it is a different answer from the headless one.
func TestASurfaceThatHasPublishedNothingIsUnavailableRatherThanAbsent(t *testing.T) {
	fields := uiFields(t, liveUIConfig(), diag.UISurfaceFacts{})

	kind := fieldByKey(t, fields, diag.KeySurfaceKind)
	assert.Equal(t, diag.StateUnavailable, kind.Loaded.State)
	assert.Equal(t, diag.StateUnavailable, kind.Effective.State)

	theme := fieldByKey(t, fields, diag.KeyThemeName)
	assert.Equal(t, diag.StateUnavailable, theme.Loaded.State)
	assert.Equal(t, diag.SourceUnknown, theme.Loaded.Source.Kind)

	// The configured layers still answer: nobody had to render for a
	// configuration to have been read.
	keymap := fieldByKey(t, fields, diag.KeyKeymapMode)
	assert.Equal(t, "readline", keymap.Configured.Value.Str)
}

// Wiring that was never done degrades into an honest answer rather than into
// a zero value that would read as a setting somebody switched off.
func TestNilAccessorsDegradeIntoUnavailableRatherThanIntoASurfaceNobodyHas(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), diag.NewUICollector(diag.UIDeps{}))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryUI)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	for _, field := range snapshot.Categories[0].Fields {
		for name, layer := range map[string]diag.Observation{
			"configured": field.Configured, "loaded": field.Loaded, "effective": field.Effective,
		} {
			assert.NotEqual(t, diag.StatePresent, layer.State, "%s %s", field.Key, name)
		}
	}
}

// A notification is not the mode: it is the mode, the terminal's focus and
// the sender's health added up, and that sum is what people actually ask
// about. A sender that already failed leaves notifications off whatever the
// mode still says.
func TestWhetherANotificationWouldArriveAddsUpModeFocusAndTheSendersHealth(t *testing.T) {
	cases := []struct {
		name     string
		notifier diag.NotifierFacts
		delivery string
		mode     string
	}{
		{
			name:     "unfocused mode while the terminal has focus drops it",
			notifier: diag.NotifierFacts{Known: true, Mode: "unfocused", FocusTrusted: true, Focused: true},
			delivery: "suppressed", mode: "unfocused",
		},
		{
			name:     "unfocused mode with the terminal elsewhere delivers",
			notifier: diag.NotifierFacts{Known: true, Mode: "unfocused", FocusTrusted: true},
			delivery: "armed", mode: "unfocused",
		},
		{
			name:     "a terminal that never reports focus keeps notifying",
			notifier: diag.NotifierFacts{Known: true, Mode: "unfocused", Focused: true},
			delivery: "armed", mode: "unfocused",
		},
		{
			name:     "always mode ignores focus",
			notifier: diag.NotifierFacts{Known: true, Mode: "always", FocusTrusted: true, Focused: true},
			delivery: "armed", mode: "always",
		},
		{
			name:     "off is off",
			notifier: diag.NotifierFacts{Known: true, Mode: "off"},
			delivery: "off", mode: "off",
		},
		{
			name:     "a sender that failed leaves the mode saying one thing and the process doing another",
			notifier: diag.NotifierFacts{Known: true, Mode: "always", Broken: true},
			delivery: "broken", mode: "off",
		},
		{
			name:     "no notifier at all is not a mode nobody set",
			notifier: diag.NotifierFacts{},
			delivery: "unattached",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			surface := liveSurface()
			surface.Notifications = tc.notifier
			fields := uiFields(t, liveUIConfig(), surface)

			delivery := fieldByKey(t, fields, diag.KeyNotifyDelivery)
			assert.Equal(t, tc.delivery, delivery.Effective.Value.Str)

			mode := fieldByKey(t, fields, diag.KeyNotifyMode)
			assert.Equal(t, "unfocused", mode.Configured.Value.Str, "the configured mode never moves")
			if tc.notifier.Known {
				assert.Equal(t, tc.mode, mode.Effective.Value.Str)
				return
			}
			assert.Equal(t, diag.StateUnset, mode.Loaded.State)
		})
	}
}

// Silence is a setting. A notification that arrives without a sound was asked
// to, and reporting that as "nothing set it" would be a different answer.
func TestASilentNotificationIsASettingAndNotAMissingValue(t *testing.T) {
	config := liveUIConfig()
	config.Notifications.Sound = ""
	surface := liveSurface()
	surface.Notifications.Sound = ""

	sound := fieldByKey(t, uiFields(t, config, surface), diag.KeyNotifySound)
	assert.Equal(t, diag.StateUnset, sound.Configured.State)
	assert.Equal(t, diag.StateUnset, sound.Effective.State)
	assert.Equal(t, diag.SourceConfigFile, sound.Configured.Source.Kind)
}

// One recording is admitted across the whole process, so a session that is
// not recording can still find the microphone held — by another session on
// screen. That is the case worth telling apart, and the two are not the same
// answer as "idle".
func TestVoiceTellsThisSessionsRecordingApartFromAMicrophoneHeldElsewhere(t *testing.T) {
	cases := []struct {
		name     string
		state    string
		gateBusy bool
		activity string
	}{
		{"recording here", "listening", true, "recording"},
		{"held by another session", "idle", true, "busy_elsewhere"},
		{"nobody is recording", "idle", false, "idle"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			surface := liveSurface()
			surface.Voice.State, surface.Voice.GateBusy = tc.state, tc.gateBusy
			capture := fieldByKey(t, uiFields(t, liveUIConfig(), surface), diag.KeyVoiceCapture)
			assert.Equal(t, tc.activity, capture.Effective.Value.Str)
		})
	}
}

// A transcription path that resolved is not the same as one a segment would
// take: with no capture to feed it, nothing is ever transcribed, and the
// effective layer is where that shows.
func TestATranscriptionPathWithNoCaptureToFeedItTranscribesNothing(t *testing.T) {
	surface := liveSurface()
	surface.Voice.CaptureReady = false

	fields := uiFields(t, liveUIConfig(), surface)
	backend := fieldByKey(t, fields, diag.KeyVoiceBackend)
	assert.Equal(t, "command", backend.Loaded.Value.Str)
	assert.Equal(t, diag.StateUnset, backend.Effective.State)

	state := fieldByKey(t, fields, diag.KeyVoiceState)
	assert.Equal(t, "unresolved", state.Loaded.Value.Str)
}

// The reason a backend did not resolve travels as a closed vocabulary and
// never as the sentence written for a person: that one names the binary it
// looked for and the model it did not find.
func TestAnUnresolvedBackendReportsTheReasonWithoutTheSentenceWrittenForAPerson(t *testing.T) {
	cases := []struct {
		missing diag.VoiceMissing
		ref     string
	}{
		{diag.VoiceMissingBinary, "not on PATH"},
		{diag.VoiceMissingModel, "no model is"},
		{diag.VoiceMissingConfiguredModel, "pins a model nothing here matches"},
		{diag.VoiceMissingOther, "not a missing artifact"},
	}
	for _, tc := range cases {
		t.Run(string(tc.missing), func(t *testing.T) {
			surface := liveSurface()
			surface.Voice.Backend, surface.Voice.Missing = "", tc.missing
			backend := fieldByKey(t, uiFields(t, liveUIConfig(), surface), diag.KeyVoiceBackend)
			assert.Equal(t, diag.StateUnset, backend.Loaded.State)
			assert.Contains(t, backend.Loaded.Source.Ref, tc.ref)
		})
	}
}

// A credential is three booleans and never a value: one somebody set, one the
// resolved path would ask for, one the next request would carry. A local
// backend sends nowhere, so the last one does not exist rather than being
// false.
func TestACredentialIsReportedByPresenceAndOnlyWhereSomethingWouldCarryIt(t *testing.T) {
	local := fieldByKey(t, uiFields(t, liveUIConfig(), liveSurface()), diag.KeyVoiceCredential)
	assert.Equal(t, diag.KindBool, local.Configured.Value.Kind)
	assert.True(t, local.Configured.Value.Bool)
	assert.False(t, local.Loaded.Value.Bool, "a local backend asks for none")
	assert.Equal(t, diag.StateNotApplicable, local.Effective.State)

	surface := liveSurface()
	surface.Voice.Backend, surface.Voice.Remote, surface.Voice.Model = "http", true, ""
	remote := fieldByKey(t, uiFields(t, liveUIConfig(), surface), diag.KeyVoiceCredential)
	assert.True(t, remote.Loaded.Value.Bool)
	assert.Equal(t, diag.StatePresent, remote.Effective.State)
	assert.True(t, remote.Effective.Value.Bool)

	model := fieldByKey(t, uiFields(t, liveUIConfig(), surface), diag.KeyVoiceModel)
	assert.Equal(t, diag.StateNotApplicable, model.Loaded.State, "an endpoint carries its own model")
}

// The exclusions are as much a part of the contract as the keys. A chord, a
// capture or transcription command line, an audio device, an endpoint, a
// notification's title or body and any credential all have to have nowhere to
// go — so no value in the category may carry a path separator, a chord
// spelling or anything that is not one of the words these fields report.
func TestNothingInTheSurfaceCategoryCarriesAChordAPathOrACredential(t *testing.T) {
	fields := uiFields(t, liveUIConfig(), liveSurface())
	require.NotEmpty(t, fields)
	for _, field := range fields {
		for name, layer := range map[string]diag.Observation{
			"configured": field.Configured, "loaded": field.Loaded, "effective": field.Effective,
		} {
			for _, text := range append([]string{layer.Value.Str}, layer.Value.List...) {
				where := field.Key + " " + name
				assert.NotContains(t, text, "/", where)
				assert.NotContains(t, text, "\\", where)
				assert.NotContains(t, text, "+", where)
				assert.NotContains(t, strings.ToLower(text), "ctrl", where)
				assert.NotContains(t, strings.ToLower(text), "alt", where)
				assert.NotContains(t, strings.ToLower(text), "http", where)
			}
		}
	}
}

// The model travels as the file it is, never as the place it was found: a
// path is what the whole category refuses to carry, and the name is what a
// person already sees in /voice status.
func TestTheResolvedModelTravelsAsAFileNameAndNeverAsAPath(t *testing.T) {
	model := fieldByKey(t, uiFields(t, liveUIConfig(), liveSurface()), diag.KeyVoiceModel)
	assert.Equal(t, "large-v3", model.Configured.Value.Str)
	assert.Equal(t, "ggml-large-v3.bin", model.Loaded.Value.Str)
	assert.Equal(t, model.Loaded, model.Effective)
}

// The catalog names what this category answers and what it deliberately
// leaves out, and listing it observes nothing: no surface accessor is
// reached, so a catalog request cannot be what publishes a status.
func TestListingTheSurfaceCategoryObservesNothing(t *testing.T) {
	var reads int
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), diag.NewUICollector(diag.UIDeps{
		Config: func() diag.UIConfigFacts { reads++; return liveUIConfig() },
		Surface: func() diag.UISurfaceFacts {
			reads++
			return liveSurface()
		},
	}))

	var entry diag.CatalogEntry
	for _, candidate := range registry.Catalog().Categories {
		if candidate.Category == diag.CategoryUI {
			entry = candidate
		}
	}
	require.Equal(t, diag.CategoryUI, entry.Category)
	assert.Zero(t, reads, "the catalog is answered from the declared key set alone")
	assert.Len(t, entry.Keys, 15)

	// The exclusions have to survive the response budget: a reason the
	// bounder cuts would keep the subjects and lose the promises.
	assert.Contains(t, entry.Reason, "no chord spelling")
	assert.Contains(t, entry.Reason, "no credential by value")
	assert.Contains(t, entry.Reason, "changes nothing")
	assert.LessOrEqual(t, len(entry.Reason), diag.DefaultLimits().MaxValueBytes)
}
