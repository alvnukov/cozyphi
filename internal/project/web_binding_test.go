package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// findModel resolves names against a fixed catalog — the seam the session's
// model list plugs into in production.
func findModel(entries ...llm.ModelConfig) func(string) (llm.ModelConfig, bool) {
	return func(name string) (llm.ModelConfig, bool) {
		for _, e := range entries {
			if e.Name == name {
				return e, true
			}
		}
		return llm.ModelConfig{}, false
	}
}

// TestWebModelPinDecodes: web.model is read from the web section and kept
// verbatim apart from trimming — it is a reference to a models-list entry,
// not a model definition of its own.
func TestWebModelPinDecodes(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, `models:
  - name: reader-4o
    base_url: https://api.example.test/v1
    api_key: secret-key
web:
  enabled: true
  model: " reader-4o "
`)

	require.NoError(t, p.LoadConfig())

	assert.Equal(t, "reader-4o", p.Config().Web.Model)
}

// TestWebBindingStates: the pin resolves against the models list only —
// absent pin, unknown name, and a resolvable entry are three distinct
// verdicts, and a caller that cannot supply the list resolves nothing.
func TestWebBindingStates(t *testing.T) {
	reader := llm.ModelConfig{
		Name:               "reader-4o",
		ProviderID:         "openai",
		BaseURL:            "https://api.example.test/v1",
		APIKey:             "secret-key",
		ConnectionIdentity: "account-fingerprint",
	}

	unset := (WebConfig{}).Binding(findModel(reader))
	assert.Equal(t, WebBindingUnset, unset.State())
	assert.Empty(t, unset.Pin())
	assert.Empty(t, unset.Identity())
	assert.Contains(t, unset.NotReadyReason(), "web.model")

	missing := (WebConfig{Model: "ghost"}).Binding(findModel(reader))
	assert.Equal(t, WebBindingMissing, missing.State())
	assert.Equal(t, "ghost", missing.Pin())
	assert.Contains(t, missing.NotReadyReason(), `"ghost"`)
	assert.Contains(t, missing.NotReadyReason(), "names no configured model")
	assert.Contains(t, missing.NotReadyReason(), "no fallback to the session model")

	resolved := (WebConfig{Model: "reader-4o"}).Binding(findModel(reader))
	require.Equal(t, WebBindingResolved, resolved.State())
	assert.Equal(t, "reader-4o", resolved.Pin())
	assert.Contains(t, resolved.NotReadyReason(), "preflight",
		"a resolved pin is still not readiness — the capability preflight is the missing piece")

	noFinder := (WebConfig{Model: "reader-4o"}).Binding(nil)
	assert.Equal(t, WebBindingMissing, noFinder.State(),
		"a caller without a model catalog resolves nothing")
}

func TestWebBindingWithoutConnectionIdentityFailsClosed(t *testing.T) {
	reader := llm.ModelConfig{
		Name: "reader-4o", ProviderID: "openai", BaseURL: "https://api.example.test/v1", APIKey: "secret-key",
	}

	binding := (WebConfig{Model: reader.Name}).Binding(findModel(reader))

	assert.Equal(t, WebBindingUnidentified, binding.State())
	assert.Contains(t, binding.NotReadyReason(), "stable account or connection identity")
	assert.Empty(t, binding.Fingerprint())
	_, ok := binding.Model()
	assert.False(t, ok, "an unidentified recipient must not reach the transport owner")
}

// TestWebBindingModelCarriesCredentialsOnlyWhenResolved: Model() is the
// transport seam — it hands the full connection config, credentials
// included, to the future quarantined reader, and nothing to anyone else.
func TestWebBindingModelCarriesCredentialsOnlyWhenResolved(t *testing.T) {
	reader := llm.ModelConfig{
		Name: "reader-4o", BaseURL: "https://api.example.test/v1", APIKey: "secret-key",
		ConnectionIdentity: "account-fingerprint",
	}

	_, ok := (WebConfig{}).Binding(findModel(reader)).Model()
	assert.False(t, ok, "unset hands out no config")
	_, ok = (WebConfig{Model: "ghost"}).Binding(findModel(reader)).Model()
	assert.False(t, ok, "a stale pin hands out no config")

	model, ok := (WebConfig{Model: "reader-4o"}).Binding(findModel(reader)).Model()
	require.True(t, ok)
	assert.Equal(t, "secret-key", model.APIKey)
	assert.Equal(t, "https://api.example.test/v1", model.BaseURL)
}

