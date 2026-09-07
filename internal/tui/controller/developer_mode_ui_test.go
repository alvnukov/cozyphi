package controller

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/project"
)

// uiSession stands up a developer session under a configuration somebody has
// been at: notifications narrowed and named, two commands rebound, voice
// switched on. Nothing renders — that is the point, since a surface publishes
// its own half separately.
func uiSession(t *testing.T, prepare ...func(*project.Project)) (*Controller, *project.Project) {
	t.Helper()
	proj, cwd := developerProject(t)
	require.NoError(t, os.WriteFile(proj.Global().ConfigFile(), []byte(uiConfigYAML), 0o600))
	require.NoError(t, proj.LoadConfig())
	for _, step := range prepare {
		step(proj)
	}

	rt, err := NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })
	require.NoError(t, rt.GrantDeveloperMode())
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	c, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	return c, proj
}

const uiConfigYAML = `
models:
  - name: test-model
    api_key: k
notifications:
  mode: unfocused
  sound: Ping
keybinds:
  palette: F9
  help: F2
voice:
  enabled: true
`

// uiField asks one session what it can say about its surface, through the
// same path the harness tool takes.
func uiField(t *testing.T, c *Controller, key string) diag.Field {
	t.Helper()
	explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryUI, key)
	require.NoError(t, err)
	return explained.Field
}

// The configuration half is read where a session is assembled, from the
// owners that decode it, and every subject of the surface answers from it
// before anything has rendered.
func TestASessionReportsWhatWasConfiguredForItsSurfaceBeforeAnythingRenders(t *testing.T) {
	c, _ := uiSession(t)

	assert.Equal(t, "unfocused", uiField(t, c, diag.KeyNotifyMode).Configured.Value.Str)
	assert.Equal(t, "Ping", uiField(t, c, diag.KeyNotifySound).Configured.Value.Str)
	assert.Equal(t, []string{"help", "palette"},
		uiField(t, c, diag.KeyKeybindsOverrides).Configured.Value.List, "names, sorted; no chords")
	assert.True(t, uiField(t, c, diag.KeyVoiceState).Configured.Value.Bool)
	assert.Contains(t, uiField(t, c, diag.KeyKeybindsCommands).Loaded.Value.List, "palette")
}

// A session whose View is still being built has said nothing about itself,
// and this view may not go and read a live binding table or a live palette to
// find out. It reports that rather than an empty surface.
func TestASessionWhoseSurfaceHasNotPublishedReportsSoRatherThanAnEmptyOne(t *testing.T) {
	c, _ := uiSession(t)

	for _, key := range []string{diag.KeySurfaceKind, diag.KeyThemeName, diag.KeyNotifyDelivery} {
		field := uiField(t, c, key)
		assert.Equal(t, diag.StateUnavailable, field.Loaded.State, key)
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, key)
	}
	assert.Equal(t, "unfocused", uiField(t, c, diag.KeyNotifyMode).Configured.Value.Str,
		"what was configured is answered all the same")
}

// The surface hands its account over on the goroutine that owns the widgets,
// and what a later question reads is that account and not the widgets.
func TestWhatTheSurfacePublishesIsWhatTheNextQuestionReads(t *testing.T) {
	c, _ := uiSession(t)
	c.PublishUIStatus(diag.UISurfaceFacts{
		Known: true,
		Shape: diag.UIShapeTerminal,
		Theme: components.ObserveTheme(components.OpencodeTheme(), components.PinkTheme()),
		Keys:  diag.KeybindRuntimeFacts{Known: true, Profile: "standard", Editing: "standard"},
		Notifications: diag.NotifierFacts{
			Known: true, Mode: "unfocused", Sound: "Ping", FocusTrusted: true, Focused: true,
		},
		Revision: "one",
	})

	theme := uiField(t, c, diag.KeyThemeName)
	assert.Equal(t, "opencode", theme.Loaded.Value.Str)
	assert.Equal(t, "Pink", theme.Effective.Value.Str)
	assert.Equal(t, "one", theme.Revision)
	assert.Equal(t, "suppressed", uiField(t, c, diag.KeyNotifyDelivery).Effective.Value.Str,
		"unfocused mode, and the terminal is being looked at")

	c.PublishUIStatus(diag.UISurfaceFacts{
		Known: true, Shape: diag.UIShapeTerminal,
		Theme:    components.ObserveTheme(components.OpencodeTheme(), components.DarkTheme()),
		Revision: "two",
	})
	theme = uiField(t, c, diag.KeyThemeName)
	assert.Equal(t, "Dark", theme.Effective.Value.Str, "a palette switch republishes")
	assert.Equal(t, "two", theme.Revision, "and two states are visibly two")
}

