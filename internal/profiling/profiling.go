// Package profiling owns this process's pprof endpoint: the one place that
// decides whether it is served, and the only thing that knows whether the
// listener came up.
//
// It exists as a package rather than as a few lines in main so the endpoint
// has an owner the harness can observe. "COZYPHI_PPROF is set" is not the
// same fact as "profiles are being served" — an address already in use fails
// silently otherwise — and only whoever started the listener can tell them
// apart.
//
// Enable with:  COZYPHI_PPROF=127.0.0.1:6060 cozyphi
// Then:         curl http://127.0.0.1:6060/debug/pprof/goroutine?debug=2
package profiling

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	//nolint:gosec // G108: pprof handlers on DefaultServeMux; served only when COZYPHI_PPROF is set
	_ "net/http/pprof"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// EnvAddr is the environment entry that asks for an endpoint, by naming the
// address to serve it on.
const EnvAddr = "COZYPHI_PPROF"

// readHeaderTimeout bounds how long a client may take to send its headers.
// Nothing here is meant to face a network, but a server with no timeout at
// all is a server one idle connection can hold open.
const readHeaderTimeout = 5 * time.Second

// Endpoint is one pprof listener. It carries what the harness may say about
// itself — how far its address reaches and whether it is up — and never the
// address, which is answered about rather than reported.
type Endpoint struct {
	mu        sync.Mutex
	exposure  diag.ProfilingExposure
	lifecycle diag.ProfilingLifecycle
	server    *http.Server
}

// The endpoint this process started, if any. The pprof handlers live on the
// default mux, so there is one endpoint per process by construction and this
// is where the harness's wiring finds it: main starts it before any registry
// exists, and a registry built later has no other way to reach it.
var (
	processMu sync.Mutex
	process   *Endpoint
)

// Start serves /debug/pprof when the environment names an address, and does
// nothing at all when it does not. It returns the endpoint it started, or
// nil when none was asked for.
//
// The listener is opened here rather than in the goroutine, so an address
// that is malformed or already taken is known to be a failure now instead of
// being reported as an endpoint that is up. Serving then happens in the
// background, because it never ends on its own.
func Start(out io.Writer) *Endpoint {
	addr := strings.TrimSpace(os.Getenv(EnvAddr))
	if addr == "" {
		setProcess(nil)
		return nil
	}
	e := &Endpoint{exposure: exposureOf(addr), lifecycle: diag.ProfilingStopped}
	setProcess(e)

	// A ListenConfig rather than net.Listen: the listener outlives every
	// request that could carry a deadline, so the context it is opened under
	// is the process's own and ends when the process does.
	var config net.ListenConfig
	listener, err := config.Listen(context.Background(), "tcp", addr)
	if err != nil {
		report(out, "cozyphi: pprof:", err)
		return e
	}
	server := &http.Server{ReadHeaderTimeout: readHeaderTimeout}
	e.mu.Lock()
	e.server, e.lifecycle = server, diag.ProfilingServing
	e.mu.Unlock()
	report(out, "cozyphi: pprof on http://"+listener.Addr().String()+"/debug/pprof/")
	go func() {
		err := server.Serve(listener)
		e.mu.Lock()
		e.lifecycle = diag.ProfilingStopped
		e.mu.Unlock()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			report(out, "cozyphi: pprof:", err)
		}
	}()
	return e
}

// Close stops serving. A nil endpoint is one that was never started, and
// closing it is not an error.
func (e *Endpoint) Close() error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	server := e.server
	e.server = nil
	e.mu.Unlock()
	if server == nil {
		return nil
	}
	return server.Close()
}

// exposureOf is how far an address reaches, decided from the address alone.
// Nothing is resolved: a name is not looked up, because answering a question
// about this process must not put a query on the network. A name that is not
// literally localhost is therefore reported as a named host, which is the
// cautious answer of the two.
func exposureOf(addr string) diag.ProfilingExposure {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.Trim(host, "[]")
	if host == "" || host == "0.0.0.0" || host == "::" {
		return diag.ProfilingEveryInterface
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return diag.ProfilingLoopback
		}
		if ip.IsUnspecified() {
			return diag.ProfilingEveryInterface
		}
		return diag.ProfilingNamedHost
	}
	if strings.EqualFold(host, "localhost") {
		return diag.ProfilingLoopback
	}
	return diag.ProfilingNamedHost
}

// setProcess records which endpoint this process is serving from, so a
// registry built after startup can find it.
func setProcess(e *Endpoint) {
	processMu.Lock()
	process = e
	processMu.Unlock()
}

// current is the endpoint this process started, or nil.
func current() *Endpoint {
	processMu.Lock()
	defer processMu.Unlock()
	return process
}

// report writes one line to the caller's stream, if it wanted one.
func report(out io.Writer, parts ...any) {
	if out == nil {
		return
	}
	fmt.Fprintln(out, parts...)
}
