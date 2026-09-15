package util

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestNormalizeLF(t *testing.T) {
	require.Equal(t, "a\nb\nc", NormalizeLF("a\r\nb\rc"))
	require.Equal(t, "already\nlf", NormalizeLF("already\nlf"))
	require.Empty(t, NormalizeLF(""))
}

func TestTruncateRunesLeavesFittingTextAlone(t *testing.T) {
	require.Equal(t, "короткая", TruncateRunes("короткая", 8))
	require.Equal(t, "короткая", TruncateRunes("короткая", 100))
	require.Empty(t, TruncateRunes("", 10))
}

func TestTruncateRunesCountsRunesNotBytes(t *testing.T) {
	require.Equal(t, "аб…", TruncateRunes("абвг", 2))
	require.Equal(t, "…", TruncateRunes("абвг", 0))
	require.Equal(t, "…", TruncateRunes("абвг", -1))
}

// The byte-slicing helpers this replaced cut inside a multi-byte character on
// roughly half of all limits, and the approval prompt showed a replacement
// glyph where the command was cut.
func TestTruncateRunesNeverCutsInsideARune(t *testing.T) {
	path := "bash -lc 'ls /Users/зол/каталог/подкаталог/файл-с-очень-длинным-именем.txt'"
	for limit := range 60 {
		out := TruncateRunes(path, limit)
		require.True(t, utf8.ValidString(out), "limit %d produced invalid UTF-8: %q", limit, out)
		require.Equal(t, limit+1, utf8.RuneCountInString(out), "limit %d", limit)
	}
}

func TestTruncateRunesWithCarriesTheCallersMarker(t *testing.T) {
	require.Equal(t, "аб (rest elided)", TruncateRunesWith("абвг", 2, " (rest elided)"))
	require.Equal(t, "абвг", TruncateRunesWith("абвг", 4, " (rest elided)"))
}

func TestTruncateBytesBacksUpToARuneBoundary(t *testing.T) {
	// "абвг" is 8 bytes; a limit of 3 lands inside the second rune.
	require.Equal(t, "а…", TruncateBytes("абвг", 3))
	require.Equal(t, "абвг", TruncateBytes("абвг", 8))
}

func TestTruncateBytesOnASCIICutsExactly(t *testing.T) {
	require.Equal(t, "ab…", TruncateBytes("abcd", 2))
	require.Equal(t, "abcd", TruncateBytes("abcd", 4))
}

func TestTruncateBytesStaysWithinBudgetAndValid(t *testing.T) {
	text := strings.Repeat("привет ", 40)
	for limit := range 120 {
		out := TruncateBytes(text, limit)
		require.True(t, utf8.ValidString(out), "limit %d produced invalid UTF-8: %q", limit, out)
		require.LessOrEqual(t, len(strings.TrimSuffix(out, "…")), limit, "limit %d", limit)
	}
}

func TestTruncateBytesWithCarriesTheCallersMarker(t *testing.T) {
	require.Equal(t, "а\n…(truncated)", TruncateBytesWith("абвг", 3, "\n…(truncated)"))
	require.Equal(t, "абвг", TruncateBytesWith("абвг", 8, "\n…(truncated)"))
}
