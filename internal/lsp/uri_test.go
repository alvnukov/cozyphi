package lsp

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFileURIRoundTrip pins the encode and decode of file:// URIs for both
// platforms from a single POSIX host. The OS-specific normalization lives
// behind toURIPath and osPathFromURIPath, which take an explicit windows flag,
// so the Windows cases are exercised deterministically without a Windows box.
// url.Parse is platform-independent, so it can stand in for the wire on either
// side.
func TestFileURIRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		windows bool
		osPath  string
		uri     string
	}{
		{"posix plain", false, "/home/u/main.go", "file:///home/u/main.go"},
		{"posix space", false, "/home/u/a b.go", "file:///home/u/a%20b.go"},
		{"windows plain", true, `C:\Users\zx\main.go`, "file:///C:/Users/zx/main.go"},
		{"windows space", true, `C:\Users\zx\a b.go`, "file:///C:/Users/zx/a%20b.go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// encode: OS path -> file:// URI.
			got := "file://" + fileURIEscape(toURIPath(tc.osPath, tc.windows))
			assert.Equal(t, tc.uri, got, "encode")

			// decode: file:// URI -> OS path. url.Parse percent-decodes the
			// path the same way pathFromURI does in production.
			u, err := url.Parse(tc.uri)
			require.NoError(t, err)
			assert.Equal(t, tc.osPath, osPathFromURIPath(u.Path, tc.windows), "decode")
		})
	}
}

// TestFileURIDecodesPercentEncodedDriveColon covers servers that emit the drive
// colon percent-encoded (file:///C%3A/...): once url.Parse decodes the path it
// must normalize to the same Windows path as the literal-colon form.
func TestFileURIDecodesPercentEncodedDriveColon(t *testing.T) {
	u, err := url.Parse("file:///C%3A/Users/zx/main.go")
	require.NoError(t, err)
	assert.Equal(t, `C:\Users\zx\main.go`, osPathFromURIPath(u.Path, true))
}