// The whole arrangement exists to keep a tool goroutine off the widgets: the
// surface publishes while questions are being answered, and neither side
// touches what the other owns. Run with -race, this is the assertion.
func TestPublishingAndObservingRunConcurrentlyWithoutSharingState(t *testing.T) {
	c, _ := uiSession(t)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 200 {
			live := components.PinkTheme()
			if i%2 == 0 {
				live = components.DarkTheme()
			}
			c.PublishUIStatus(diag.UISurfaceFacts{
				Known: true,
				Shape: diag.UIShapeTerminal,
				Theme: components.ObserveTheme(components.OpencodeTheme(), live),
				Keys: diag.KeybindRuntimeFacts{
					Known: true, Profile: "standard", Accepted: []string{"palette"},
				},
				Voice: diag.VoiceRuntimeFacts{Known: true, Enabled: true, State: "listening", Pending: i},
			})
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryUI)
			require.NoError(t, err)
			require.Len(t, snapshot.Categories, 1)
		}
	}()
	wg.Wait()

	assert.NotEmpty(t, uiField(t, c, diag.KeyThemeName).Effective.Value.Str)
}

// A process without the capability has no registry to publish into, and
// handing an account over is then a no-op rather than a panic — the View
// calls it at eight lifecycle points and does not know which kind of process
// it is in.
func TestPublishingIntoASessionWithoutTheCapabilityIsANoOp(t *testing.T) {
	rt, ws := developerRuntime(t, false)
	c, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	require.Nil(t, c.diagnostics)

	c.PublishUIStatus(diag.UISurfaceFacts{Known: true, Shape: diag.UIShapeTerminal})
	assert.False(t, c.uiSurfaceState().Known, "nothing was stored, because nothing could read it")

	var absent *Controller
	absent.PublishUIStatus(diag.UISurfaceFacts{Known: true})
	assert.False(t, absent.uiSurfaceState().Known)
}

// The dialect is read from the stored preferences once, where the session is
// built, and the empty string is kept as it was written: "nothing persisted a
// dialect" parses into the same mode an explicit "standard" does, and telling
// the two apart is the whole point of the layer.
func TestThePersistedDialectIsToldApartFromNobodyHavingPersistedOne(t *testing.T) {
	unstored, _ := uiSession(t)
	field := uiField(t, unstored, diag.KeyKeymapMode)
	assert.Equal(t, diag.StateUnset, field.Configured.State)
	assert.Equal(t, diag.SourceConfigFile, field.Configured.Source.Kind,
		"the file was read; it simply persists no dialect")

	stored, _ := uiSession(t, func(p *project.Project) {
		require.NoError(t, project.MutateUIState(p.Global(), func(s *project.UIState) {
			s.EditingMode = "readline"
		}))
	})
	field = uiField(t, stored, diag.KeyKeymapMode)
	assert.Equal(t, diag.StatePresent, field.Configured.State)
	assert.Equal(t, "readline", field.Configured.Value.Str)
}

// Answering must never be what reads the preferences off disk: the dialect is
// captured once, and a hundred questions later the file has still been opened
// exactly as many times as sessions were built.
func TestAnsweringNeverRereadsTheStoredPreferences(t *testing.T) {
	c, proj := uiSession(t)
	require.NoError(t, project.MutateUIState(proj.Global(), func(s *project.UIState) {
		s.EditingMode = "vim"
	}))
	require.NoError(t, os.Remove(proj.Global().UIStateFile()))

	for range 5 {
		field := uiField(t, c, diag.KeyKeymapMode)
		assert.Equal(t, diag.StateUnset, field.Configured.State,
			"a file that has since been deleted cannot change an answer that never re-reads it")
	}
}
