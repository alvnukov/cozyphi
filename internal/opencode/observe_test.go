package opencode

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/provider"
)

func TestTheImportTellsItsFourOutcomesApart(t *testing.T) {
	t.Parallel()
	loaded := &Source{models: make([]llm.ModelConfig, 3)}
	cases := []struct {
		name    string
		enabled bool
		source  *Source
		loadErr error
		want    diag.ImportFacts
	}{{
		name:    "off",
		enabled: false,
		source:  loaded,
		loadErr: errors.New("neither of these happened, because the import never ran"),
		want:    diag.ImportFacts{Known: true, State: diag.ImportDisabled},
	}, {
		name:    "never reached",
		enabled: true,
		want:    diag.ImportFacts{Known: true, State: diag.ImportNotLoaded},
	}, {
		name:    "failed",
		enabled: true,
		loadErr: errors.New("read opencode.json"),
		want:    diag.ImportFacts{Known: true, State: diag.ImportFailed},
	}, {
		name:    "loaded",
		enabled: true,
		source:  loaded,
		want:    diag.ImportFacts{Known: true, State: diag.ImportLoaded, Models: 3},
	}}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, item.want, ImportObservation(item.enabled, item.source, item.loadErr))
		})
	}
}

func TestAnImportThatIsOffSaysSoEvenWhenItRanBefore(t *testing.T) {
	t.Parallel()
	// The setting is the first question asked, so a source left over from an
	// earlier decision cannot make a disabled import look like a live one.
	facts := ImportObservation(false, &Source{models: make([]llm.ModelConfig, 9)}, nil)

	assert.Equal(t, diag.ImportDisabled, facts.State)
	assert.Zero(t, facts.Models, "a count belongs to an import that happened")
}

func TestAFailedImportReportsTheFailureAndNoneOfItsText(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	authPath := filepath.Join(dir, "auth.json")
	// Both the path and the file's own bytes are things the error may name,
	// and neither may survive into an observation.
	require.NoError(t, os.WriteFile(configPath, []byte(`{"provider": "config-body-sentinel"`), 0o600))
	require.NoError(t, os.WriteFile(authPath,
		[]byte(`{"openai":{"type":"api","key":"auth-key-sentinel"}}`), 0o600))

	_, err := Load(Options{ConfigPath: configPath, AuthPath: authPath})
	require.Error(t, err)

	facts := ImportObservation(true, nil, err)

	assert.Equal(t, diag.ImportFacts{Known: true, State: diag.ImportFailed}, facts)
	rendered := fmt.Sprintf("%#v", facts)
	for _, secret := range []string{"config-body-sentinel", "auth-key-sentinel", dir, "opencode.json"} {
		assert.NotContains(t, rendered, secret, "a failure says that it failed, and nothing about what it read")
	}
}

func TestALoadedImportCountsItsModelsAndNamesNoneOfThem(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	authPath := filepath.Join(dir, "auth.json")
	require.NoError(t, os.WriteFile(authPath,
		[]byte(`{"openai":{"type":"api","key":"import-key-sentinel"}}`), 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte(`{"provider": {}}`), 0o600))

	source, err := Load(Options{
		ConfigPath: configPath, AuthPath: authPath,
		Catalog: []provider.Info{{
			ID: "openai", BaseURL: "https://api.openai.com/v1", Protocol: llm.ProtocolOpenAI,
			Models: []provider.Model{
				{ID: "model-name-sentinel", ContextWindow: 128000, MaxOutputTokens: 8192},
				{ID: "second-model-sentinel", ContextWindow: 128000, MaxOutputTokens: 8192},
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, source.Models(), 2)

	facts := ImportObservation(true, source, nil)

	assert.Equal(t, diag.ImportFacts{Known: true, State: diag.ImportLoaded, Models: 2}, facts)
	rendered := fmt.Sprintf("%#v", facts)
	for _, secret := range []string{"import-key-sentinel", "model-name-sentinel", "second-model-sentinel"} {
		assert.NotContains(t, rendered, secret, "how many models were imported, never which")
	}
}
