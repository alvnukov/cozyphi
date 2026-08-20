package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeLF(t *testing.T) {
	require.Equal(t, "a\nb\nc", NormalizeLF("a\r\nb\rc"))
	require.Equal(t, "already\nlf", NormalizeLF("already\nlf"))
	require.Empty(t, NormalizeLF(""))
}
