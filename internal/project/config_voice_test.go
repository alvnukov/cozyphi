package project

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/voice"
)

func TestVoiceSectionDecodesThroughTheVoicePackage(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, `models:
  - name: m
    api_key: k
voice:
  enabled: true
  language: ru
  auto_send: true
  max_seconds: 60
  capture:
    device: ":1"
  stt:
    backend: command
    model: base
`)

	require.NoError(t, p.LoadConfig())

	v := p.Config().Voice
	assert.True(t, v.Enabled)
	assert.Equal(t, "ru", v.Language)
	assert.True(t, v.AutoSend)
	assert.Equal(t, 60, v.MaxSeconds)
	assert.Equal(t, ":1", v.Capture.Device)
	assert.Equal(t, voice.BackendCommand, v.STT.Backend)
}

func TestMissingVoiceSectionKeepsThePackageDefaults(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\n")

	require.NoError(t, p.LoadConfig())

	assert.Equal(t, voice.Defaults(), p.Config().Voice)
}

func TestInvalidVoiceSectionFailsTheLoad(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "voice:\n  max_seconds: 5\n")

	err := p.LoadConfig()

	require.Error(t, err, "an out-of-range max_seconds is a load-time error, not a silent default")
}

// The single config.yaml owner rewrites the file for unrelated edits; the
// voice section is not one of its keys, so it has to survive verbatim.
func TestVoiceSectionSurvivesAConfigRewrite(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, `models:
  - name: m
    api_key: k
voice:
  enabled: true
  language: de
`)

	require.NoError(t, SetDangerouslyAllowAll(p.Global(), true))

	raw, err := os.ReadFile(p.Global().ConfigFile())
	require.NoError(t, err)
	assert.Contains(t, string(raw), "language: de", "the voice section is not the rewriter's to drop")

	require.NoError(t, p.LoadConfig())
	assert.True(t, p.Config().Voice.Enabled)
	assert.Equal(t, "de", p.Config().Voice.Language)
	assert.True(t, p.Config().Permissions.DangerouslyAllowAll)
}
