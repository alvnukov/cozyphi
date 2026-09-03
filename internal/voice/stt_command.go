package voice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alvnukov/cozyphi/internal/proc"
	"github.com/alvnukov/cozyphi/internal/redact"
)

// commandOutputLimit bounds what a transcription command may print. A minute
// of speech is a few kilobytes; a megabyte is a runaway command.
const commandOutputLimit = 1 << 20

// CommandTranscriber runs a local transcription binary (whisper-cpp by
// default) on a temporary WAV file.
type CommandTranscriber struct {
	argv    []string
	model   string
	timeout time.Duration
	// tempDir overrides the WAV location; empty means the OS temp dir.
	tempDir string
}

// NewCommandTranscriber parses the configured command line once, so a bad
// quoting is reported at construction instead of at the end of a recording.
func NewCommandTranscriber(commandLine, model string, timeout time.Duration) (*CommandTranscriber, error) {
	argv, err := splitArgs(commandLine)
	if err != nil {
		return nil, fmt.Errorf("voice.stt.command is not parseable: %w", err)
	}
	if len(argv) == 0 {
		return nil, errors.New("voice.stt.command is empty")
	}
	if timeout <= 0 {
		timeout = DefaultTimeoutSeconds * time.Second
	}
	return &CommandTranscriber{argv: argv, model: model, timeout: timeout}, nil
}

// Transcribe writes the audio to a private temporary file and runs the
// command. The file is removed before returning, whatever the outcome.
func (t *CommandTranscriber) Transcribe(ctx context.Context, req Request) (Result, error) {
	path, cleanup, err := t.writeWAV(req.WAV)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()

	argv := expandArgs(t.argv, map[string]string{
		"file":  path,
		"model": t.model,
		"lang":  req.Language,
		"hint":  req.Prompt,
	})

	runCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	res, err := proc.Run(runCtx, proc.Spec{Argv: argv}, proc.Limit{Bytes: commandOutputLimit})
	switch {
	case err != nil:
		return Result{}, fmt.Errorf("cannot run %s: %s", argv[0], redact.Redact(err.Error()))
	case ctx.Err() != nil:
		return Result{}, ctx.Err()
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		return Result{}, fmt.Errorf("transcription timed out after %s — raise voice.stt.timeout_seconds", t.timeout)
	case res.ExitCode != 0:
		return Result{}, fmt.Errorf("transcription failed (%s exit %d) — %s; /voice retry keeps the recording",
			filepath.Base(argv[0]), res.ExitCode, firstLine(redact.Redact(res.Output)))
	}
	return Result{Text: NormalizeTranscript(res.Output)}, nil
}

// writeWAV puts the audio in a 0600 file the command can read.
func (t *CommandTranscriber) writeWAV(wav []byte) (path string, cleanup func(), err error) {
	dir := t.tempDir
	if dir == "" {
		dir = os.TempDir()
	}
	f, err := os.CreateTemp(dir, "cozyphi-voice-*.wav")
	if err != nil {
		return "", nil, fmt.Errorf("cannot create a temporary WAV file: %w", err)
	}
	name := f.Name()
	cleanup = func() { _ = os.Remove(name) }
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, fmt.Errorf("cannot secure %s: %w", name, err)
	}
	if _, err := f.Write(wav); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, fmt.Errorf("cannot write %s: %w", name, err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("cannot close %s: %w", name, err)
	}
	return name, cleanup, nil
}
