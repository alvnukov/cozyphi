package voice

import (
	"context"
	"regexp"
	"strings"
)

// Request is one transcription job. WAV is a complete 16 kHz mono PCM file,
// Language is the configured language (possibly "auto") and Prompt is the
// vocabulary hint, which may be empty.
type Request struct {
	WAV      []byte
	Language string
	Prompt   string
}

// Result is what a backend heard. Language is filled in only by backends that
// report it.
type Result struct {
	Text     string
	Language string
}

// Transcriber turns recorded audio into text. Implementations must respect the
// context and must never leak a credential into an error.
type Transcriber interface {
	Transcribe(ctx context.Context, req Request) (Result, error)
}

// timestampLine matches a whisper-cli segment prefix like
// "[00:00:00.000 --> 00:00:02.000]  ". Defensive: -nt already suppresses it,
// but a user command may drop the flag.
var timestampLine = regexp.MustCompile(`\[\d{2}:\d{2}:\d{2}[.,]\d{3}\s*-->\s*\d{2}:\d{2}:\d{2}[.,]\d{3}\]`)

// whitespaceRun collapses to a single space, so a transcript never carries a
// newline into the one-line composer.
var whitespaceRun = regexp.MustCompile(`\s+`)

// NormalizeTranscript trims the text, strips whisper timestamps and collapses
// internal whitespace, keeping sentence punctuation intact.
func NormalizeTranscript(s string) string {
	s = timestampLine.ReplaceAllString(s, " ")
	s = whitespaceRun.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
