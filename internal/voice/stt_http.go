package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/redact"
	"github.com/alvnukov/cozyphi/internal/util"
)

// httpErrorBodyLimit bounds how much of a failing response we read for the
// error message.
const httpErrorBodyLimit = 4 << 10

// HTTPTranscriber posts the audio to an OpenAI-compatible
// /audio/transcriptions endpoint.
type HTTPTranscriber struct {
	baseURL string
	model   string
	timeout time.Duration
	client  *http.Client
	// apiKey never leaves this struct: it is not printed, not logged, and
	// scrubbed out of every error this type returns.
	apiKey string
}

// NewHTTPTranscriber builds the transcriber. The key is taken by value and
// kept unexported on purpose.
func NewHTTPTranscriber(baseURL, model, apiKey string, timeout time.Duration) (*HTTPTranscriber, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("voice.stt.base_url is empty")
	}
	if timeout <= 0 {
		timeout = DefaultTimeoutSeconds * time.Second
	}
	return &HTTPTranscriber{
		baseURL: baseURL,
		model:   model,
		timeout: timeout,
		client:  util.DefaultHTTPClient(),
		apiKey:  apiKey,
	}, nil
}

// Transcribe uploads the WAV and returns the recognized text.
func (t *HTTPTranscriber) Transcribe(ctx context.Context, req Request) (Result, error) {
	body, contentType, err := t.multipart(req)
	if err != nil {
		return Result{}, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	url := t.baseURL + "/audio/transcriptions"
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, t.wrap("cannot build the transcription request", err)
	}
	httpReq.Header.Set("Content-Type", contentType)
	if t.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+t.apiKey)
	}

	resp, err := util.DoWithRetry(t.client, httpReq)
	if err != nil {
		return Result{}, t.wrap("cannot reach "+t.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Result{}, t.statusError(resp)
	}

	var decoded struct {
		Text     string `json:"text"`
		Language string `json:"language"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, httpErrorBodyLimit<<6))
	if err != nil {
		return Result{}, t.wrap("cannot read the transcription response", err)
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return Result{}, fmt.Errorf("transcription returned an unreadable response — %s",
			t.scrub(firstLine(string(raw))))
	}
	return Result{Text: NormalizeTranscript(decoded.Text), Language: decoded.Language}, nil
}

// multipart builds the upload body: the WAV plus the usual OpenAI fields.
func (t *HTTPTranscriber) multipart(req Request) (body []byte, contentType string, err error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	part, err := w.CreateFormFile("file", "voice.wav")
	if err != nil {
		return nil, "", t.wrap("cannot build the upload", err)
	}
	if _, err := part.Write(req.WAV); err != nil {
		return nil, "", t.wrap("cannot build the upload", err)
	}

	fields := [][2]string{
		{"model", t.model},
		{"response_format", "json"},
	}
	// "auto" is our own spelling: the API detects the language when the field
	// is absent.
	if req.Language != "" && req.Language != DefaultLanguage {
		fields = append(fields, [2]string{"language", req.Language})
	}
	if req.Prompt != "" {
		fields = append(fields, [2]string{"prompt", req.Prompt})
	}
	for _, f := range fields {
		if f[1] == "" {
			continue
		}
		if err := w.WriteField(f[0], f[1]); err != nil {
			return nil, "", t.wrap("cannot build the upload", err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", t.wrap("cannot build the upload", err)
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

// statusError turns a non-200 into a one-sentence message that names the step
// and the next action. Some providers echo the key back in the body, so the
// body is scrubbed before it is shown.
func (t *HTTPTranscriber) statusError(resp *http.Response) error {
	detail, _ := io.ReadAll(io.LimitReader(resp.Body, httpErrorBodyLimit))
	msg := fmt.Sprintf("transcription failed (HTTP %d)", resp.StatusCode)
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		msg += " — check voice.stt.api_key; /voice retry keeps the recording"
	default:
		msg += " — /voice retry keeps the recording"
	}
	if body := firstLine(strings.TrimSpace(string(detail))); body != "" {
		msg += " (" + t.scrub(body) + ")"
	}
	return errors.New(msg)
}

// wrap prefixes an underlying failure and scrubs it.
func (t *HTTPTranscriber) wrap(what string, err error) error {
	return fmt.Errorf("%s — %s", what, t.scrub(err.Error()))
}

// scrub removes the key this transcriber holds, then applies the shared
// redaction rules. The literal replacement matters: redact only knows a few
// key shapes, and a self-hosted endpoint may use any.
func (t *HTTPTranscriber) scrub(s string) string {
	if t.apiKey != "" {
		s = strings.ReplaceAll(s, t.apiKey, redact.Marker)
	}
	return redact.Redact(s)
}
