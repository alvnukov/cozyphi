package provider_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/provider"
)

// The sentinels below are everything a credential record can hold. Not one of
// them may appear in an observation, in any spelling: not whole, not
// truncated to a suffix, not hashed.
const (
	sentinelKey     = "sk-observe-key-sentinel"
	sentinelUser    = "userinfo-sentinel"
	sentinelQuery   = "query-token-sentinel"
	sentinelAccount = "account-id-sentinel"
	sentinelAccess  = "access-token-sentinel"
	sentinelRefresh = "refresh-token-sentinel"
)

// secretEndpoint is a base URL that carries a credential in every place a URL
// can hide one: the userinfo and the query.
var secretEndpoint = "https://" + sentinelUser + ":" + sentinelKey +
	"@acme.invalid/v1?token=" + sentinelQuery

// refusingClient fails the test on any request. It is what proves an
// observation reaches no network: a catalog refresh, a token refresh and a
// quota probe all go through this client, and none of them may run.
func refusingClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: refusingTransport{t: t}}
}

type refusingTransport struct{ t *testing.T }

func (r refusingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.t.Errorf("observation reached the network: %s %s", req.Method, req.URL.Host)
	return nil, fmt.Errorf("network is not allowed here")
}

// connectedProviders is what a signed-in user's store holds: an API key for
// one provider and an OAuth session for another. Both shapes are here because
// they hide credentials in different fields, and an observation has to stay
// silent about all of them.
var connectedProviders = []string{"acme", "openai"}

// connectedManager opens a manager over a saved catalog and a credential file
// stuffed with sentinels, exactly as a signed-in user's would be.
func connectedManager(t *testing.T) (*provider.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "providers.json")
	credentialsPath := filepath.Join(dir, "credentials.json")
	require.NoError(t, os.WriteFile(cachePath,
		[]byte(validatedCacheJSON("acme", "Acme", secretEndpoint, "openai", "acme-1")), 0o600))
	credentials := fmt.Sprintf(`{"version":1,"providers":{
		"acme":{"type":"api","key":%q,"base_url":%q,"protocol":"openai"},
		"openai":{"type":"oauth","access":%q,"refresh":%q,"account_id":%q,"expires":%d,
			"base_url":"https://chatgpt.com/backend-api/codex","protocol":"openai-responses"}}}`,
		sentinelKey, secretEndpoint,
		sentinelAccess, sentinelRefresh, sentinelAccount,
		time.Now().Add(time.Hour).UnixMilli())
	require.NoError(t, os.WriteFile(credentialsPath, []byte(credentials), 0o600))

	manager, err := provider.Open(provider.Options{
		CachePath: cachePath, CredentialsPath: credentialsPath, HTTPClient: refusingClient(t),
	})
	require.NoError(t, err)
	return manager, credentialsPath
}

func TestObservationNamesTheProvidersACredentialExistsForAndNothingElse(t *testing.T) {
	manager, _ := connectedManager(t)

	facts := manager.Observation()

	assert.True(t, facts.Known)
	assert.Equal(t, connectedProviders, facts.Connected, "presence is the whole of what a credential says")
	assert.Equal(t, 3, facts.Catalog, "the saved provider joins the two built-in ones")
	assert.True(t, facts.Cached)

	rendered := fmt.Sprintf("%#v", facts)
	for _, secret := range []string{
		sentinelKey, sentinelUser, sentinelQuery, sentinelAccount,
		sentinelAccess, sentinelRefresh, "acme.invalid",
	} {
		assert.NotContains(t, rendered, secret, "an observation carries nothing a credential record holds")
	}
	// The suffix of a key is a key. Half a sentinel is as forbidden as all of it.
	assert.NotContains(t, rendered, sentinelKey[len(sentinelKey)-6:])
}

func TestObservationSaysWhetherASavedCatalogWasRead(t *testing.T) {
	dir := t.TempDir()
	fresh, err := provider.Open(provider.Options{
		CachePath:       filepath.Join(dir, "providers.json"),
		CredentialsPath: filepath.Join(dir, "credentials.json"),
		HTTPClient:      refusingClient(t),
	})
	require.NoError(t, err)

	facts := fresh.Observation()
	assert.False(t, facts.Cached, "nothing has been refreshed yet; the built-in table stands alone")
	assert.Equal(t, 2, facts.Catalog)
	assert.Empty(t, facts.Connected, "an empty store is an empty list, not an unknown one")
	assert.Equal(t, "0", facts.Revision)

	saved, _ := connectedManager(t)
	assert.True(t, saved.Observation().Cached)
}

func TestObservingNeverRefreshesTheCatalogOrReadsTheStoreAgain(t *testing.T) {
	manager, credentialsPath := connectedManager(t)

	// Whatever lands in the credential file after the manager opened it must
	// not reach an observation: a read that re-read the store would pick this
	// up, and a read that re-validated it would fail on it.
	poison := fmt.Sprintf(`{"version":1,"providers":{"intruder":{
		"type":"api","key":%q,"base_url":"https://intruder.invalid","protocol":"openai"}}}`, sentinelKey)
	require.NoError(t, os.WriteFile(credentialsPath, []byte(poison), 0o600))

	for range 3 {
		facts := manager.Observation()
		assert.Equal(t, connectedProviders, facts.Connected,
			"an observation answers from what is already in memory")
		assert.NotContains(t, fmt.Sprintf("%#v", facts), "intruder")
	}
	// The refusing client fails the test itself if anything above reached out.
}

func TestObservationFollowsTheCredentialGeneration(t *testing.T) {
	dir := t.TempDir()
	manager, err := provider.Open(provider.Options{
		CachePath:       filepath.Join(dir, "providers.json"),
		CredentialsPath: filepath.Join(dir, "credentials.json"),
		HTTPClient:      refusingClient(t),
	})
	require.NoError(t, err)
	before := manager.Observation()

	require.NoError(t, manager.Connect(provider.ConnectRequest{
		ProviderID:      "zai-coding-plan",
		ExpectedBaseURL: "https://api.z.ai/api/coding/paas/v4",
		APIKey:          sentinelKey,
	}))

	after := manager.Observation()
	assert.Equal(t, []string{"zai-coding-plan"}, after.Connected)
	assert.NotEqual(t, before.Revision, after.Revision,
		"two observations of one store are told apart by generation, not by value")
	assert.NotContains(t, fmt.Sprintf("%#v", after), sentinelKey)
}

func TestANilProviderManagerObservesNothingRatherThanPanicking(t *testing.T) {
	var manager *provider.Manager

	facts := manager.Observation()

	assert.False(t, facts.Known, "no manager is not the same as an empty catalog")
	assert.Empty(t, facts.Connected)
}
