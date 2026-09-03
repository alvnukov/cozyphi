package voice

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/redact"
)

// upload is what the fake endpoint saw.
type upload struct {
	path     string
	auth     string
	filename string
	file     []byte
	fields   map[string]string
}

// readUpload parses the multipart request the transcriber sent.
func readUpload(t *testing.T, r *http.Request) upload {
	t.Helper()
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	require.NoError(t, err)

	got := upload{path: r.URL.Path, auth: r.Header.Get("Authorization"), fields: map[string]string{}}
	mr := multipart.NewReader(r.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		body, err := io.ReadAll(part)
		require.NoError(t, err)
		if part.FormName() == "file" {
			got.filename = part.FileName()
			got.file = body
			continue
		}
		got.fields[part.FormName()] = string(body)
	}
	return got
}

func TestHTTPTranscriberUploadsTheAudioAndReturnsTheText(t *testing.T) {
	var seen upload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		seen = readUpload(t, r)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "  Hello   world.\n", "language": "en"})
	}))
	defer srv.Close()

	tr, err := NewHTTPTranscriber(srv.URL+"/v1/", "whisper-1", "voice-secret-key-9f3a", 10*time.Second)
	require.NoError(t, err)

	wav := EncodeWAV([]int16{1, 2, 3, 4}, SampleRate)
	got, err := tr.Transcribe(t.Context(), Request{WAV: wav, Language: "ru", Prompt: "cozyphi, worktree"})
	require.NoError(t, err)

	assert.Equal(t, "Hello world.", got.Text)
	assert.Equal(t, "en", got.Language)
	assert.Equal(t, "/v1/audio/transcriptions", seen.path, "the trailing slash in base_url is trimmed")
	assert.Equal(t, "Bearer voice-secret-key-9f3a", seen.auth)
	assert.Equal(t, "voice.wav", seen.filename)
	assert.Equal(t, wav, seen.file)
	assert.Equal(t, "whisper-1", seen.fields["model"])
	assert.Equal(t, "json", seen.fields["response_format"])
	assert.Equal(t, "ru", seen.fields["language"])
	assert.Equal(t, "cozyphi, worktree", seen.fields["prompt"])
}

func TestHTTPTranscriberOmitsAutoLanguageAndAnEmptyPrompt(t *testing.T) {
	var seen upload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = readUpload(t, r)
		_, _ = io.WriteString(w, `{"text":"ok"}`)
	}))
	defer srv.Close()

	tr, err := NewHTTPTranscriber(srv.URL, "whisper-1", "", 10*time.Second)
	require.NoError(t, err)

	_, err = tr.Transcribe(t.Context(), Request{Language: DefaultLanguage})
	require.NoError(t, err)
	assert.NotContains(t, seen.fields, "language", "auto lets the endpoint detect the language")
	assert.NotContains(t, seen.fields, "prompt")
	assert.Empty(t, seen.auth, "no key means no Authorization header")
}

// TestHTTPTranscriberNeverLeaksTheAPIKeyIntoAnError is the security guard the
// spec asks for: some endpoints echo the credential back in the error body.
func TestHTTPTranscriberNeverLeaksTheAPIKeyIntoAnError(t *testing.T) {
	const key = "voice-secret-key-9f3a"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Incorrect API key provided: `+key+`"}}`)
	}))
	defer srv.Close()

	tr, err := NewHTTPTranscriber(srv.URL, "whisper-1", key, 10*time.Second)
	require.NoError(t, err)

	_, err = tr.Transcribe(t.Context(), Request{WAV: []byte("wav")})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), key, "the API key never reaches an error message")
	assert.Contains(t, err.Error(), redact.Marker)
	assert.Contains(t, err.Error(), "transcription failed (HTTP 401)")
	assert.Contains(t, err.Error(), "check voice.stt.api_key")
	assert.Contains(t, err.Error(), "/voice retry keeps the recording")
}

func TestHTTPTranscriberReportsAPlainRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "audio too short\nsecond line")
	}))
	defer srv.Close()

	tr, err := NewHTTPTranscriber(srv.URL, "whisper-1", "", 10*time.Second)
	require.NoError(t, err)

	_, err = tr.Transcribe(t.Context(), Request{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transcription failed (HTTP 400)")
	assert.Contains(t, err.Error(), "audio too short")
	assert.NotContains(t, err.Error(), "second line", "only the first line is shown")
}

func TestHTTPTranscriberReportsAnUnreadableResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>gateway</html>")
	}))
	defer srv.Close()

	tr, err := NewHTTPTranscriber(srv.URL, "whisper-1", "", 10*time.Second)
	require.NoError(t, err)

	_, err = tr.Transcribe(t.Context(), Request{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transcription returned an unreadable response")
}

func TestHTTPTranscriberReportsAnUnreachableEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL
	srv.Close()

	tr, err := NewHTTPTranscriber(base, "whisper-1", "", 2*time.Second)
	require.NoError(t, err)

	_, err = tr.Transcribe(t.Context(), Request{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reach "+base)
}

func TestNewHTTPTranscriberRequiresABaseURL(t *testing.T) {
	_, err := NewHTTPTranscriber("  ", "whisper-1", "k", time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "voice.stt.base_url is empty")
}

func TestHTTPTranscriberHonorsACanceledContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = io.WriteString(w, `{"text":"late"}`)
	}))
	defer srv.Close()
	defer close(release)

	tr, err := NewHTTPTranscriber(srv.URL, "whisper-1", "", 100*time.Millisecond)
	require.NoError(t, err)

	_, err = tr.Transcribe(t.Context(), Request{})
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "cannot reach "), "a timeout is reported as an unreachable endpoint")
}
