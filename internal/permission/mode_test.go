package permission

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModeOf(t *testing.T) {
	assert.Equal(t, Mode(""), ModeOf(nil))
	assert.Equal(t, Mode(""), ModeOf(AllowAll{}))

	g, err := NewGate(Policy{Mode: ModeReadonly}, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, ModeReadonly, ModeOf(g))

	var enabled atomic.Bool
	bypass := &BypassGate{Inner: g, Enabled: &enabled}
	assert.Equal(t, ModeReadonly, ModeOf(bypass))
}
