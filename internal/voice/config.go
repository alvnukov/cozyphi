// Package voice turns speech into composer text. It captures microphone audio
// through an external command (ffmpeg by default), transcribes it either with
// a local command (whisper-cpp) or over an OpenAI-compatible HTTP endpoint,
// and reports the result through a callback. Nothing here imports the TUI, and
// the API key never leaves the transcriber that needs it.
package voice

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Backend names the transcription path.
type Backend string

// The transcription backends. "auto" resolves to one of the other two at load
// time; an unresolvable "auto" leaves voice unconfigured with a hint.
const (
	BackendAuto    Backend = "auto"
	BackendCommand Backend = "command"
	BackendHTTP    Backend = "http"
)

// HintMode selects the vocabulary hint sent along with a transcription.
type HintMode string

// The hint modes. Phase 2 adds "session" (recent transcript words).
const (
	HintsGlossary HintMode = "glossary"
	HintsOff      HintMode = "off"
)

// Defaults and bounds for the voice section.
const (
	// SampleRate is the capture and WAV rate every backend expects.
	SampleRate = 16000
	// DefaultMaxSeconds is the longest single segment: audio is cut here even
	// when the speaker never pauses.
	DefaultMaxSeconds = 30
	// MinMaxSeconds and MaxMaxSeconds bound voice.max_seconds.
	MinMaxSeconds = 5
	MaxMaxSeconds = 120
	// DefaultSegmentSilenceMS is the trailing silence that closes a segment.
	DefaultSegmentSilenceMS = 800
	// MinSegmentSilenceMS and MaxSegmentSilenceMS bound voice.segment_silence_ms.
	MinSegmentSilenceMS = 200
	MaxSegmentSilenceMS = 5000
	// DefaultAutoPauseSeconds is how long the microphone may hear nothing
	// before the dialog mode pauses itself.
	DefaultAutoPauseSeconds = 300
	// MinAutoPauseSeconds and MaxAutoPauseSeconds bound voice.auto_pause_seconds.
	MinAutoPauseSeconds = 30
	MaxAutoPauseSeconds = 3600
	// DefaultTimeoutSeconds bounds one transcription request.
	DefaultTimeoutSeconds = 60
	// MaxTimeoutSeconds bounds voice.stt.timeout_seconds.
	MaxTimeoutSeconds = 3600
	// DefaultLanguage passes language detection to the backend.
	DefaultLanguage = "auto"
	// DefaultDevice is the preset's "whatever the system picked" device.
	DefaultDevice = "default"
	// AutoCommand is the "pick a preset for this OS" spelling.
	AutoCommand = "auto"
	// DefaultSTTCommand is the whisper-cpp command line used when no other is set.
	DefaultSTTCommand = "whisper-cli -m {model} -l {lang} --prompt {hint} -nt -f {file}"
	// modelPlaceholder is the command template slot the model path fills; a
	// template without it needs no model at all.
	modelPlaceholder = "{model}"
)

// defaultGlossary seeds the vocabulary hint with the project's own jargon,
// which is exactly what a speaker dictating into cozyphi tends to say.
var defaultGlossary = []string{"cozyphi", "worktree", "goreleaser", "xui"}

// Config is the decoded voice section. It is safe to print: String hides the
// API key, which lives in an unexported field and never rides on a Msg.
type Config struct {
	Enabled  bool
	Language string
	// MaxSeconds is the longest single segment.
	MaxSeconds int
	// SegmentSilenceMS is the trailing silence that closes a segment.
	SegmentSilenceMS int
	// AutoPauseSeconds is the continuous silence that pauses the mode.
	AutoPauseSeconds int
	Hints            HintMode
	Glossary         []string
	Capture          CaptureConfig
	STT              STTConfig
}

// CaptureConfig describes how microphone audio reaches us.
type CaptureConfig struct {
	// Command is "auto" (an ffmpeg preset for this OS) or a command line that
	// writes s16le 16 kHz mono PCM to stdout.
	Command string
	// Device is the preset's device name or index; a custom command receives
	// it through the {device} placeholder.
	Device string
}

