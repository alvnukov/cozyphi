package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Every fixture is a published documentation credential, never a live value.
func TestRedactMasksKnownSecretShapes(t *testing.T) {
	cases := []string{
		"AKIAIOSFODNN7EXAMPLE",
		"aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"ghp_16C7e42F292c6917E981eE487bD0aA9aBCdefgh",
		"sk-provocaretestkey0123456789abcdef",
		"api_key=8dzHxTGmSsaKq4p3XfBq2c",
		"password=hunter2_certified_fixture",
		"COZYPHI_API_KEY=skliveabcdefghijklmnop",
		"Bearer eyJhbGciOiJIUzI1NiJ9.fake.fake",
	}
	for _, secret := range cases {
		t.Run(secret, func(t *testing.T) {
			got := Redact("deploy with " + secret + " and tell nobody")
			assert.NotContains(t, got, secret, "the raw secret shape must not survive")
			assert.Contains(t, got, Marker, "the mask replaces the matched span")
			assert.Contains(t, got, "deploy with ", "prose around the secret survives")
			assert.Contains(t, got, " and tell nobody")
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
	}
	for _, line := range prose {
		assert.Equal(t, line, Redact(line))
	}
	assert.Empty(t, Redact(""))
}

// Redact is idempotent: the marker matches no rule, so the load path and the
// projection renderer can re-mask already-masked text without harm.
func TestRedactIsIdempotent(t *testing.T) {
	once := Redact("export COZYPHI_API_KEY=skliveabcdefghijklmnop now")
	assert.Equal(t, once, Redact(once))
}
