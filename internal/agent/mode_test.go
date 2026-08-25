package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeModeIncludesUsePlan(t *testing.T) {
	require.Equal(t, ModeUsePlan, normalizeMode(ModeUsePlan))
	require.Equal(t, ModePlan, normalizeMode(ModePlan))
	require.Equal(t, ModeBuild, normalizeMode(ModeBuild))
	require.Equal(t, ModeBuild, normalizeMode(""))
	require.Equal(t, ModeBuild, normalizeMode("bogus"))
}
