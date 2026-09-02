package project

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// TestLoadConfigWarnsWhenProtocolIsSniffed: a model that never declares a
// protocol still works, but the guess is surfaced as a warning instead of
// silently switching an OpenAI-compatible gateway to the Anthropic wire
// format.
func TestLoadConfigWarnsWhenProtocolIsSniffed(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: claude-3-5-sonnet
    api_key: sk-test
    base_url: https://llm.corp/v1
`), 0o600))

	require.NoError(t, p.LoadConfig())
	assert.Equal(t, llm.ProtocolAnthropic, p.Config().Model().Protocol)
	warnings := p.Config().Warnings()
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "claude-3-5-sonnet")
	assert.Contains(t, warnings[0], "protocol")
}

// TestLoadConfigExplicitProtocolBeatsSniffing: a declared protocol decides,
// even when both the model name and the URL look anthropic-ish.
func TestLoadConfigExplicitProtocolBeatsSniffing(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: claude-3-5-sonnet
    api_key: sk-test
    base_url: https://llm.corp/v1
    protocol: openai
`), 0o600))

	require.NoError(t, p.LoadConfig())
	assert.Equal(t, llm.ProtocolOpenAI, p.Config().Model().Protocol)
	assert.Empty(t, p.Config().Warnings())
}

// The warning must not send the user to a key that cannot help: `provider`
// is metadata, so an entry that sets it keeps the guess and the warning.
func TestSniffWarningNamesOnlyTheProtocolKey(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: claude-3-5-sonnet
    api_key: sk-test
    base_url: https://llm.corp/v1
    provider: anthropic
`), 0o600))

	require.NoError(t, p.LoadConfig())
	warnings := p.Config().Warnings()
	require.Len(t, warnings, 1, "an explicit provider does not settle the protocol")
	assert.Contains(t, warnings[0], "set protocol explicitly")
	assert.NotContains(t, warnings[0], "provider")
	assert.Equal(t, "anthropic", p.Config().Model().ProviderID, "the key is still carried through")
}
