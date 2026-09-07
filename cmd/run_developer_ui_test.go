package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
)

// uiConfigYAML is a configuration somebody has been at: notifications
// narrowed and named, two commands rebound, speech input switched off. None
// of it can render in a headless run, and all of it was still configured.
const uiConfigYAML = `notifications:
  mode: always
  sound: Ping
keybinds:
  palette: F9
  help: F2
voice:
  enabled: false
`

// headlessUI runs one developer-mode round that asks what this run's surface
// is, and returns the fixture, the decoded answer and its raw text. The
// prepare hook runs after the bootstrap and before the run, which is the only
// window in which the stored preferences can be written: the run reads them
// itself, once, on its way in.
func headlessUI(
	t *testing.T,
	prepare func(*developerFixture),
	extraConfig ...string,
) (*developerFixture, map[string]harnessField, string) {
	t.Helper()
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"ui"}`)
		}
		return doneText()
	}, extraConfig...)
	if prepare != nil {
		prepare(fixture)
	}

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what do you render through", maxRounds: 3, timeout: 20 * time.Second, developerMode: true,
	})
	require.Equal(t, ExitOK, exit)

	outputs := fixture.toolOutputs()
	require.Len(t, outputs, 1, "exactly one harness call, so exactly one answer")
	return fixture, harnessFields(t, outputs[0]), outputs[0]
}

// A run started to answer a prompt and exit renders nothing, and says so
// first: every layer a surface would own means something different once that
// is known.
func TestHeadlessSaysItRendersNothingBeforeSayingAnythingAboutASurface(t *testing.T) {
	_, fields, _ := headlessUI(t, nil)

	kind := fields["surface.kind"]
	assert.Equal(t, "none", kind.Loaded.Value.String)
	assert.Equal(t, "none", kind.Effective.Value.String)
	assert.Equal(t, "not_applicable", kind.Configured.State, "nothing configures the shape")

	for _, key := range []string{"theme.name", "theme.builtin", "notify.delivery", "voice.state"} {
		assert.Equal(t, "not_applicable", fields[key].Effective.State, key)
	}
}

// A headless run must not hide a setting somebody made: what was configured
// for a surface is reported by a process that has none, because that is
// exactly where "why did my notification not arrive" gets asked.
func TestHeadlessHidesNoSettingSomebodyMadeForASurfaceItDoesNotHave(t *testing.T) {
	_, fields, _ := headlessUI(t, nil, uiConfigYAML)

	assert.Equal(t, "always", fields["notify.mode"].Configured.Value.String)
	assert.Equal(t, "Ping", fields["notify.sound"].Configured.Value.String)
	assert.Equal(t, []string{"help", "palette"}, fields["keybinds.overrides"].Configured.Value.List)
	assert.False(t, fields["voice.state"].Configured.Value.Bool, "switched off is a setting")
	assert.Contains(t, fields["keybinds.commands"].Loaded.Value.List, "palette",
		"what may be rebound does not stop being true because nothing paints")
}

// The dialect is read from the stored preferences on the way in, and the
// empty string is kept as it was written: "nothing persisted a dialect"
// parses into the same mode an explicit "standard" does, and telling the two
// apart is the whole point of the layer.
func TestHeadlessReportsThePersistedDialectAndTellsItApartFromNone(t *testing.T) {
	_, none, _ := headlessUI(t, nil)
	assert.Equal(t, "unset", none["keymap.mode"].Configured.State)
	assert.Equal(t, "config_file", none["keymap.mode"].Configured.Source.Kind,
		"the file was read; it simply persists no dialect")

	_, stored, _ := headlessUI(t, func(f *developerFixture) {
		require.NoError(t, project.MutateUIState(f.bs.Proj.Global(), func(s *project.UIState) {
			s.EditingMode = "readline"
		}))
	})
	assert.Equal(t, "present", stored["keymap.mode"].Configured.State)
	assert.Equal(t, "readline", stored["keymap.mode"].Configured.Value.String)
	assert.Equal(t, "not_applicable", stored["keymap.mode"].Effective.State,
		"a dialect nothing composes in is not the one anything is editing in")
}

// A run with no bootstrap at all reports absence rather than a surface
// somebody configured to be empty.
func TestUIFactsWithoutABootstrapAreAbsentRatherThanEmpty(t *testing.T) {
	assert.False(t, headlessUIFacts(nil).Known)
	assert.False(t, headlessUIFacts(&runBootstrap{}).Known)
}

// The exclusions hold in the answer the model actually receives: no chord, no
// path, no endpoint and no key is anywhere in the text of it.
func TestTheHeadlessSurfaceAnswerCarriesNoChordNoPathAndNoKey(t *testing.T) {
	fixture, _, answer := headlessUI(t, nil, uiConfigYAML)

	for _, forbidden := range []string{"F9", "Ctrl+", "test-key", fixture.bs.Cwd} {
		assert.NotContains(t, answer, forbidden)
	}

	// Every value the answer carries, flattened: a separator in any of them
	// would be a path, and a path is what this category refuses to carry.
	var decoded struct {
		Categories []struct {
			Fields []harnessField `json:"fields"`
		} `json:"categories"`
	}
	require.NoError(t, json.Unmarshal([]byte(answer), &decoded))
	require.NotEmpty(t, decoded.Categories)
	for _, field := range decoded.Categories[0].Fields {
		for _, layer := range []harnessObservation{field.Configured, field.Loaded, field.Effective} {
			for _, text := range append([]string{layer.Value.String}, layer.Value.List...) {
				assert.False(t, strings.ContainsAny(text, `/\`), "%s carries %q", field.Key, text)
			}
		}
	}
}
