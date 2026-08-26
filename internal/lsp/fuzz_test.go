package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// The fuzz targets pin the adversarial contract: arbitrary bytes on the wire,
// in URIs, location shapes, markup, and diagnostic payloads must never panic
// the client or escape the frozen bounds — they can only produce typed errors
// or bounded normalized output.

// FuzzReadFrame drives the framing reader with arbitrary wire bytes: it must
// never panic and never hand out a body above the frozen frame cap.
func FuzzReadFrame(f *testing.F) {
	f.Add([]byte("Content-Length: 2\r\n\r\n{}"))
	f.Add([]byte("Content-Length: -1\r\n\r\n"))
	f.Add([]byte("Content-Length: 99999999999999999999\r\n\r\n"))
	f.Add([]byte("Content-Length: 5\r\n\r\nshort"))
	f.Add([]byte("\r\n\r\n"))
	f.Add([]byte("Content-Length: 0\r\n\r\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		raw, err := readFrame(bufio.NewReader(bytes.NewReader(data)))
		if err == nil && len(raw) > MaxFrameBytes {
			t.Fatalf("frame body %d exceeds cap %d", len(raw), MaxFrameBytes)
		}
	})
}

// FuzzContentLength drives the header parser: any input parses without panic;
// cap enforcement belongs to readFrame and is asserted there.
func FuzzContentLength(f *testing.F) {
	f.Add("Content-Length: 42\r\n\r\n")
	f.Add("content-length:\t7 \r\n\r\n")
	f.Add("X-Other: 1\r\nContent-Length: 3\r\n\r\n")
	f.Add("garbage")
	f.Add("Content-Length: \x00\r\n\r\n")
	f.Fuzz(func(_ *testing.T, header string) {
		_, _ = contentLength(header)
	})
}

// FuzzPathFromURI proves arbitrary URI text either fails closed or yields an
// absolute cleaned path: a server can never smuggle a relative or remote
// reference past the URI boundary.
func FuzzPathFromURI(f *testing.F) {
	f.Add("file:///abs/path.go")
	f.Add("file://localhost/etc/passwd")
	f.Add("file://evil/share/x")
	f.Add("http://example.com/x")
	f.Add("file:relative.go")
	f.Add("%")
	f.Fuzz(func(t *testing.T, raw string) {
		path, err := pathFromURI(raw)
		if err != nil {
			return
		}
		if !filepath.IsAbs(path) || path != filepath.Clean(path) {
			t.Fatalf("pathFromURI(%q) = %q: not an absolute cleaned path", raw, path)
		}
	})
}

// FuzzLocate drives the Location normalization seam: any decoded shape is
// contained, converted, or rejected — never a panic and never an escaping
// workspace-relative path.
func FuzzLocate(f *testing.F) {
	workspace := f.TempDir()
	f.Add(`{"uri":"file:///nonexistent.go","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}}}`)
	f.Add(`{"uri":"file://host/x","range":{}}`)
	f.Add(`{"uri":"","range":{"start":{"line":1,"character":2},"end":{"line":3,"character":4}}}`)
	f.Add(`{"uri":"file:///a%00b","range":null}`)
	f.Fuzz(func(t *testing.T, raw string) {
		var l wireLocation
		if json.Unmarshal([]byte(raw), &l) != nil {
			return
		}
		loc, _, _, _ := locate(workspace, OpDefinition, l)
		if loc.File != "" && (loc.File == ".." || strings.HasPrefix(loc.File, "../")) {
			t.Fatalf("locate leaked %q outside the workspace", loc.File)
		}
	})
}

// FuzzNormalizeDefinition drives the definition payload decoder across
// Location, LocationLink, array, and garbage shapes.
func FuzzNormalizeDefinition(f *testing.F) {
	m := &Manager{workspace: f.TempDir()}
	f.Add(`null`)
	f.Add(`[]`)
	f.Add(`{"uri":"file:///x.go","range":{}}`)
	f.Add(
		`[{"targetUri":"file:///y.go","targetSelectionRange":{"start":{"line":1,"character":2},"end":{"line":1,"character":4}}}]`,
	)
	f.Add(`[{"uri":123}]`)
	f.Add(`{"weird":true}`)
	f.Fuzz(func(_ *testing.T, raw string) {
		_, _ = m.normalizeDefinition(json.RawMessage(raw))
	})
}

// FuzzMarkedStringText drives the hover markup decoder: MarkedString,
// MarkupContent, arrays, and garbage must normalize or fail closed.
func FuzzMarkedStringText(f *testing.F) {
	f.Add(`"plain"`)
	f.Add(`{"language":"go","value":"x"}`)
	f.Add(`[{"language":"go","value":"a"},"b"]`)
	f.Add(`{"kind":"markdown","value":"**b**"}`)
	f.Add(`[]`)
	f.Add(`0`)
	f.Fuzz(func(_ *testing.T, raw string) {
		_, _ = markedStringText(json.RawMessage(raw))
	})
}

// FuzzPublishDiagnostics drives the push-diagnostics decoder: arbitrary
// publishDiagnostics payloads must never panic the cache.
func FuzzPublishDiagnostics(f *testing.F) {
	workspace := f.TempDir()
	f.Add(`{"uri":"file:///x.go","diagnostics":[{"range":{},"severity":1,"message":"m"}]}`)
	f.Add(`{"uri":"file:///x.go","version":2,"diagnostics":[]}`)
	f.Add(`{}`)
	f.Add(`null`)
	f.Add(`{"uri":5,"diagnostics":"no"}`)
	f.Fuzz(func(_ *testing.T, raw string) {
		diag := newDiagCache()
		diag.publish(workspace, newDocStore(), json.RawMessage(raw))
	})
}

// FuzzBoundText proves the text-field bounder never returns more than the
// frozen cap and never splits an originally valid UTF-8 sequence. Sanitizing
// invalid input is not its job: JSON decoding already guarantees valid UTF-8
// for every text field it bounds.
func FuzzBoundText(f *testing.F) {
	f.Add("")
	f.Add(strings.Repeat("é", MaxTextFieldBytes+10))
	f.Add("\xff\xfe")
	f.Fuzz(func(t *testing.T, s string) {
		out, _ := boundText(s)
		if len(out) > MaxTextFieldBytes {
			t.Fatalf("bounded text %d exceeds cap %d", len(out), MaxTextFieldBytes)
		}
		if utf8.ValidString(s) && !utf8.ValidString(out) {
			t.Fatalf("bounder split a valid UTF-8 sequence: %q -> %q", s, out)
		}
	})
}
