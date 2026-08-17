package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComputeLineHashIsThreeLetters(t *testing.T) {
	h := ComputeLineHash("hello world")
	require.Len(t, h, LineHashLen)
	for _, c := range h {
		require.GreaterOrEqual(t, c, 'a')
		require.LessOrEqual(t, c, 'z')
	}
}

func TestComputeLineHashIgnoresWhitespace(t *testing.T) {
	require.Equal(t, ComputeLineHash("a b"), ComputeLineHash("ab"))
	require.Equal(t, ComputeLineHash("\tx\n"), ComputeLineHash("x"))
}

func TestComputeFileHashStableAndSensitive(t *testing.T) {
	a := ComputeFileHash("one\ntwo\n")
	b := ComputeFileHash("one\ntwo\n")
	c := ComputeFileHash("one\ntwo\nthree\n")
	require.Equal(t, a, b)
	require.Len(t, a, FileHashLen)
	require.NotEqual(t, a, c)
	for _, ch := range a {
		ok := (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'F')
		require.True(t, ok, "unexpected file hash rune %q", ch)
	}
}

func TestComputeFileHashNormalizesTrailingWhitespace(t *testing.T) {
	require.Equal(t, ComputeFileHash("line  \n"), ComputeFileHash("line\n"))
	require.Equal(t, ComputeFileHash("line\r\n"), ComputeFileHash(NormalizeFileHashText("line\r\n")))
}

func TestFormatFileHeader(t *testing.T) {
	require.Equal(t, "@file foo/bar.go#A1B2", FormatFileHeader("foo/bar.go", "a1b2"))
}
