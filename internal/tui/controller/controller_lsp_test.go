package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLSPStatusesNilSafe pins the sidebar data source: without an LSP manager
// (disabled config or failed open) the panel gets nil, never a panic.
func TestLSPStatusesNilSafe(t *testing.T) {
	var c Controller
	assert.Nil(t, c.LSPStatuses())
}
