package util

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	httpMaxIdleConns          = 100
	httpMaxIdleConnsPerHost   = 32
	httpMaxConnsPerHost       = 64
	httpIdleConnTimeout       = 50 * time.Second
	httpDialTimeout           = 30 * time.Second
	httpKeepAlive             = 30 * time.Second
	httpTLSHandshakeTimeout   = 10 * time.Second
	httpResponseHeaderTimeout = 3 * time.Minute
	httpExpectContinueTimeout = 1 * time.Second
)

var (
	sharedHTTPClient   *http.Client
	sharedTransport    *http.Transport
	initSharedHTTPOnce sync.Once
)

func initSharedHTTP() {
	dialer := &net.Dialer{Timeout: httpDialTimeout, KeepAlive: httpKeepAlive}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = dialer.DialContext
	// Keep HTTP/2 enabled (the cloned default advertises "h2" via ALPN).
	// Some providers (e.g. api.z.ai) stall or drop TLS handshakes when the
	// client forces HTTP/1.1 only.
	tr.MaxIdleConns = httpMaxIdleConns
	tr.MaxIdleConnsPerHost = httpMaxIdleConnsPerHost
	tr.MaxConnsPerHost = httpMaxConnsPerHost
	tr.IdleConnTimeout = httpIdleConnTimeout
	tr.TLSHandshakeTimeout = httpTLSHandshakeTimeout
	tr.ResponseHeaderTimeout = httpResponseHeaderTimeout
	tr.ExpectContinueTimeout = httpExpectContinueTimeout

	sharedTransport = tr
	sharedHTTPClient = &http.Client{Transport: tr, Timeout: 0}
}

// DefaultHTTPClient returns a shared http.Client suitable for SSE streams.
func DefaultHTTPClient() *http.Client {
	initSharedHTTPOnce.Do(initSharedHTTP)
	return sharedHTTPClient
}

// SharedHTTPTransport returns the shared Transport, for building custom clients
// (e.g. ones with their own redirect policy) that still reuse the connection pool.
func SharedHTTPTransport() *http.Transport {
	initSharedHTTPOnce.Do(initSharedHTTP)
	return sharedTransport
}