// TestWebBindingDisplayFormsCarryNoSecrets: Identity, Fingerprint and
// NotReadyReason are derived from routing facts alone — a key rotation
// changes none of them, and none of them leak the key.
func TestWebBindingDisplayFormsCarryNoSecrets(t *testing.T) {
	base := llm.ModelConfig{
		Name:               "reader-4o",
		ProviderID:         "openai",
		Protocol:           llm.ProtocolOpenAI,
		BaseURL:            "https://user:password@gateway.example/v1/tenant-secret?token=query-secret",
		APIKey:             "secret-key",
		ConnectionIdentity: "account-fingerprint",
	}
	resolve := func(cfg llm.ModelConfig) WebBinding {
		return (WebConfig{Model: cfg.Name}).Binding(findModel(cfg))
	}

	a := resolve(base)
	identity := a.Identity()
	assert.Contains(t, identity, `provider="openai"`)
	assert.Contains(t, identity, `protocol="openai"`)
	assert.Contains(t, identity, `endpoint="https://gateway.example"`)
	for _, form := range []string{identity, a.Fingerprint(), a.NotReadyReason()} {
		for _, secret := range []string{"secret-key", "user", "password", "tenant-secret", "query-secret"} {
			assert.NotContains(t, form, secret)
		}
	}

	rotated := base
	rotated.APIKey = "rotated-key"
	b := resolve(rotated)
	assert.Equal(t, a.Fingerprint(), b.Fingerprint(),
		"a key rotation is not a route change")
	assert.Equal(t, a.Identity(), b.Identity())
	assert.NotContains(t, b.NotReadyReason(), "rotated-key")
}

// TestWebBindingFingerprintFollowsTheRoute: two distinct pins and any
// change to the effective route — endpoint, wire model, provider — yield
// distinct fingerprints; the same route fingerprints the same way across
// resolutions. The fingerprint is the invalidation key: pending state
// checked against one fingerprint must not ride another.
func TestWebBindingFingerprintFollowsTheRoute(t *testing.T) {
	cheap := llm.ModelConfig{
		Name: "cheap", ProviderID: "openai", BaseURL: "https://a.test/v1", ConnectionIdentity: "account-a",
	}
	reader := llm.ModelConfig{
		Name: "reader", ProviderID: "openai", BaseURL: "https://b.test/v1", ConnectionIdentity: "account-a",
	}

	first := (WebConfig{Model: "cheap"}).Binding(findModel(cheap, reader))
	second := (WebConfig{Model: "reader"}).Binding(findModel(cheap, reader))
	require.Equal(t, WebBindingResolved, first.State())
	require.Equal(t, WebBindingResolved, second.State())
	assert.NotEqual(t, first.Fingerprint(), second.Fingerprint(),
		"two pinned configurations are two distinct bindings")
	assert.Equal(t, first.Fingerprint(), (WebConfig{Model: "cheap"}).Binding(findModel(cheap, reader)).Fingerprint(),
		"the same route fingerprints the same way")

	moved := cheap
	moved.BaseURL = "https://a2.test/v1"
	assert.NotEqual(t, first.Fingerprint(), (WebConfig{Model: "cheap"}).Binding(findModel(moved)).Fingerprint(),
		"a changed endpoint is a new binding")

	rewired := cheap
	rewired.APIName = "gpt-4o-mini"
	assert.NotEqual(t, first.Fingerprint(), (WebConfig{Model: "cheap"}).Binding(findModel(rewired)).Fingerprint(),
		"a changed wire model is a new binding")

	reprovidered := cheap
	reprovidered.ProviderID = "anthropic"
	assert.NotEqual(t, first.Fingerprint(), (WebConfig{Model: "cheap"}).Binding(findModel(reprovidered)).Fingerprint(),
		"a changed provider is a new binding")

	reaccounted := cheap
	reaccounted.ConnectionIdentity = "account-b"
	assert.NotEqual(t, first.Fingerprint(), (WebConfig{Model: "cheap"}).Binding(findModel(reaccounted)).Fingerprint(),
		"a changed recipient is a new binding")

	limited := cheap
	limited.MaxOutputTokens = 4096
	assert.NotEqual(t, first.Fingerprint(), (WebConfig{Model: "cheap"}).Binding(findModel(limited)).Fingerprint(),
		"a changed effective output limit is a new binding")

	unresolved := (WebConfig{Model: "cheap"}).Binding(findModel(reader))
	require.Equal(t, WebBindingMissing, unresolved.State())
	assert.Empty(t, unresolved.Fingerprint(),
		"a pin with no route has nothing to fingerprint")
}

// TestWebBindingUnconfiguredFingerprintEmpty: unset and missing bindings
// have no fingerprint to compare against — invalidation only exists once a
// route did.
func TestWebBindingUnconfiguredFingerprintEmpty(t *testing.T) {
	assert.Empty(t, (WebConfig{}).Binding(nil).Fingerprint())
	assert.Empty(t, (WebConfig{}).Binding(nil).Identity())
	assert.Empty(t, (WebConfig{}).Binding(nil).Pin())
}
