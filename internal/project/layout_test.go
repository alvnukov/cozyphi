package project

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGlobalLayoutLSPConfigFile(t *testing.T) {
	t.Parallel()
	g := GlobalLayout{root: "/tmp/.cozyphi"}
	assert.Equal(t, filepath.Join("/tmp/.cozyphi", "lsp.json"), g.LSPConfigFile())
}
