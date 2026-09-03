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
	"sort"
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
		if got := resolveCommandSTT(cfg, env, false); got.Backend != "" {
			return got
		}
		if got := resolveHTTPSTT(cfg, false); got.Backend != "" {
			return got
		}
		return ResolvedSTT{
			Hint: "no transcriber configured — install whisper-cpp and a ggml model, or " + httpHint,
		}
	}
}

// resolveCommandSTT resolves the local command backend. explicit turns "this
// is not available" into a hint the user sees; under auto the caller falls
// through to HTTP instead.
func resolveCommandSTT(cfg Config, env ResolveEnv, explicit bool) ResolvedSTT {
	argv, err := splitArgs(cfg.STT.Command)
	if err != nil || len(argv) == 0 {
		if explicit {
			return ResolvedSTT{Hint: "voice.stt.command is not a valid command line"}
		}
		return ResolvedSTT{}
	}
	if _, err := env.LookBin(argv[0]); err != nil {
		if explicit {
			return ResolvedSTT{Hint: "voice.stt.command needs " + argv[0] + " on PATH — install whisper-cpp"}
		}
		return ResolvedSTT{}
	}
	model := findModel(cfg.STT.Model, env)
	if model == "" {
		if explicit {
			return ResolvedSTT{Hint: "no speech model found — set voice.stt.model to a ggml-*.bin file"}
		}
		return ResolvedSTT{}
	}
	return ResolvedSTT{Backend: BackendCommand, Command: cfg.STT.Command, ModelPath: model}
}

func resolveHTTPSTT(cfg Config, explicit bool) ResolvedSTT {
	switch {
	case cfg.STT.BaseURL != "" && cfg.STT.apiKey != "":
		return ResolvedSTT{Backend: BackendHTTP}
	case !explicit:
		return ResolvedSTT{}
	case cfg.STT.BaseURL == "":
		return ResolvedSTT{Hint: "voice.stt.backend is http but base_url is empty — " + httpHint}
	default:
		return ResolvedSTT{Hint: "voice.stt.backend is http but api_key is empty — " + httpHint}
	}
}

// findModel returns the model file to pass to the command backend: the
// configured path when it exists, else the first ggml-*.bin in the model
// directories, in order.
func findModel(configured string, env ResolveEnv) string {
	if configured != "" {
		if _, err := os.Stat(configured); err == nil {
			return configured
		}
		return ""
	}
	dirs := make([]string, 0, 1+len(env.ExtraModelDirs))
	if env.ModelsDir != "" {
		dirs = append(dirs, env.ModelsDir)
	}
	dirs = append(dirs, env.ExtraModelDirs...)
	for _, dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, "ggml-*.bin"))
		if err != nil || len(matches) == 0 {
			continue
		}
		sort.Strings(matches)
		return matches[0]
	}
	return ""
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
