package voice

import (
	"path/filepath"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// ObserveConfig projects the voice section into the shape the diagnostics
// registry reports. It is an allowlist by construction: the capture command
// line, the audio device, the transcription command line and the endpoint
// have nowhere to go, and the API key is answered as a bool — the value lives
// in an unexported field and is never read out of it, not even to be masked.
//
// It observes and returns. It resolves nothing, looks up no binary and opens
// no device.
func ObserveConfig(cfg Config) diag.VoiceConfigFacts {
	capture := diag.VoiceCaptureCommand
	if cfg.Capture.Command == "" || cfg.Capture.Command == AutoCommand {
		capture = diag.VoiceCapturePreset
	}
	return diag.VoiceConfigFacts{
		Known:      true,
		Enabled:    cfg.Enabled,
		Backend:    string(cfg.STT.Backend),
		Capture:    capture,
		Model:      cfg.STT.Model,
		Credential: cfg.STT.HasAPIKey(),
		Tuning:     observeTuning(cfg),
	}
}

// observeTuning projects the settings that shape a recording without naming
// anything a person wrote: the language, the ceilings, the hint mode, the
// provider name and how many glossary terms there are. The terms themselves
// are words somebody chose for their own domain and never travel.
func observeTuning(cfg Config) diag.VoiceTuning {
	return diag.VoiceTuning{
		Language:         cfg.Language,
		MaxSeconds:       cfg.MaxSeconds,
		SegmentSilenceMS: cfg.SegmentSilenceMS,
		AutoPauseSeconds: cfg.AutoPauseSeconds,
		TimeoutSeconds:   cfg.STT.TimeoutSeconds,
		Hints:            string(cfg.Hints),
		GlossaryTerms:    len(cfg.Glossary),
		Provider:         cfg.STT.Provider,
	}
}

// Observe projects a live session and the microphone admission it shares into
// the same shape: what resolved on this machine and what is being done with
// it. A nil session reports absence rather than a session that is switched
// off — the two are different, because one is built even when voice is off.
//
// The resolved model travels as a file name, never as the path it was found
// at, and the hint that explains an unresolved backend to a person does not
// travel at all: its machine-readable half does.
//
// Every accessor it reads is mutex-guarded or atomic, so it is safe from any
// goroutine. It observes and returns: it starts no capture, transcribes
// nothing, reconfigures nothing and takes no admission from the gate.
func Observe(session *Session, gate *CaptureGate) diag.VoiceRuntimeFacts {
	if session == nil {
		return diag.VoiceRuntimeFacts{}
	}
	cfg := session.Config()
	resolved := session.Resolved()
	facts := diag.VoiceRuntimeFacts{
		Known:        true,
		Enabled:      cfg.Enabled,
		Backend:      string(resolved.STT.Backend),
		Missing:      observeMissing(resolved.STT),
		Remote:       resolved.STT.Backend == BackendHTTP,
		Credential:   cfg.STT.HasAPIKey(),
		CaptureReady: len(resolved.Capture.Argv) > 0,
		State:        session.State().String(),
		Pending:      session.Pending(),
		GateBusy:     gate.Busy(),
		Tuning:       observeTuning(cfg),
	}
	if resolved.STT.ModelPath != "" {
		facts.Model = filepath.Base(resolved.STT.ModelPath)
	}
	return facts
}

// observeMissing maps the reason a backend did not resolve onto the closed
// vocabulary the registry reports. A resolved backend is missing nothing, and
// a failure that is not a missing artifact — a malformed command line, say —
// is reported as such rather than as the sentence that explains it.
func observeMissing(stt ResolvedSTT) diag.VoiceMissing {
	if stt.Backend != "" {
		return diag.VoiceMissingNone
	}
	switch stt.Missing {
	case MissingBinary:
		return diag.VoiceMissingBinary
	case MissingModel:
		return diag.VoiceMissingModel
	case MissingConfiguredModel:
		return diag.VoiceMissingConfiguredModel
	case MissingNone:
		return diag.VoiceMissingOther
	default:
		return diag.VoiceMissingOther
	}
}
