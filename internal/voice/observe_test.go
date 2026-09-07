package voice

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// The configuration half is an allowlist: what somebody switched on, which
// backend they named, whether capture is the preset or their own command, the
// model they pinned, and whether a key is there — never the command line, the
// device, the endpoint or the key itself.
func TestTheConfiguredVoiceTravelsWithoutACommandLineADeviceOrAKey(t *testing.T) {
	cfg := Defaults()
	cfg.Enabled = true
	cfg.Capture.Command = "sox -d -r 16000 /tmp/rec.wav"
	cfg.Capture.Device = "MacBook Pro Microphone"
	cfg.STT.Backend = BackendHTTP
	cfg.STT.BaseURL = "https://api.example.com/v1/audio/transcriptions"
	cfg.STT.Model = "whisper-1"
	cfg.STT.apiKey = "sk-not-a-real-key"

	facts := ObserveConfig(cfg)
	require.True(t, facts.Known)
	assert.True(t, facts.Enabled)
	assert.Equal(t, "http", facts.Backend)
	assert.Equal(t, diag.VoiceCaptureCommand, facts.Capture)
	assert.Equal(t, "whisper-1", facts.Model)
	assert.True(t, facts.Credential, "presence, and only presence")

	for _, secret := range []string{"sk-not-a-real-key", "sox", "MacBook", "api.example.com"} {
		assert.NotContains(t, factsText(facts), secret)
	}
}

// The preset is what almost everybody runs, and it is reported as the preset
// rather than as the command line it expands into on this platform.
func TestThePresetCaptureIsReportedAsThePresetAndNotAsItsCommandLine(t *testing.T) {
	cfg := Defaults()
	require.Equal(t, AutoCommand, cfg.Capture.Command, "the default is the preset")
	assert.Equal(t, diag.VoiceCapturePreset, ObserveConfig(cfg).Capture)

	cfg.Capture.Command = ""
	assert.Equal(t, diag.VoiceCapturePreset, ObserveConfig(cfg).Capture, "unset is the preset too")
}

// A key that is not there is false and not absent: "no key configured" is an
// answer somebody debugging a 401 needs.
func TestAVoiceConfigWithNoKeySaysSo(t *testing.T) {
	facts := ObserveConfig(Defaults())
	assert.False(t, facts.Credential)
	assert.True(t, facts.Enabled, "voice is on by default; whether it resolved is another question")

	off := Defaults()
	off.Enabled = false
	assert.False(t, ObserveConfig(off).Enabled)
}

// The runtime half is what resolved on this machine. The model travels as the
// file it is, never as the directory it was found in, because a path is what
// this whole category refuses to carry.
func TestAResolvedModelTravelsAsAFileNameAndNotAsWhereItWasFound(t *testing.T) {
	session := NewSession(Options{
		Config: Defaults(),
		Resolved: Resolved{
			Capture: ResolvedCapture{Argv: []string{"sox", "-d"}},
			STT: ResolvedSTT{
				Backend:   BackendCommand,
				Command:   "whisper-cli -m {model} -f {input}",
				ModelPath: "/Users/somebody/.cozyphi/models/ggml-large-v3.bin",
			},
		},
	}, nil)

	facts := Observe(session, nil)
	require.True(t, facts.Known)
	assert.Equal(t, "ggml-large-v3.bin", facts.Model)
	assert.NotContains(t, facts.Model, "/")
	assert.Equal(t, "command", facts.Backend)
	assert.True(t, facts.CaptureReady)
	assert.False(t, facts.Remote, "a local binary sends nowhere")
	assert.Equal(t, "idle", facts.State)
}

// An endpoint carries its own model, so there is no file to name — and the
// endpoint itself does not travel either.
func TestARemoteBackendReportsNoModelFileAndNoEndpoint(t *testing.T) {
	cfg := Defaults()
	cfg.STT.apiKey = "sk-not-a-real-key"
	session := NewSession(Options{
		Config: cfg,
		Resolved: Resolved{
			Capture: ResolvedCapture{Argv: []string{"sox", "-d"}},
			STT:     ResolvedSTT{Backend: BackendHTTP},
		},
	}, nil)

	facts := Observe(session, nil)
	assert.True(t, facts.Remote)
	assert.Empty(t, facts.Model)
	assert.True(t, facts.Credential)
}