// STTConfig describes how audio becomes text.
type STTConfig struct {
	Backend        Backend
	Command        string
	Model          string
	BaseURL        string
	Provider       string
	TimeoutSeconds int

	// apiKey is deliberately unexported: it must never appear in a Msg, a
	// toast, a log line or a %v of this struct.
	apiKey string
}

// String renders the capture settings for a log line.
func (c CaptureConfig) String() string {
	return "capture{command:" + c.Command + " device:" + c.Device + "}"
}

// String renders the STT settings without the API key.
func (c STTConfig) String() string {
	return fmt.Sprintf("stt{backend:%s command:%q model:%q base_url:%q key:%t timeout:%ds}",
		c.Backend, c.Command, c.Model, c.BaseURL, c.apiKey != "", c.TimeoutSeconds)
}

// HasAPIKey reports whether an HTTP credential is configured.
func (c STTConfig) HasAPIKey() bool { return c.apiKey != "" }

// String renders the whole section without the API key, so a %v of a Config
// can never leak a credential.
func (c Config) String() string {
	return fmt.Sprintf(
		"voice{enabled:%t language:%s max_seconds:%d segment_silence_ms:%d auto_pause_seconds:%d "+
			"hints:%s glossary:%d %s %s}",
		c.Enabled, c.Language, c.MaxSeconds, c.SegmentSilenceMS, c.AutoPauseSeconds,
		c.Hints, len(c.Glossary), c.Capture, c.STT)
}

// Hint returns the vocabulary hint for a transcription request.
func (c Config) Hint() string {
	if c.Hints != HintsGlossary || len(c.Glossary) == 0 {
		return ""
	}
	return strings.Join(c.Glossary, ", ")
}

// Defaults returns the configuration used when the section is absent.
func Defaults() Config {
	return Config{
		Enabled:          true,
		Language:         DefaultLanguage,
		MaxSeconds:       DefaultMaxSeconds,
		SegmentSilenceMS: DefaultSegmentSilenceMS,
		AutoPauseSeconds: DefaultAutoPauseSeconds,
		Hints:            HintsGlossary,
		Glossary:         append([]string(nil), defaultGlossary...),
		Capture:          CaptureConfig{Command: AutoCommand, Device: DefaultDevice},
		STT: STTConfig{
			Backend:        BackendAuto,
			Command:        DefaultSTTCommand,
			TimeoutSeconds: DefaultTimeoutSeconds,
		},
	}
}

// FileConfig mirrors the YAML shape. Pointer fields distinguish an absent key
// from a zero value, so "enabled: false" and "no voice section" differ.
type FileConfig struct {
	Enabled  *bool   `yaml:"enabled"`
	Language *string `yaml:"language"`
	// AutoSend is gone; the field stays so a stale key is rejected with a
	// sentence instead of being ignored by the loader.
	AutoSend         *bool              `yaml:"auto_send"`
	MaxSeconds       *int               `yaml:"max_seconds"`
	SegmentSilenceMS *int               `yaml:"segment_silence_ms"`
	AutoPauseSeconds *int               `yaml:"auto_pause_seconds"`
	Hints            *string            `yaml:"hints"`
	Glossary         []string           `yaml:"glossary"`
	Capture          *CaptureFileConfig `yaml:"capture"`
	STT              *STTFileConfig     `yaml:"stt"`
}

// CaptureFileConfig mirrors voice.capture.
type CaptureFileConfig struct {
	Command *string `yaml:"command"`
	Device  *string `yaml:"device"`
}

// STTFileConfig mirrors voice.stt.
type STTFileConfig struct {
	Backend        *string `yaml:"backend"`
	Command        *string `yaml:"command"`
	Model          *string `yaml:"model"`
	BaseURL        *string `yaml:"base_url"`
	APIKey         *string `yaml:"api_key"`
	Provider       *string `yaml:"provider"`
	TimeoutSeconds *int    `yaml:"timeout_seconds"`
}

