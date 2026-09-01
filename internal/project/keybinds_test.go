package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeKeybindsConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// TestParseConfigValidatesKeybinds: the keybinds section is checked at load —
// an unknown command or two commands on one chord is a config error, never a
// dead key discovered at the keyboard.
func TestParseConfigValidatesKeybinds(t *testing.T) {
	base := "models:\n  - name: m\n    api_key: k\n"

	cfg, err := parseConfigFile(writeKeybindsConfig(t, base+"keybinds:\n  plan-editor: Ctrl+G\n"))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"plan-editor": "Ctrl+G"}, cfg.Keybinds)

	_, err = parseConfigFile(writeKeybindsConfig(t, base+"keybinds:\n  help: Ctrl+K\n"))
	require.Error(t, err, "help and palette on one chord must fail the load")
	assert.Contains(t, err.Error(), "palette")

	_, err = parseConfigFile(writeKeybindsConfig(t, base+"keybinds:\n  warp-core: F2\n"))
	require.ErrorContains(t, err, "unknown command")
}
