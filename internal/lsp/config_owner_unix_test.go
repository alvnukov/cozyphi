//go:build !windows

package lsp

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLoadConfigWrongOwnerRejected fails closed for a config owned by another
// account. Changing ownership needs privileges: the test skips where the
// platform or the caller cannot demonstrate it.
func TestLoadConfigWrongOwnerRejected(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root owns every file; ownership check is not observable")
	}
	path := writeConfigFile(t, t.TempDir(), "lsp.json", `{"enabled":true}`, 0o600)
	if err := syscall.Chown(path, 0, 0); err != nil {
		t.Skip("cannot change file owner without privileges")
	}
	_, err := LoadConfig(path)
	require.Error(t, err)
}
