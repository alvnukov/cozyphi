//go:build !windows

package voice

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeTranscriptStripsTimestampsAndCollapsesWhitespace(t *testing.T) {
	in := "[00:00:00.000 -->  00:00:02.400]   Hello,   world.\n[00:00:02.400 --> 00:00:03.000]  Again!\n"
	assert.Equal(t, "Hello, world. Again!", NormalizeTranscript(in))
	assert.Empty(t, NormalizeTranscript("   \n\t "))
}

func TestCommandTranscriberReturnsTheNormalizedOutput(t *testing.T) {
	tr, err := NewCommandTranscriber(
		`sh -c 'echo "[00:00:00.000 --> 00:00:01.000]   Hello   world  "' sh {file}`,
		"",
		time.Minute,
	)
	require.NoError(t, err)

	got, err := tr.Transcribe(t.Context(), Request{WAV: EncodeWAV([]int16{1, 2, 3}, SampleRate)})
	require.NoError(t, err)
	assert.Equal(t, "Hello world", got.Text)
}

func TestCommandTranscriberPassesTheAudioAndRemovesTheTempFile(t *testing.T) {
	dir := t.TempDir()
	copyPath := filepath.Join(dir, "copy.wav")
	pathPath := filepath.Join(dir, "path")

	line := `sh -c 'cp "$1" ` + copyPath + `; echo "$1" > ` + pathPath + `; echo ok' sh {file}`
	tr, err := NewCommandTranscriber(line, "", time.Minute)
	require.NoError(t, err)

	wav := EncodeWAV([]int16{4, 5, 6, 7}, SampleRate)
	_, err = tr.Transcribe(t.Context(), Request{WAV: wav})
	require.NoError(t, err)

	got, err := os.ReadFile(copyPath) //nolint:gosec // G304: the path is built by the test
	require.NoError(t, err)
	assert.Equal(t, wav, got)

	raw, err := os.ReadFile(pathPath) //nolint:gosec // G304: the path is built by the test
	require.NoError(t, err)
	temp := string(raw[:len(raw)-1])
	assert.FileExists(t, copyPath)
	_, statErr := os.Stat(temp)
	assert.True(t, os.IsNotExist(statErr), "the temporary WAV is removed after transcription")
}

func TestCommandTranscriberExpandsModelLanguageAndHint(t *testing.T) {
	out := filepath.Join(t.TempDir(), "argv")
	line := `sh -c 'printf "%s|" "$@" > ` + out + `; echo text' sh {model} -l {lang} --prompt {hint}`
	tr, err := NewCommandTranscriber(line, "ggml-base.bin", time.Minute)
	require.NoError(t, err)

	_, err = tr.Transcribe(t.Context(), Request{WAV: nil, Language: "ru", Prompt: "cozyphi, worktree"})
	require.NoError(t, err)
	raw, err := os.ReadFile(out) //nolint:gosec // G304: the path is built by the test
	require.NoError(t, err)
	assert.Equal(t, "ggml-base.bin|-l|ru|--prompt|cozyphi, worktree|", string(raw))
}

func TestCommandTranscriberDropsAnEmptyHintWithItsFlag(t *testing.T) {
	out := filepath.Join(t.TempDir(), "argv")
	line := `sh -c 'printf "%s|" "$@" > ` + out + `; echo text' sh -m {model} --prompt {hint}`
	tr, err := NewCommandTranscriber(line, "ggml-base.bin", time.Minute)
	require.NoError(t, err)

	_, err = tr.Transcribe(t.Context(), Request{})
	require.NoError(t, err)
	raw, err := os.ReadFile(out) //nolint:gosec // G304: the path is built by the test
	require.NoError(t, err)
	assert.Equal(t, "-m|ggml-base.bin|", string(raw))
}

func TestCommandTranscriberReportsANonZeroExit(t *testing.T) {
	tr, err := NewCommandTranscriber(`sh -c 'echo "model load failed"; exit 3' sh {file}`, "", time.Minute)
	require.NoError(t, err)

	_, err = tr.Transcribe(t.Context(), Request{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transcription failed (sh exit 3)")
	assert.Contains(t, err.Error(), "model load failed")
	assert.Contains(t, err.Error(), "/voice retry keeps the recording")
}

func TestCommandTranscriberReportsATimeout(t *testing.T) {
	tr, err := NewCommandTranscriber(`sh -c 'sleep 30' sh {file}`, "", 100*time.Millisecond)
	require.NoError(t, err)

	_, err = tr.Transcribe(t.Context(), Request{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transcription timed out after 100ms")
	assert.Contains(t, err.Error(), "raise voice.stt.timeout_seconds")
}

func TestCommandTranscriberReportsAMissingBinary(t *testing.T) {
	tr, err := NewCommandTranscriber("cozyphi-no-such-binary {file}", "", time.Minute)
	require.NoError(t, err)

	_, err = tr.Transcribe(t.Context(), Request{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot run cozyphi-no-such-binary")
}

func TestNewCommandTranscriberRejectsABadCommandLine(t *testing.T) {
	_, err := NewCommandTranscriber(`whisper-cli -m "unbalanced`, "", time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "voice.stt.command is not parseable")

	_, err = NewCommandTranscriber("   ", "", time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "voice.stt.command is empty")
}
