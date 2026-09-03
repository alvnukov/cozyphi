package voice

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"
)

// ggmlMagic is the first four bytes of every ggml model file. It is the only
// thing a downloaded file is verified against: whisper.cpp publishes no
// checksums and the sizes change between releases.
const ggmlMagic = "lmgg"

// partSuffix marks a half-downloaded model. It survives a quit so the next
// /voice install resumes instead of starting over.
const partSuffix = ".part"

// progressInterval is the fastest the progress callback fires; the last call
// of a download always fires.
const progressInterval = 200 * time.Millisecond

// downloadChunk is one read from the network.
const downloadChunk = 256 << 10

// installing keeps one download per process: two concurrent writers would
// fight over the same .part file.
var installing atomic.Bool

// InstallProgress is one report from a running download.
type InstallProgress struct {
	// Name is the catalog model being downloaded.
	Name string
	// Done is how many bytes the .part file holds, resumed bytes included.
	Done int64
	// Total is the full size, or 0 when the server did not say.
	Total int64
}

// Percent renders the progress as whole percent, or "" when the total is
// unknown.
func (p InstallProgress) Percent() string {
	if p.Total <= 0 || p.Done < 0 {
		return ""
	}
	done := min(p.Done, p.Total)
	return strconv.FormatInt(done*100/p.Total, 10) + "%"
}

// InstallOptions carries everything Install needs from the caller.
type InstallOptions struct {
	// Dir is the models directory; it is created when missing.
	Dir string
	// Client is the HTTP client to download with; nil uses the default one,
	// which honors the proxy environment.
	Client *http.Client
	// Progress is called on the downloading goroutine, at most every 200 ms
	// and once at the end of a completed download.
	Progress func(InstallProgress)
	// URL overrides Model.URL; tests point it at an httptest server.
	URL string
}

// Install downloads m into opts.Dir and returns the installed file. It resumes
// an interrupted download from the leftover .part file, verifies the result
// before publishing it, and renames it into place in one step, so the final
// path never holds a partial or corrupt model. Errors are one line each: they
// are what the user sees in a toast.
func Install(ctx context.Context, m Model, opts InstallOptions) (string, error) {
	if m.File == "" {
		return "", errors.New("unknown speech model")
	}
	if opts.Dir == "" {
		return "", errors.New("no models directory to install into")
	}
	if !installing.CompareAndSwap(false, true) {
		return "", errors.New("another model download is already running")
	}
	defer installing.Store(false)

	final := filepath.Join(opts.Dir, m.File)
	if hasGGMLMagic(final) {
		return final, nil
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create the models directory: %w", err)
	}
	part := final + partSuffix
	rep := &progressReporter{fn: opts.Progress, name: m.Name}
	total, err := download(ctx, m, opts, part, rep)
	if err != nil {
		return "", err
	}
	if err := verify(part, total); err != nil {
		// A file that is not a model is worse than no file: drop it so the
		// next install starts clean instead of resuming garbage.
		_ = os.Remove(part)
		return "", err
	}
	if err := os.Rename(part, final); err != nil {
		return "", fmt.Errorf("cannot install the model: %w", err)
	}
	return final, nil
}

// download appends the model to part and returns the expected total size, 0
// when the server did not say. Every error path leaves part on disk.
func download(
	ctx context.Context,
	m Model,
	opts InstallOptions,
	part string,
	rep *progressReporter,
) (int64, error) {
	offset := fileSize(part)
	url := opts.URL
	if url == "" {
		url = m.URL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("download failed: %w", err)
	}
	if offset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}
	client := opts.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	total := int64(0)
	switch {
	case resp.StatusCode == http.StatusPartialContent && offset > 0:
		// The server honored the Range: keep what we have and append.
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
		if resp.ContentLength > 0 {
			total = offset + resp.ContentLength
		}
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent:
		// No resume: the body is the whole file, so the part restarts.
		offset = 0
		if resp.ContentLength > 0 {
			total = resp.ContentLength
		}
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && offset > 0:
		// Nothing left to fetch: the part is already the whole file, and
		// verification decides whether it is a model.
		rep.report(offset, 0)
		return 0, nil
	default:
		return 0, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	//nolint:gosec // The part file is a public model, not a secret.
	f, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return 0, fmt.Errorf("cannot write to the models directory: %w", err)
	}
	done, err := copyBody(ctx, f, resp.Body, offset, total, rep)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		return 0, fmt.Errorf("cannot write to the models directory: %w", closeErr)
	}
	if err != nil {
		return 0, err
	}
	rep.report(done, total)
	return total, nil
}

// copyBody streams the response into f, reporting progress and stopping on a
// cancelled context. It returns how many bytes the part file holds.
func copyBody(
	ctx context.Context,
	f io.Writer,
	body io.Reader,
	offset, total int64,
	rep *progressReporter,
) (int64, error) {
	done := offset
	rep.report(done, total)
	buf := make([]byte, downloadChunk)
	for {
		if err := ctx.Err(); err != nil {
			return done, fmt.Errorf("download cancelled — /voice install resumes it: %w", err)
		}
		n, readErr := body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return done, fmt.Errorf("cannot write to the models directory: %w", err)
			}
			done += int64(n)
			rep.report(done, total)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			if err := ctx.Err(); err != nil {
				return done, fmt.Errorf("download cancelled — /voice install resumes it: %w", err)
			}
			return done, interruptedError(done, total)
		}
	}
	if total > 0 && done < total {
		return done, interruptedError(done, total)
	}
	return done, nil
}

// interruptedError names how far the download got, so the user can see that
// resuming is worth it.
func interruptedError(done, total int64) error {
	if total <= 0 {
		return fmt.Errorf("download interrupted at %s — /voice install resumes it", FormatBytes(done))
	}
	return fmt.Errorf("download interrupted at %s of %s — /voice install resumes it",
		FormatBytes(done), FormatBytes(total))
}

// verify rejects anything that is not a complete ggml model, before the file
// can be renamed to the name the transcriber loads.
func verify(part string, total int64) error {
	badModel := errors.New("downloaded file is not a ggml model — try /voice install again")
	info, err := os.Stat(part)
	if err != nil || info.IsDir() {
		return badModel
	}
	if total > 0 && info.Size() != total {
		return badModel
	}
	if !hasGGMLMagic(part) {
		return badModel
	}
	return nil
}

// progressReporter throttles the caller's progress callback.
type progressReporter struct {
	fn       func(InstallProgress)
	name     string
	started  bool
	lastDone int64
	last     time.Time
}

// report emits progress unless the last one was less than progressInterval
// ago. The first report of a download and the one that completes it always
// emit, and a repeat of the last numbers never does, so the caller sees one
// final report and no duplicate.
func (p *progressReporter) report(done, total int64) {
	if p == nil || p.fn == nil {
		return
	}
	if p.started && done == p.lastDone {
		return
	}
	if p.started && done != total && time.Since(p.last) < progressInterval {
		return
	}
	p.started, p.lastDone, p.last = true, done, time.Now()
	p.fn(InstallProgress{Name: p.name, Done: done, Total: total})
}

// fileSize is the size of path, 0 when it is missing.
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return 0
	}
	return info.Size()
}

// hasGGMLMagic reports whether path starts with the ggml magic.
func hasGGMLMagic(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, len(ggmlMagic))
	if _, err := io.ReadFull(f, head); err != nil {
		return false
	}
	return string(head) == ggmlMagic
}