// DecodeConfig turns the YAML mirror into a Config, applying defaults and
// rejecting unknown enum values and out-of-range numbers. It touches no
// filesystem and no PATH: that is Resolve's job.
func DecodeConfig(raw FileConfig) (Config, error) {
	cfg := Defaults()
	if raw.Enabled != nil {
		cfg.Enabled = *raw.Enabled
	}
	if raw.Language != nil {
		lang := strings.TrimSpace(*raw.Language)
		if lang == "" {
			return Config{}, errors.New("voice.language must not be empty (ru, en or auto)")
		}
		cfg.Language = lang
	}
	if raw.AutoSend != nil {
		return Config{}, errors.New("voice.auto_send was removed; the dialog mode sends on Enter")
	}
	if raw.MaxSeconds != nil {
		if *raw.MaxSeconds < MinMaxSeconds || *raw.MaxSeconds > MaxMaxSeconds {
			return Config{}, fmt.Errorf("voice.max_seconds must be between %d and %d, got %d",
				MinMaxSeconds, MaxMaxSeconds, *raw.MaxSeconds)
		}
		cfg.MaxSeconds = *raw.MaxSeconds
	}
	if raw.SegmentSilenceMS != nil {
		if *raw.SegmentSilenceMS < MinSegmentSilenceMS || *raw.SegmentSilenceMS > MaxSegmentSilenceMS {
			return Config{}, fmt.Errorf("voice.segment_silence_ms must be between %d and %d, got %d",
				MinSegmentSilenceMS, MaxSegmentSilenceMS, *raw.SegmentSilenceMS)
		}
		cfg.SegmentSilenceMS = *raw.SegmentSilenceMS
	}
	if raw.AutoPauseSeconds != nil {
		if *raw.AutoPauseSeconds < MinAutoPauseSeconds || *raw.AutoPauseSeconds > MaxAutoPauseSeconds {
			return Config{}, fmt.Errorf("voice.auto_pause_seconds must be between %d and %d, got %d",
				MinAutoPauseSeconds, MaxAutoPauseSeconds, *raw.AutoPauseSeconds)
		}
		cfg.AutoPauseSeconds = *raw.AutoPauseSeconds
	}
	if raw.Hints != nil {
		mode := HintMode(strings.TrimSpace(*raw.Hints))
		if mode != HintsGlossary && mode != HintsOff {
			return Config{}, fmt.Errorf("voice.hints must be glossary or off, got %q", *raw.Hints)
		}
		cfg.Hints = mode
	}
	if raw.Glossary != nil {
		cfg.Glossary = cleanGlossary(raw.Glossary)
	}
	if err := decodeCapture(&cfg, raw.Capture); err != nil {
		return Config{}, err
	}
	if err := decodeSTT(&cfg, raw.STT); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func decodeCapture(cfg *Config, raw *CaptureFileConfig) error {
	if raw == nil {
		return nil
	}
	if raw.Command != nil {
		command := strings.TrimSpace(*raw.Command)
		if command == "" {
			return errors.New("voice.capture.command must not be empty (auto, or a command line)")
		}
		cfg.Capture.Command = command
	}
	if raw.Device != nil {
		device := strings.TrimSpace(*raw.Device)
		if device == "" {
			return errors.New("voice.capture.device must not be empty (default, or a device name)")
		}
		cfg.Capture.Device = device
	}
	return nil
}

func decodeSTT(cfg *Config, raw *STTFileConfig) error {
	if raw == nil {
		return nil
	}
	if raw.Backend != nil {
		backend := Backend(strings.TrimSpace(*raw.Backend))
		if backend != BackendAuto && backend != BackendCommand && backend != BackendHTTP {
			return fmt.Errorf("voice.stt.backend must be auto, command or http, got %q", *raw.Backend)
		}
		cfg.STT.Backend = backend
	}
	if raw.Command != nil {
		command := strings.TrimSpace(*raw.Command)
		if command == "" {
			return errors.New("voice.stt.command must not be empty")
		}
		cfg.STT.Command = command
	}
	if raw.Model != nil {
		cfg.STT.Model = strings.TrimSpace(*raw.Model)
	}
	if raw.BaseURL != nil {
		cfg.STT.BaseURL = strings.TrimRight(strings.TrimSpace(*raw.BaseURL), "/")
	}
	if raw.APIKey != nil {
		cfg.STT.apiKey = strings.TrimSpace(*raw.APIKey)
	}
	if raw.Provider != nil && strings.TrimSpace(*raw.Provider) != "" {
		cfg.STT.Provider = strings.TrimSpace(*raw.Provider)
		return errors.New(
			"voice.stt.provider is not supported yet: set voice.stt.base_url and voice.stt.api_key instead")
	}
	if raw.TimeoutSeconds != nil {
		if *raw.TimeoutSeconds < 1 || *raw.TimeoutSeconds > MaxTimeoutSeconds {
			return fmt.Errorf("voice.stt.timeout_seconds must be between 1 and %d, got %d",
				MaxTimeoutSeconds, *raw.TimeoutSeconds)
		}
		cfg.STT.TimeoutSeconds = *raw.TimeoutSeconds
	}
	return nil
}

func cleanGlossary(in []string) []string {
	out := make([]string, 0, len(in))
	for _, word := range in {
		if w := strings.TrimSpace(word); w != "" {
			out = append(out, w)
		}
	}
	return out
}

// ResolveEnv is everything Resolve needs from the outside world: the OS it is
// resolving for, the project's binary lookup, and where models may live. A
// test fills it in and never touches the real PATH.
type ResolveEnv struct {
	GOOS      string
	LookBin   func(name string) (string, error)
	ModelsDir string
	// ExtraModelDirs are searched after ModelsDir; the packaged whisper-cpp
	// model directories go here.
	ExtraModelDirs []string
}

// Resolved is the filesystem- and PATH-dependent half of the configuration:
// the exact capture argv and the backend that will actually run.
type Resolved struct {
	Capture ResolvedCapture
	STT     ResolvedSTT
}

// ResolvedCapture is the capture command, or the reason there is none.
type ResolvedCapture struct {
	// Argv is the ready-to-run command; nil when capture is unconfigured.
	Argv []string
	// Hint is the one-line reason and next action when Argv is nil.
	Hint string
}

// Missing names what a resolved backend lacks, so the UI can offer to fix it
// instead of only printing the hint.
type Missing int

// The reasons transcription did not resolve.
const (
	// MissingNone means nothing is missing, or the hint is not about a
	// missing artifact (a malformed command line, say).
	MissingNone Missing = iota
	// MissingBinary means the transcription binary is not on PATH.
	MissingBinary
	// MissingModel means the binary is there but no ggml model is installed.
	// This is the one case Ctrl+G answers with an offer to download one.
	MissingModel
	// MissingConfiguredModel means voice.stt.model is set and nothing matches
	// it; another model is never silently used instead.
	MissingConfiguredModel
)

// ResolvedSTT is the chosen transcription backend, or the reason there is none.
type ResolvedSTT struct {
	// Backend is command or http; empty when transcription is unconfigured.
	Backend Backend
	// Command is the command template for BackendCommand.
	Command string
	// ModelPath is the resolved model file for BackendCommand.
	ModelPath string
	// Hint is the one-line reason and next action when Backend is empty.
	Hint string
	// Missing is the machine-readable half of Hint.
	Missing Missing
}

// Ready reports whether both halves resolved, i.e. voice can actually record
// and transcribe.
func (r Resolved) Ready() bool { return len(r.Capture.Argv) > 0 && r.STT.Backend != "" }

// Hint returns the first unconfigured half's hint, or "".
func (r Resolved) Hint() string {
	if r.Capture.Hint != "" {
		return r.Capture.Hint
	}
	return r.STT.Hint
}

// Resolve picks the capture command and the transcription backend for this
// machine. It never fails: an unresolvable half carries a Hint that names the
// missing piece and what to do about it.
func Resolve(cfg Config, env ResolveEnv) Resolved {
	if env.GOOS == "" {
		env.GOOS = runtime.GOOS
	}
	if env.LookBin == nil {
		env.LookBin = func(string) (string, error) { return "", errors.New("no binary lookup") }
	}
	return Resolved{
		Capture: resolveCapture(cfg, env),
		STT:     resolveSTT(cfg, env),
	}
}

func resolveCapture(cfg Config, env ResolveEnv) ResolvedCapture {
	if cfg.Capture.Command != AutoCommand {
		argv, err := splitArgs(cfg.Capture.Command)
		if err != nil || len(argv) == 0 {
			return ResolvedCapture{Hint: "voice.capture.command is not a valid command line"}
		}
		return ResolvedCapture{Argv: expandArgs(argv, map[string]string{"device": cfg.Capture.Device})}
	}
	if env.GOOS == "windows" {
		return ResolvedCapture{
			Hint: "voice capture has no preset on Windows — set voice.capture.command",
		}
	}
	ffmpeg, err := env.LookBin("ffmpeg")
	if err != nil {
		return ResolvedCapture{
			Hint: "no capture command found — install ffmpeg or set voice.capture.command",
		}
	}
	return ResolvedCapture{Argv: capturePreset(ffmpeg, env, cfg.Capture.Device)}
}

// capturePreset builds the ffmpeg argv for this OS. Everything after the input
// is identical: one channel, 16 kHz, raw signed 16-bit little-endian on stdout.
func capturePreset(ffmpeg string, env ResolveEnv, device string) []string {
	argv := []string{ffmpeg, "-hide_banner", "-loglevel", "error"}
	switch env.GOOS {
	case "darwin":
		argv = append(argv, "-f", "avfoundation", "-i", ":"+device)
	default:
		if _, err := env.LookBin("pactl"); err == nil {
			argv = append(argv, "-f", "pulse", "-i", device)
		} else {
			argv = append(argv, "-f", "alsa", "-i", device)
		}
	}
	return append(argv, "-ac", "1", "-ar", "16000", "-f", "s16le", "-")
}

// httpHint is the same sentence in the two places that need it.
const httpHint = "set voice.stt.base_url and api_key"

func resolveSTT(cfg Config, env ResolveEnv) ResolvedSTT {
	switch cfg.STT.Backend {
	case BackendCommand:
		return resolveCommandSTT(cfg, env, true)
	case BackendHTTP:
		return resolveHTTPSTT(cfg, true)
	default:
		local := resolveCommandSTT(cfg, env, false)
		if local.Backend != "" {
			return local
		}
		// A configured endpoint wins over a local setup that did not resolve;
		// otherwise the local reason is the one worth showing, because it is
		// the one the user can act on.
		if remote := resolveHTTPSTT(cfg, false); remote.Backend != "" {
			return remote
		}
		return local
	}
}

// resolveCommandSTT resolves the local command backend. explicit turns "this
// is not available" into a hint the user sees; under auto the caller falls
// through to HTTP instead.
func resolveCommandSTT(cfg Config, env ResolveEnv, explicit bool) ResolvedSTT {
	argv, err := splitArgs(cfg.STT.Command)
	if err != nil || len(argv) == 0 {
		return ResolvedSTT{Hint: "voice.stt.command is not a valid command line"}
	}
	if _, err := env.LookBin(argv[0]); err != nil {
		if explicit {
			return ResolvedSTT{
				Missing: MissingBinary,
				Hint:    "voice.stt.command needs " + argv[0] + " on PATH — install whisper-cpp",
			}
		}
		return ResolvedSTT{
			Missing: MissingBinary,
			Hint:    argv[0] + " not found — brew install whisper-cpp, or " + httpHint,
		}
	}
	// A template that never substitutes a model does not need one: it may be
	// a wrapper that knows its own weights.
	if !strings.Contains(cfg.STT.Command, modelPlaceholder) {
		return ResolvedSTT{Backend: BackendCommand, Command: cfg.STT.Command}
	}
	model, missing := findModel(cfg.STT.Model, env)
	if model == "" {
		return ResolvedSTT{Missing: missing, Hint: missingModelHint(missing, cfg.STT.Model)}
	}
	return ResolvedSTT{Backend: BackendCommand, Command: cfg.STT.Command, ModelPath: model}
}

// missingModelHint is the one line the user gets when the command backend has
// no weights to load.
func missingModelHint(missing Missing, configured string) string {
	if missing == MissingConfiguredModel {
		return "voice.stt.model not found: " + configured + " — /voice install, or fix the path"
	}
	def, _ := LookupModel(DefaultModel)
	return "no speech model installed — /voice install downloads " + modelFilePrefix + def.Name +
		" (~" + FormatBytes(def.ApproxBytes) + ")"
}

func resolveHTTPSTT(cfg Config, explicit bool) ResolvedSTT {
	switch {
	// In auto mode a keyless endpoint is taken at face value, because a local
	// whisper-server wants no key; explicit http still says what is missing.
	case cfg.STT.BaseURL != "" && (cfg.STT.apiKey != "" || !explicit):
		return ResolvedSTT{Backend: BackendHTTP}
	case !explicit:
		return ResolvedSTT{}
	case cfg.STT.BaseURL == "":
		return ResolvedSTT{Hint: "voice.stt.backend is http but base_url is empty — " + httpHint}
	default:
		return ResolvedSTT{Hint: "voice.stt.backend is http but api_key is empty — " + httpHint}
	}
}

// ModelDirs lists where models are looked up, best first: the cozyphi models
// directory, then whatever a packaged whisper-cpp brought.
func ModelDirs(env ResolveEnv) []string {
	dirs := make([]string, 0, 1+len(env.ExtraModelDirs))
	if env.ModelsDir != "" {
		dirs = append(dirs, env.ModelsDir)
	}
	return append(dirs, env.ExtraModelDirs...)
}

// findModel returns the model file to pass to the command backend and, when
// there is none, why: a pinned voice.stt.model that matches nothing is a
// different problem from having no model at all.
func findModel(configured string, env ResolveEnv) (string, Missing) {
	dirs := ModelDirs(env)
	if configured != "" {
		if path := lookupConfiguredModel(configured, dirs); path != "" {
			return path, MissingNone
		}
		return "", MissingConfiguredModel
	}
	if path := bestInstalledModel(dirs); path != "" {
		return path, MissingNone
	}
	return "", MissingModel
}

// lookupConfiguredModel resolves voice.stt.model in its three spellings: a
// path, a catalog name, or a file name in one of the model directories.
func lookupConfiguredModel(configured string, dirs []string) string {
	if strings.ContainsRune(configured, '/') || strings.ContainsRune(configured, os.PathSeparator) {
		if isFile(configured) {
			return configured
		}
		return ""
	}
	if strings.HasSuffix(configured, modelFileSuffix) && isFile(configured) {
		return configured
	}
	names := []string{configured}
	if m, ok := LookupModel(configured); ok {
		names = append(names, m.File)
	}
	for _, dir := range dirs {
		for _, name := range names {
			if path := filepath.Join(dir, name); isFile(path) {
				return path
			}
		}
	}
	return ""
}

// bestInstalledModel picks the model to use when nothing is pinned: the
// highest catalog rank, then the earlier directory, then the exact catalog
// file name over a quantized variant, then alphabetical.
func bestInstalledModel(dirs []string) string {
	type candidate struct {
		path  string
		rank  int
		dir   int
		exact bool
	}
	best := candidate{rank: -2, dir: -1}
	for i, dir := range dirs {
		for _, ins := range InstalledModels([]string{dir}) {
			cur := candidate{
				path:  ins.Path,
				rank:  ins.Rank,
				dir:   i,
				exact: ins.Name != "" && filepath.Base(ins.Path) == ModelFileName(ins.Name),
			}
			switch {
			case cur.rank != best.rank:
				if cur.rank > best.rank {
					best = cur
				}
			case best.path == "" || (cur.dir == best.dir && cur.exact && !best.exact):
				best = cur
			}
		}
	}
	return best.path
}

// isFile reports whether path is an existing regular file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// DefaultModelDirs lists the directories a packaged whisper-cpp keeps models
// in. They are searched after ~/.cozyphi/models.
func DefaultModelDirs() []string {
	return []string{
		"/opt/homebrew/share/whisper-cpp",
		"/usr/local/share/whisper-cpp",
		"/usr/share/whisper-cpp",
	}
}
