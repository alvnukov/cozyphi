package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The Anthropic key body carries hyphens and an underscore, which is the shape
// that used to walk through the mask untouched. Like every fixture here it is
// a made-up value, never a live credential.
const (
	anthropicKey  = "sk-ant-api03-FIXTURE_only-not-a-live-key-0000000000000000000000000000AA"
	anthropicBare = "sk-ant-fixtureonlynotalivekey00000000000000AA"
)

// Every fixture is a published documentation credential, never a live value.
// The expectation is the whole line, so a rule that masked only part of a key
// and left the rest in the output fails here rather than passing a
// "does not contain the secret" check on the strength of the missing prefix.
func TestRedactMasksKnownSecretShapes(t *testing.T) {
	cases := []struct{ secret, masked string }{
		{"AKIAIOSFODNN7EXAMPLE", Marker},
		{
			"aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"aws_secret_access_key=" + Marker,
		},
		{"ghp_16C7e42F292c6917E981eE487bD0aA9aBCdefgh", Marker},
		{"sk-provocaretestkey0123456789abcdef", Marker},
		{"api_key=8dzHxTGmSsaKq4p3XfBq2c", "api_key=" + Marker},
		{"password=hunter2_certified_fixture", "password=" + Marker},
		{"COZYPHI_API_KEY=skliveabcdefghijklmnop", "COZYPHI_API_KEY=" + Marker},
		{"Bearer eyJhbGciOiJIUzI1NiJ9.fake.fake", "Bearer " + Marker},
		// The providers this product actually talks to. Anthropic first: it is
		// the one the pack used to miss.
		{anthropicKey, Marker},
		{anthropicBare, Marker},
		{"sk-proj-fixtureonly0123456789abcdefghij", Marker},
		{"sk-svcacct-fixtureonly0123456789abcdefghij", Marker},
		{"sk-admin-fixtureonly0123456789abcdefghij", Marker},
		{"gsk_fixtureonly0123456789abcdefghijklmnopqrstuvwxyz01", Marker},
		{"AIzaSyFIXTUREonly-0123456789_abcdefghij", Marker},
		// The two ways a key reaches text: an exported environment variable,
		// and the YAML this product's own configuration is written in. The
		// assignment rule reads only the first, so the value in the second is
		// masked by its own shape or not at all.
		{"ANTHROPIC_API_KEY=" + anthropicKey, "ANTHROPIC_API_KEY=" + Marker},
		{"api_key: " + anthropicKey, "api_key: " + Marker},
	}
	for _, tc := range cases {
		t.Run(tc.secret, func(t *testing.T) {
			got := Redact("deploy with " + tc.secret + " and tell nobody")
			assert.Equal(t, "deploy with "+tc.masked+" and tell nobody", got,
				"the whole credential is replaced and the prose around it survives")
		})
	}
}

// The pack must stay tight: mentions of secret vocabulary without an actual
// credential shape are ordinary prose and pass through byte-for-byte.
func TestRedactLeavesOrdinaryProseUntouched(t *testing.T) {
	prose := []string{
		"make fmt-check lint test; the AWS docs and the sk- placeholder in the README stay",
		"task-sk-v2-plan-tool-safety-hardening keeps its kebab slug",
		"secret: keep-this-context-bounded is a directive, not a credential",
		"commit 6e86df0d31f5c4e2b3a4ad0e12ab34cd56ef7890 lands the merge",
		// A word ending in "risk" lends the letters of the Anthropic prefix to
		// the slug that follows it; the word boundary is what refuses them.
		"risk-ant-colony-optimization-benchmark-2026 is a branch name",
		// The prefixes named without a key behind them.
		"the sk-ant- prefix is what the rule anchors to, not the hyphens",
		"sk-proj- and sk-svcacct- name kinds of key, not values",
		"gsk_ and AIza are prefixes; neither is a credential on its own",
	}
	for _, line := range prose {
		assert.Equal(t, line, Redact(line))
	}
	assert.Empty(t, Redact(""))
}

// Redact is idempotent: the marker matches no rule, so the load path and the
// projection renderer can re-mask already-masked text without harm.
func TestRedactIsIdempotent(t *testing.T) {
	for _, line := range []string{
		"export COZYPHI_API_KEY=skliveabcdefghijklmnop now",
		"models[0].api_key was " + anthropicKey + ", rotated today",
		"gsk_fixtureonly0123456789abcdefghijklmnopqrstuvwxyz01 and " + anthropicBare,
	} {
		once := Redact(line)
		assert.Equal(t, once, Redact(once))
		assert.Contains(t, once, Marker)
	}
}
