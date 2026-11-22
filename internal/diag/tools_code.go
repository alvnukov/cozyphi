package diag

import (
	"time"

	"github.com/alvnukov/cozyphi/internal/redact"
)

// CodeModeSettings describes one layer of the program runtime. An empty advisor
// in the effective layer means discovery cannot make an auxiliary model call.
type CodeModeSettings struct {
	Known          bool
	Enabled        bool
	Discovery      string
	AdvisorModel   string
	AdvisorTimeout time.Duration
}

func (s ToolState) codeModeFields() []Field {
	cfg, live := s.CodeConfig, s.CodeMode
	source := Source{Kind: SourceSession, Ref: "the program runtime bound to this session"}
	fields := []Field{
		{Key: KeyCodeMode, Configured: layerObservation(cfg.Known, true, BoolValue(cfg.Enabled),
			Source{Kind: SourceConfigFile, Ref: KeyCodeMode})},
		{Key: KeyCodeDiscovery, Configured: codeSetting(cfg.Known, cfg.Discovery, KeyCodeDiscovery)},
		{Key: KeyCodeAdvisor, Configured: codeSetting(cfg.Known, cfg.AdvisorModel, KeyCodeAdvisor)},
		{Key: KeyCodeTimeout, Configured: codeSetting(cfg.Known, timeoutLabel(cfg.AdvisorTimeout), KeyCodeTimeout)},
	}
	modeSource := source
	if s.CodeFlag == "--code-mode" || s.CodeFlag == "--direct-tools" {
		modeSource = Source{Kind: SourceCLIFlag, Ref: s.CodeFlag}
	}
	fields[0].Loaded = layerObservation(live.Known, true, BoolValue(live.Enabled), modeSource)
	for i, value := range []string{live.Discovery, live.AdvisorModel, timeoutLabel(live.AdvisorTimeout)} {
		f := &fields[i+1]
		f.Loaded = layerObservation(live.Known, value != "", StringValue(redact.Redact(value)), source)
		if live.Known && !live.Enabled {
			f.Loaded = NotApplicable(SourceSession)
		}
	}
	for i := range fields {
		f := &fields[i]
		f.Effective = f.Loaded
		f.Scope, f.Apply, f.Revision = ScopeSession, ApplyRestart, s.Revision
	}
	return fields
}

func codeSetting(known bool, value, ref string) Observation {
	return layerObservation(
		known,
		value != "",
		StringValue(redact.Redact(value)),
		Source{Kind: SourceConfigFile, Ref: ref},
	)
}

func timeoutLabel(timeout time.Duration) string {
	if timeout <= 0 {
		return ""
	}
	return timeout.String()
}
