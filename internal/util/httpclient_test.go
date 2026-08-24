package util

import (
	"net/http"
	"slices"
	"testing"
)

// TestSharedTransportNegotiatesHTTP2 guards the provider handshake fix: the
// shared transport must keep HTTP/2 enabled instead of forcing HTTP/1.1-only.
// api.z.ai (and other CDN-fronted providers) stall or drop the TLS handshake
// when the client advertises only http/1.1 in ALPN.
func TestSharedTransportNegotiatesHTTP2(t *testing.T) {
	tr := SharedHTTPTransport()
	if !tr.ForceAttemptHTTP2 {
		t.Fatalf("ForceAttemptHTTP2 = false, want true (HTTP/2 enabled)")
	}
	if tr.TLSClientConfig == nil || !slices.Contains(tr.TLSClientConfig.NextProtos, "h2") {
		t.Fatalf("NextProtos = %v, want h2 in ALPN advertisement", nextProtos(tr))
	}
}

func nextProtos(tr *http.Transport) []string {
	if tr == nil || tr.TLSClientConfig == nil {
		return nil
	}
	return tr.TLSClientConfig.NextProtos
}
