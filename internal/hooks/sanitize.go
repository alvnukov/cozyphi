package hooks

import (
	"os"
	"strings"
)

// Env keys injected into every command hook process.
const (
	EnvHookEvent   = "PHI_HOOK_EVENT"
	EnvSessionID   = "PHI_SESSION_ID"
	EnvCwd         = "PHI_CWD"
	EnvProjectDir  = "PHI_PROJECT_DIR"
)

// sensitiveEnvSubstrings match (case-insensitive) against the env key.
// Keys containing any of these are stripped before spawning a hook.
var sensitiveEnvSubstrings = []string{
	"API_KEY",
	"SECRET",
	"TOKEN",
	"PASSWORD",
	"CREDENTIAL",
	"PRIVATE_KEY",
	"AUTHORIZATION",
	"BEARER",
	"OAUTH",
	"AWS_ACCESS_KEY",
	"AWS_SECRET",
	"AWS_SESSION_TOKEN",
	"PHI_API_KEY",
}

type hookEnv struct {
	Event      string
	SessionID  string
	Cwd        string
	ProjectDir string
}

// sanitizeEnv copies parent env, drops sensitive keys, and injects PHI_HOOK_*.
func sanitizeEnv(parent []string, extra hookEnv) []string {
	out := make([]string, 0, len(parent)+4)
	for _, kv := range parent {
		key, _, _ := strings.Cut(kv, "=")
		if isSensitiveEnvKey(key) {
			continue
		}
		// Drop keys we are about to overwrite so duplicates don't confuse hooks.
		switch key {
		case EnvHookEvent, EnvSessionID, EnvCwd, EnvProjectDir:
			continue
		}
		out = append(out, kv)
	}
	out = append(out,
		EnvHookEvent+"="+extra.Event,
		EnvSessionID+"="+extra.SessionID,
		EnvCwd+"="+extra.Cwd,
		EnvProjectDir+"="+extra.ProjectDir,
	)
	return out
}

func isSensitiveEnvKey(key string) bool {
	k := strings.ToUpper(key)
	if k == "PHI_API_KEY" {
		return true
	}
	for _, sub := range sensitiveEnvSubstrings {
		if strings.Contains(k, sub) {
			return true
		}
	}
	return false
}

// environ is overridable in tests.
var environ = os.Environ
