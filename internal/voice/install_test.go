package voice

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// modelBytes is a plausible ggml file: the magic plus filler.
func modelBytes(n int) []byte {
	body := make([]byte, n)
	copy(body, ggmlMagic)
	for i := len(ggmlMagic); i < n; i++ {
		body[i] = byte('a' + i%26)
	}
	return body
}

// recorder collects the Range headers the installer sent, so a resume can be
// asserted byte-exactly.
type recorder struct {
	mu     sync.Mutex
	ranges []string
	hits   int
}

func (r *recorder) record(rng string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ranges = append(r.ranges, rng)
	r.hits++
}

func (r *recorder) snapshot() ([]string, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ranges...), r.hits
}

// modelServer serves body and honours Range the way huggingface does.
func modelServer(t *testing.T, body []byte, rec *recorder) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rng := r.Header.Get("Range")
		rec.record(rng)
		start := 0
		if rng != "" {
			_, _ = fmt.Sscanf(rng, "bytes=%d-", &start)
		}
		if start >= len(body) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)-start))
		if start > 0 {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(body)-1, len(body)))
			w.WriteHeader(http.StatusPartialContent)
		}
		_, _ = w.Write(body[start:])
	}))
	t.Cleanup(srv.Close)
	return srv
}

// tinyModel is the catalog entry the installer tests download.
func tinyModel(t *testing.T) Model {
	t.Helper()
	m, ok := LookupModel("tiny")
	require.True(t, ok)
	return m
}

func TestInstallDownloadsVerifiesAndReportsProgress(t *testing.T) {
	body := modelBytes(3 * downloadChunk)
	rec := &recorder{}
	srv := modelServer(t, body, rec)
	dir := filepath.Join(t.TempDir(), "models")
	m := tinyModel(t)

	var reports []InstallProgress
	path, err := Install(t.Context(), m, InstallOptions{
		Dir:      dir,
		URL:      srv.URL,
		Progress: func(p InstallProgress) { reports = append(reports, p) },
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "ggml-tiny.bin"), path)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, body, got)
	assert.NoFileExists(t, path+partSuffix)

	ranges, hits := rec.snapshot()
	assert.Equal(t, 1, hits)
	assert.Equal(t, []string{""}, ranges)

	require.NotEmpty(t, reports)
	assert.Equal(t, InstallProgress{Name: "tiny", Done: 0, Total: int64(len(body))}, reports[0])
	last := reports[len(reports)-1]
	assert.Equal(t, int64(len(body)), last.Done)
	assert.Equal(t, last.Total, last.Done)
	assert.Equal(t, "100%", last.Percent())
	for i := 1; i < len(reports); i++ {
		assert.NotEqual(t, reports[i-1].Done, reports[i].Done, "duplicate progress report")
	}
}

func TestInstallResumesFromThePartFile(t *testing.T) {
	body := modelBytes(2048)
	rec := &recorder{}
	srv := modelServer(t, body, rec)
	dir := t.TempDir()
	m := tinyModel(t)
	part := filepath.Join(dir, m.File) + partSuffix
	require.NoError(t, os.WriteFile(part, body[:700], 0o600))

	path, err := Install(t.Context(), m, InstallOptions{Dir: dir, URL: srv.URL})
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, body, got)
	assert.NoFileExists(t, part)

	ranges, _ := rec.snapshot()
	assert.Equal(t, []string{"bytes=700-"}, ranges)
}

func TestInstallRestartsWhenTheServerIgnoresTheRange(t *testing.T) {
	body := modelBytes(1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	m := tinyModel(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, m.File)+partSuffix, []byte("stale bytes"), 0o600))

	path, err := Install(t.Context(), m, InstallOptions{Dir: dir, URL: srv.URL})
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

func TestInstallTreatsA416AsACompletePartFile(t *testing.T) {
	body := modelBytes(1024)
	rec := &recorder{}
	srv := modelServer(t, body, rec)
	dir := t.TempDir()
	m := tinyModel(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, m.File)+partSuffix, body, 0o600))

	path, err := Install(t.Context(), m, InstallOptions{Dir: dir, URL: srv.URL})
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

