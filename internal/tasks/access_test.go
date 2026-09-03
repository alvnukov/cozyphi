package tasks_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tasks"
)

func TestParseAccess(t *testing.T) {
	for raw, want := range map[string]tasks.Access{
		"":      tasks.AccessWrite,
		"write": tasks.AccessWrite,
		"Ask":   tasks.AccessAsk,
		"read":  tasks.AccessRead,
		"OFF":   tasks.AccessOff,
	} {
		got, err := tasks.ParseAccess(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
	}
	padded, err := tasks.ParseAccess("  ask\t")
	require.NoError(t, err)
	assert.Equal(t, tasks.AccessAsk, padded, "surrounding whitespace is not part of the value")
	_, err = tasks.ParseAccess("maybe")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown value "maybe"`)
	assert.Contains(t, err.Error(), "off, read, ask or write")
}

func TestAccessLevels(t *testing.T) {
	assert.Equal(t, tasks.AccessWrite, tasks.Access("").Normalized())
	assert.True(t, tasks.Access("").Writable())
	assert.True(t, tasks.AccessAsk.Writable())
	assert.False(t, tasks.AccessRead.Writable())
	assert.False(t, tasks.AccessOff.Writable())

	// The row tightens from the default and wraps.
	var seen []tasks.Access
	level := tasks.AccessWrite
	for range 4 {
		level = level.Next()
		seen = append(seen, level)
	}
	assert.Equal(t, []tasks.Access{tasks.AccessAsk, tasks.AccessRead, tasks.AccessOff, tasks.AccessWrite}, seen)
	assert.Equal(t, "write", tasks.Access("").String())
}