// The reason a backend did not resolve travels as its machine-readable half.
// The hint is a sentence written for a person and names the binary it looked
// for and the model it did not find, so it stays where it was written.
func TestAnUnresolvedBackendReportsWhatIsMissingAndNotTheHint(t *testing.T) {
	cases := []struct {
		missing Missing
		want    diag.VoiceMissing
	}{
		{MissingBinary, diag.VoiceMissingBinary},
		{MissingModel, diag.VoiceMissingModel},
		{MissingConfiguredModel, diag.VoiceMissingConfiguredModel},
		{MissingNone, diag.VoiceMissingOther},
	}
	for _, tc := range cases {
		session := NewSession(Options{
			Config: Defaults(),
			Resolved: Resolved{STT: ResolvedSTT{
				Missing: tc.missing,
				Hint:    "install whisper-cli: brew install whisper-cpp, then /voice model",
			}},
		}, nil)

		facts := Observe(session, nil)
		assert.Equal(t, tc.want, facts.Missing)
		assert.Empty(t, facts.Backend)
		assert.NotContains(t, factsText(facts), "brew")
	}
}

// A backend that did resolve is missing nothing, whatever a stale hint on the
// resolution still says.
func TestAResolvedBackendIsMissingNothing(t *testing.T) {
	session := NewSession(Options{
		Config:   Defaults(),
		Resolved: Resolved{STT: ResolvedSTT{Backend: BackendCommand, Missing: MissingModel}},
	}, nil)
	assert.Equal(t, diag.VoiceMissingNone, Observe(session, nil).Missing)
}

// One recording is admitted across the process, so a session that is not
// recording can still find the microphone held — by another session on
// screen. Both halves are reported, and telling them apart is the point.
func TestTheGateReportsAMicrophoneHeldByAnotherSession(t *testing.T) {
	gate := NewCaptureGate()
	assert.False(t, gate.Busy(), "an idle gate holds nothing")

	capture := gate.Wrap(newStubCapture())
	stream, err := capture.Start(t.Context(), "device")
	require.NoError(t, err)
	assert.True(t, gate.Busy(), "somebody is recording")

	quiet := NewSession(Options{Config: Defaults()}, nil)
	facts := Observe(quiet, gate)
	assert.Equal(t, "idle", facts.State, "not this session")
	assert.True(t, facts.GateBusy, "but the microphone is not free either")

	_, err = stream.Stop()
	require.NoError(t, err)
	assert.False(t, gate.Busy())
	assert.False(t, Observe(quiet, gate).GateBusy)
}

// A session nobody built is absence and not a session that is switched off:
// voice is built even when it is off, and a headless run has neither.
func TestASessionNobodyBuiltIsAbsenceAndNotOneSwitchedOff(t *testing.T) {
	facts := Observe(nil, NewCaptureGate())
	assert.False(t, facts.Known)
	assert.False(t, facts.Enabled)

	cfg := Defaults()
	cfg.Enabled = false
	off := Observe(NewSession(Options{Config: cfg}, nil), nil)
	assert.True(t, off.Known, "a session that exists answers")
	assert.False(t, off.Enabled, "and a session is built even when voice is switched off")
}

// Asking must never be what records: the capture is not started, the gate is
// not taken, and nothing is transcribed.
func TestObservingStartsNoCaptureAndTakesNoAdmission(t *testing.T) {
	gate := NewCaptureGate()
	capture := newStubCapture()
	transcriber := &fakeTranscriber{}
	session := NewSession(Options{
		Config:      Defaults(),
		Capture:     gate.Wrap(capture),
		Transcriber: transcriber,
		Resolved: Resolved{
			Capture: ResolvedCapture{Argv: []string{"sox", "-d"}},
			STT:     ResolvedSTT{Backend: BackendCommand, ModelPath: "/models/ggml-base.bin"},
		},
	}, nil)

	for range 5 {
		require.True(t, Observe(session, gate).Known)
	}
	assert.Zero(t, capture.started(), "no microphone was opened")
	assert.False(t, gate.Busy(), "no admission was taken")
	assert.Equal(t, StateIdle, session.State())
}

// factsText flattens everything a projection carries into one string, so a
// test can ask whether a secret is anywhere in it at all.
func factsText(facts any) string {
	var out []string
	switch f := facts.(type) {
	case diag.VoiceConfigFacts:
		out = []string{f.Backend, string(f.Capture), f.Model}
	case diag.VoiceRuntimeFacts:
		out = []string{f.Backend, string(f.Missing), f.Model, f.State}
	}
	return strings.Join(out, "\x00")
}