func TestInstallDropsAPartFileThatIsNotAModel(t *testing.T) {
	body := []byte("<html>not a model</html>")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	m := tinyModel(t)
	_, err := Install(t.Context(), m, InstallOptions{Dir: dir, URL: srv.URL})
	require.Error(t, err)
	assert.Equal(t, "downloaded file is not a ggml model — try /voice install again", err.Error())
	assert.NoFileExists(t, filepath.Join(dir, m.File))
	assert.NoFileExists(t, filepath.Join(dir, m.File)+partSuffix)
}

func TestInstallKeepsThePartFileWhenTheBodyIsShort(t *testing.T) {
	body := modelBytes(4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body[:64])
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		panic(http.ErrAbortHandler)
	}))
	defer srv.Close()

	dir := t.TempDir()
	m := tinyModel(t)
	_, err := Install(t.Context(), m, InstallOptions{Dir: dir, URL: srv.URL})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download interrupted at ")
	assert.Contains(t, err.Error(), "/voice install resumes it")
	assert.NoFileExists(t, filepath.Join(dir, m.File))
	assert.Equal(t, int64(64), fileSize(filepath.Join(dir, m.File)+partSuffix))
}

func TestInstallKeepsThePartFileWhenTheContextIsCancelled(t *testing.T) {
	body := modelBytes(4096)
	srv := modelServer(t, body, &recorder{})
	dir := t.TempDir()
	m := tinyModel(t)
	part := filepath.Join(dir, m.File) + partSuffix
	require.NoError(t, os.WriteFile(part, body[:700], 0o600))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Cancelling from the first progress report is what quitting mid-download
	// does: the bytes already fetched must survive for the next resume.
	_, err := Install(ctx, m, InstallOptions{
		Dir:      dir,
		URL:      srv.URL,
		Progress: func(_ InstallProgress) { cancel() },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download cancelled — /voice install resumes it")
	assert.ErrorIs(t, err, context.Canceled)
	assert.NoFileExists(t, filepath.Join(dir, m.File))
	assert.Equal(t, int64(700), fileSize(part))
}

func TestInstallReportsAnHTTPFailureAndKeepsThePartFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, &http.Request{})
	}))
	defer srv.Close()

	dir := t.TempDir()
	m := tinyModel(t)
	part := filepath.Join(dir, m.File) + partSuffix
	require.NoError(t, os.WriteFile(part, modelBytes(128), 0o600))

	_, err := Install(t.Context(), m, InstallOptions{Dir: dir, URL: srv.URL})
	require.Error(t, err)
	assert.Equal(t, "download failed: HTTP 404", err.Error())
	assert.FileExists(t, part)
	assert.NoFileExists(t, filepath.Join(dir, m.File))
}

func TestInstallSkipsTheDownloadWhenTheModelIsAlreadyInstalled(t *testing.T) {
	rec := &recorder{}
	srv := modelServer(t, modelBytes(1024), rec)
	dir := t.TempDir()
	m := tinyModel(t)
	final := filepath.Join(dir, m.File)
	require.NoError(t, os.WriteFile(final, modelBytes(512), 0o600))

	path, err := Install(t.Context(), m, InstallOptions{Dir: dir, URL: srv.URL})
	require.NoError(t, err)
	assert.Equal(t, final, path)
	_, hits := rec.snapshot()
	assert.Equal(t, 0, hits)
}

func TestInstallRefusesAnUnknownModelOrAMissingDirectory(t *testing.T) {
	_, err := Install(t.Context(), Model{Name: "huge"}, InstallOptions{Dir: t.TempDir()})
	require.Error(t, err)
	assert.Equal(t, "unknown speech model", err.Error())

	_, err = Install(t.Context(), tinyModel(t), InstallOptions{})
	require.Error(t, err)
	assert.Equal(t, "no models directory to install into", err.Error())
}

func TestInstallProgressPercent(t *testing.T) {
	assert.Equal(t, "42%", InstallProgress{Done: 42, Total: 100}.Percent())
	assert.Equal(t, "0%", InstallProgress{Done: 0, Total: 100}.Percent())
	assert.Equal(t, "100%", InstallProgress{Done: 120, Total: 100}.Percent())
	assert.Empty(t, InstallProgress{Done: 42}.Percent())
}
