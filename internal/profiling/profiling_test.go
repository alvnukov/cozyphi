package profiling

import (
	"io"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// forgetProcess drops this process's record of an endpoint, so one test's
// listener is never another test's answer.
func forgetProcess(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { setProcess(nil) })
	setProcess(nil)
}

// settle gives a goroutine on its way out a chance to finish, so a count
// taken right after a listener closes is not a race with the scheduler.
func settle() {
	for range 50 {
		runtime.Gosched()
		time.Sleep(2 * time.Millisecond)
	}
}

// How exposed the endpoint is is the question worth answering, and it is
// answered from the address alone. Nothing is resolved: a name is not looked
// up, because answering a question about this process must not put a query
// on the network.
func TestHowFarAnAddressReachesIsDecidedWithoutTheNetwork(t *testing.T) {
	for addr, want := range map[string]diag.ProfilingExposure{
		"127.0.0.1:6060":      diag.ProfilingLoopback,
		"localhost:6060":      diag.ProfilingLoopback,
		"LocalHost:6060":      diag.ProfilingLoopback,
		"[::1]:6060":          diag.ProfilingLoopback,
		":6060":               diag.ProfilingEveryInterface,
		"0.0.0.0:6060":        diag.ProfilingEveryInterface,
		"[::]:6060":           diag.ProfilingEveryInterface,
		"10.0.0.4:6060":       diag.ProfilingNamedHost,
		"build.internal:6060": diag.ProfilingNamedHost,
		"not-an-address":      diag.ProfilingNamedHost,
	} {
		assert.Equal(t, want, exposureOf(addr), "address %q", addr)
	}
}

// Nothing is served unless something asks. The endpoint is not merely
// unused: it does not exist, and the observation tells those apart.
func TestNothingIsServedUnlessSomethingAsks(t *testing.T) {
	t.Setenv(EnvAddr, "")
	forgetProcess(t)

	assert.Nil(t, Start(nil), "no address named, no endpoint")

	facts := Observe()
	assert.True(t, facts.Known)
	assert.False(t, facts.Requested)
	assert.Empty(t, facts.Exposure, "there is no address, so there is nothing to be exposed by")
	assert.Equal(t, diag.ProfilingOff, facts.Lifecycle)
}

// An endpoint that came up is serving until it is closed, and the harness
// says which of the two it is — the point of owning the listener rather than
// reading the environment and guessing.
func TestAnEndpointThatCameUpIsServingUntilItIsClosed(t *testing.T) {
	t.Setenv(EnvAddr, "127.0.0.1:0")
	forgetProcess(t)
	settle()
	before := runtime.NumGoroutine()

	var startup strings.Builder
	endpoint := Start(&startup)
	require.NotNil(t, endpoint)
	t.Cleanup(func() { _ = endpoint.Close() })

	serving := Observe()
	assert.True(t, serving.Requested)
	assert.Equal(t, diag.ProfilingLoopback, serving.Exposure)
	assert.Equal(t, diag.ProfilingServing, serving.Lifecycle)

	require.NoError(t, endpoint.Close())
	settle()

	stopped := Observe()
	assert.Equal(t, diag.ProfilingStopped, stopped.Lifecycle)
	assert.NotEqual(t, serving.Revision, stopped.Revision, "the listener ending is a change of state")
	assert.LessOrEqual(t, runtime.NumGoroutine(), before+1, "the closed listener leaves no goroutine behind")

	assert.Contains(t, startup.String(), "127.0.0.1",
		"the operator who asked for the endpoint is told where it is")
	for _, facts := range []diag.ProfilingFacts{serving, stopped} {
		assert.NotContains(t, facts.Revision+string(facts.Exposure), "127.0.0.1",
			"and the harness view answers about the address without reporting it")
	}
}

// "The environment is set" is not "profiles are being served". An address
// already in use fails the bind, and an endpoint that never came up must not
// be reported as one that is up.
func TestAnAddressThatCouldNotBeBoundIsNotAnEndpointThatIsUp(t *testing.T) {
	var config net.ListenConfig
	taken, err := config.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = taken.Close() })

	t.Setenv(EnvAddr, taken.Addr().String())
	forgetProcess(t)

	var startup strings.Builder
	endpoint := Start(&startup)
	require.NotNil(t, endpoint, "something asked for an endpoint, and that it failed is the answer")
	t.Cleanup(func() { _ = endpoint.Close() })

	facts := Observe()
	assert.True(t, facts.Requested)
	assert.Equal(t, diag.ProfilingLoopback, facts.Exposure)
	assert.Equal(t, diag.ProfilingStopped, facts.Lifecycle, "nothing is listening")
	assert.NotEmpty(t, startup.String(), "the operator is told their endpoint did not come up")
}

// The environment is read now and the endpoint was started once, so they can
// disagree. An address exported into a running process is asked for and is
// not being served, and that gap is what somebody wondering why the port is
// closed is looking for.
func TestAnAddressExportedIntoARunningProcessAsksForNothing(t *testing.T) {
	t.Setenv(EnvAddr, "")
	forgetProcess(t)
	require.Nil(t, Start(io.Discard))

	t.Setenv(EnvAddr, "0.0.0.0:6060")
	facts := Observe()

	assert.True(t, facts.Requested, "the environment names one now")
	assert.Empty(t, facts.Exposure, "this process started none")
	assert.Equal(t, diag.ProfilingOff, facts.Lifecycle, "and nothing is listening")
}

// Asking whether profiles are being served must not be a way to collect one:
// the observation reads this process's own record and makes no connection.
func TestObservingTheEndpointFetchesNothingFromIt(t *testing.T) {
	t.Setenv(EnvAddr, "127.0.0.1:0")
	forgetProcess(t)

	endpoint := Start(io.Discard)
	require.NotNil(t, endpoint)
	t.Cleanup(func() { _ = endpoint.Close() })

	first := Observe()
	settle()
	assert.Equal(t, first, Observe(), "reading it twice is reading it, not driving it")
	assert.Equal(t, diag.ProfilingServing, Observe().Lifecycle, "and it is still up afterwards")
}

// Closing an endpoint nobody started is not an error: a process that asked
// for none shuts down the same way as one that did.
func TestClosingAnEndpointNobodyStartedIsNotAnError(t *testing.T) {
	var none *Endpoint
	assert.NoError(t, none.Close())
	assert.NoError(t, (&Endpoint{}).Close())
}
