package voice

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeConfigAppliesDefaultsForAnAbsentSection(t *testing.T) {
	cfg, err := DecodeConfig(FileConfig{})
	require.NoError(t, err)

	assert.True(t, cfg.Enabled)
	assert.Equal(t, DefaultLanguage, cfg.Language)
	assert.False(t, cfg.AutoSend)
	assert.Equal(t, DefaultMaxSeconds, cfg.MaxSeconds)
	assert.Equal(t, HintsGlossary, cfg.Hints)
	assert.Equal(t, defaultGlossary, cfg.Glossary)
	assert.Equal(t, AutoCommand, cfg.Capture.Command)
	assert.Equal(t, DefaultDevice, cfg.Capture.Device)
	assert.Equal(t, BackendAuto, cfg.STT.Backend)
	assert.Equal(t, DefaultSTTCommand, cfg.STT.Command)
	assert.Equal(t, DefaultTimeoutSeconds, cfg.STT.TimeoutSeconds)
}

func TestDecodeConfigDistinguishesAbsentFromFalse(t *testing.T) {
	cfg, err := DecodeConfig(FileConfig{Enabled: new(false)})
	require.NoError(t, err)
	assert.False(t, cfg.Enabled, "enabled: false is honored, not treated as absent")
}

func TestDecodeConfigRejectsUnknownEnumValues(t *testing.T) {
	tests := map[string]struct {
		raw  FileConfig
		want string
	}{
		"hints": {FileConfig{Hints: new("session")}, "voice.hints must be glossary or off"},
		"backend": {
			FileConfig{STT: &STTFileConfig{Backend: new("whisper")}},
			"voice.stt.backend must be auto, command or http",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeConfig(tc.raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestDecodeConfigBoundsMaxSeconds(t *testing.T) {
	for _, seconds := range []int{9, 1801, 0, -1} {
		_, err := DecodeConfig(FileConfig{MaxSeconds: new(seconds)})
		require.Error(t, err, "max_seconds %d is out of range", seconds)
		assert.Contains(t, err.Error(), "voice.max_seconds must be between 10 and 1800")
	}
	for _, seconds := range []int{10, 300, 1800} {
		cfg, err := DecodeConfig(FileConfig{MaxSeconds: new(seconds)})
		require.NoError(t, err)
		assert.Equal(t, seconds, cfg.MaxSeconds)
	}
}

func TestDecodeConfigRejectsProviderCredentialReuse(t *testing.T) {
	_, err := DecodeConfig(FileConfig{STT: &STTFileConfig{Provider: new("openai")}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "voice.stt.provider is not supported yet")
}

func TestConfigStringNeverPrintsTheAPIKey(t *testing.T) {
	cfg, err := DecodeConfig(FileConfig{STT: &STTFileConfig{
		BaseURL: new("https://api.example.com/v1"),
		APIKey:  new("sk-super-secret-value"),
	}})
	require.NoError(t, err)

	assert.NotContains(t, cfg.String(), "sk-super-secret-value")
	assert.NotContains(t, cfg.STT.String(), "sk-super-secret-value")
	assert.True(t, cfg.STT.HasAPIKey())
}

func TestConfigHintJoinsTheGlossaryAndHonorsOff(t *testing.T) {
	cfg, err := DecodeConfig(FileConfig{Glossary: []string{"alpha", " beta ", ""}})
	require.NoError(t, err)
	assert.Equal(t, "alpha, beta", cfg.Hint())

	off, err := DecodeConfig(FileConfig{Hints: new("off")})
	require.NoError(t, err)
	assert.Empty(t, off.Hint())
}

// binSet builds a LookBin that only knows the named binaries.
func binSet(names ...string) func(string) (string, error) {
	known := make(map[string]bool, len(names))
	for _, n := range names {
		known[n] = true
	}
	return func(name string) (string, error) {
		if known[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
}

func TestResolveBuildsTheDarwinCapturePreset(t *testing.T) {
	cfg := Defaults()
	got := Resolve(cfg, ResolveEnv{GOOS: "darwin", LookBin: binSet("ffmpeg")})

	assert.Equal(t, []string{
		"/usr/bin/ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "avfoundation", "-i", ":default",
		"-ac", "1", "-ar", "16000", "-f", "s16le", "-",
	}, got.Capture.Argv)
}

func TestResolveUsesPulseWhenPactlIsPresent(t *testing.T) {
	cfg := Defaults()
	withPulse := Resolve(cfg, ResolveEnv{GOOS: "linux", LookBin: binSet("ffmpeg", "pactl")})
	assert.Contains(t, withPulse.Capture.Argv, "pulse")

	withoutPulse := Resolve(cfg, ResolveEnv{GOOS: "linux", LookBin: binSet("ffmpeg")})
	assert.Contains(t, withoutPulse.Capture.Argv, "alsa")
}

func TestResolveHasNoCapturePresetOnWindows(t *testing.T) {
	got := Resolve(Defaults(), ResolveEnv{GOOS: "windows", LookBin: binSet("ffmpeg")})
	assert.Empty(t, got.Capture.Argv)
	assert.Contains(t, got.Capture.Hint, "set voice.capture.command")
	assert.False(t, got.Ready())
}

func TestResolveExpandsTheDeviceInACustomCaptureCommand(t *testing.T) {
	cfg := Defaults()
	cfg.Capture.Command = `rec -q -t raw {device}`
	cfg.Capture.Device = "hw:1,0"

	got := Resolve(cfg, ResolveEnv{GOOS: "linux", LookBin: binSet()})
	assert.Equal(t, []string{"rec", "-q", "-t", "raw", "hw:1,0"}, got.Capture.Argv)
}

func TestResolveAutoPrefersTheLocalCommandWhenAModelExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ggml-base.bin"), []byte("model"), 0o600))

	got := Resolve(Defaults(), ResolveEnv{
		GOOS:      "darwin",
		LookBin:   binSet("ffmpeg", "whisper-cli"),
		ModelsDir: dir,
	})
	assert.Equal(t, BackendCommand, got.STT.Backend)
	assert.Equal(t, filepath.Join(dir, "ggml-base.bin"), got.STT.ModelPath)
	assert.True(t, got.Ready())
	assert.Empty(t, got.Hint())
}

func TestResolveAutoFallsBackToHTTPWhenTheModelIsMissing(t *testing.T) {
	cfg, err := DecodeConfig(FileConfig{STT: &STTFileConfig{
		BaseURL: new("https://api.example.com/v1"),
		APIKey:  new("secret"),
	}})
	require.NoError(t, err)

	got := Resolve(cfg, ResolveEnv{GOOS: "darwin", LookBin: binSet("ffmpeg"), ModelsDir: t.TempDir()})
	assert.Equal(t, BackendHTTP, got.STT.Backend)
	assert.True(t, got.Ready())
}

func TestResolveReportsTheMissingTranscriber(t *testing.T) {
	got := Resolve(Defaults(), ResolveEnv{GOOS: "darwin", LookBin: binSet("ffmpeg"), ModelsDir: t.TempDir()})
	assert.Empty(t, got.STT.Backend)
	assert.Equal(t,
		"no transcriber configured — install whisper-cpp and a ggml model, or set voice.stt.base_url and api_key",
		got.Hint())
}

func TestResolveExplicitHTTPExplainsWhatIsMissing(t *testing.T) {
	cfg := Defaults()
	cfg.STT.Backend = BackendHTTP
	got := Resolve(cfg, ResolveEnv{GOOS: "darwin", LookBin: binSet("ffmpeg")})
	assert.Contains(t, got.STT.Hint, "base_url is empty")

	cfg.STT.BaseURL = "https://api.example.com/v1"
	got = Resolve(cfg, ResolveEnv{GOOS: "darwin", LookBin: binSet("ffmpeg")})
	assert.Contains(t, got.STT.Hint, "api_key is empty")
}

func TestResolveHonorsAnExplicitModelPath(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "custom.bin")
	require.NoError(t, os.WriteFile(model, []byte("model"), 0o600))

	cfg := Defaults()
	cfg.STT.Model = model
	got := Resolve(cfg, ResolveEnv{GOOS: "darwin", LookBin: binSet("ffmpeg", "whisper-cli")})
	assert.Equal(t, model, got.STT.ModelPath)
}
