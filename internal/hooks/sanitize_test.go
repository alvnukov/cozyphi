package hooks

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeEnvStripsSecretsAndInjects(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin",
		"PHI_API_KEY=sk-secret",
		"MY_TOKEN=abc",
		"SAFE=1",
		"PHI_HOOK_EVENT=stale",
	}
	out := sanitizeEnv(parent, hookEnv{
		Event:      "pre_tool",
		SessionID:  "sid",
		Cwd:        "/cwd",
		ProjectDir: "/proj",
	})

	joined := strings.Join(out, "\n")
	assert.Contains(t, joined, "PATH=/usr/bin")
	assert.Contains(t, joined, "SAFE=1")
	assert.NotContains(t, joined, "sk-secret")
	assert.NotContains(t, joined, "MY_TOKEN=")
	assert.Contains(t, joined, EnvHookEvent+"=pre_tool")
	assert.Contains(t, joined, EnvSessionID+"=sid")
	assert.Contains(t, joined, EnvCwd+"=/cwd")
	assert.Contains(t, joined, EnvProjectDir+"=/proj")
	assert.NotContains(t, joined, "stale")
}

func TestIsSensitiveEnvKey(t *testing.T) {
	assert.True(t, isSensitiveEnvKey("PHI_API_KEY"))
	assert.True(t, isSensitiveEnvKey("openai_api_key"))
	assert.True(t, isSensitiveEnvKey("DB_PASSWORD"))
	assert.False(t, isSensitiveEnvKey("PATH"))
	assert.False(t, isSensitiveEnvKey("HOME"))
}
