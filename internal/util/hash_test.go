package util

import (
	"fmt"
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

// The TAG is the display form of the revision, never its identity.
func TestRevisionTagRoundTrip(t *testing.T) {
	rev := RevisionOf("one\ntwo\n")
	require.Equal(t, rev, RevisionOf("one\ntwo\n"), "the same text is the same revision")
	require.NotEqual(t, rev, RevisionOf("one\ntwo\nthree\n"))
	require.Equal(t, ComputeFileHash("one\ntwo\n"), rev.Tag(), "the tag is what the model sees")
	require.Len(t, rev.Tag(), FileHashLen)
	require.Equal(t, "A1B2", Revision(0xDEADBEEFA1B2).Tag(), "only the low 16 bits are displayed")
}

// The equivalences the revision accepts on purpose: callers hand it text with
// LF endings, and trailing spaces/tabs never distinguish two revisions.
func TestRevisionIgnoresTrailingWhitespace(t *testing.T) {
	require.Equal(t, RevisionOf("line\n"), RevisionOf("line  \n"))
	require.Equal(t, RevisionOf("line\n"), RevisionOf("line\t\n"))
}

// The display TAG is 16 bits, so different revisions do share one: finding a
// collision here is what the ledger and the edit pre-swap check defend against.
func TestDifferentRevisionsCanShareOneTag(t *testing.T) {
	first := RevisionOf("start\nexternal_value_0\nend\n")
	for i := 1; i < 1<<17; i++ {
		other := RevisionOf(fmt.Sprintf("start\nexternal_value_%d\nend\n", i))
		if other.Tag() != first.Tag() {
			continue
		}
		require.NotEqual(t, first, other, "the search must find a collision, not the same text twice")
		return
	}
	t.Fatal("no tag collision found in 2^17 candidates")
}

func TestFormatFileHeader(t *testing.T) {
	require.Equal(t, "@file foo/bar.go#A1B2", FormatFileHeader("foo/bar.go", "a1b2"))
}
