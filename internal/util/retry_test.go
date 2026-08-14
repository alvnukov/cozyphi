package util

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	future := now.Add(3 * time.Second).Format(http.TimeFormat)
	past := now.Add(-3 * time.Second).Format(http.TimeFormat)
	overflow := strconv.FormatUint(uint64(math.MaxInt64/int64(time.Second))+1, 10)

	tests := []struct {
		name  string
		value string
		want  time.Duration
		valid bool
	}{
		{name: "missing", value: "", want: 0, valid: false},
		{name: "seconds with whitespace", value: " 3 ", want: 3 * time.Second, valid: true},
		{name: "zero seconds", value: "0", want: 0, valid: true},
		{name: "negative seconds", value: "-1", want: 0, valid: false},
		{name: "fractional seconds", value: "1.5", want: 0, valid: false},
		{name: "signed seconds", value: "+3", want: 0, valid: false},
		{name: "seconds overflow", value: overflow, want: 0, valid: false},
		{name: "future date", value: future, want: 3 * time.Second, valid: true},
		{name: "past date", value: past, want: 0, valid: true},
		{name: "invalid date", value: "tomorrow", want: 0, valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, valid := parseRetryAfter(tt.value, now)
			if got != tt.want || valid != tt.valid {
				t.Fatalf("parseRetryAfter(%q) = (%s, %t), want (%s, %t)", tt.value, got, valid, tt.want, tt.valid)
			}
		})
	}
}

func TestRetryDelayFallbackAndHeaderPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		attempt   int
		header    string
		wantDelay time.Duration
	}{
		{name: "first fallback is immediate", attempt: 0, wantDelay: 0},
		{name: "second fallback is two seconds", attempt: 1, wantDelay: 2 * time.Second},
		{name: "zero header overrides fallback", attempt: 1, header: "0", wantDelay: 0},
		{name: "invalid header uses fallback", attempt: 1, header: "1.5", wantDelay: 2 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp *http.Response
			if tt.header != "" {
				resp = &http.Response{Header: http.Header{"Retry-After": []string{tt.header}}}
			}
			if got := retryDelay(resp, tt.attempt); got != tt.wantDelay {
				t.Fatalf("retryDelay(attempt=%d, header=%q) = %s, want %s", tt.attempt, tt.header, got, tt.wantDelay)
			}
		})
	}
}

func TestDoWithRetryRetryAfterBehavior(t *testing.T) {
	pastDate := time.Now().UTC().Add(-time.Minute).Format(http.TimeFormat)
	futureDate := time.Now().UTC().Add(time.Minute).Format(http.TimeFormat)

	type scriptedResponse struct {
		status     int
		retryAfter string
		body       string
	}
	tests := []struct {
		name         string
		responses    []scriptedResponse
		timeout      time.Duration
		wantErr      bool
		wantStatus   int
		wantRequests int32
	}{
		{
			name: "zero header overrides second fallback",
			responses: []scriptedResponse{
				{status: http.StatusServiceUnavailable},
				{status: http.StatusTooManyRequests, retryAfter: "0"},
				{status: http.StatusOK, body: "ok"},
			},
			timeout:      250 * time.Millisecond,
			wantStatus:   http.StatusOK,
			wantRequests: 3,
		},
		{
			name: "past date overrides second fallback",
			responses: []scriptedResponse{
				{status: http.StatusServiceUnavailable},
				{status: http.StatusServiceUnavailable, retryAfter: pastDate},
				{status: http.StatusOK, body: "ok"},
			},
			timeout:      250 * time.Millisecond,
			wantStatus:   http.StatusOK,
			wantRequests: 3,
		},
		{
			name: "seconds header is cancellable",
			responses: []scriptedResponse{
				{status: http.StatusServiceUnavailable, retryAfter: "60"},
			},
			timeout:      50 * time.Millisecond,
			wantErr:      true,
			wantRequests: 1,
		},
		{
			name: "date header is cancellable",
			responses: []scriptedResponse{
				{status: http.StatusServiceUnavailable, retryAfter: futureDate},
			},
			timeout:      50 * time.Millisecond,
			wantErr:      true,
			wantRequests: 1,
		},
		{
			name: "fallback is cancellable after second failure",
			responses: []scriptedResponse{
				{status: http.StatusServiceUnavailable},
				{status: http.StatusServiceUnavailable},
			},
			timeout:      50 * time.Millisecond,
			wantErr:      true,
			wantRequests: 2,
		},
		{
			name: "final retryable response is returned",
			responses: []scriptedResponse{
				{status: http.StatusServiceUnavailable, retryAfter: "0", body: "first"},
				{status: http.StatusServiceUnavailable, retryAfter: "0", body: "second"},
				{status: http.StatusServiceUnavailable, retryAfter: "0", body: "final"},
			},
			timeout:      250 * time.Millisecond,
			wantStatus:   http.StatusServiceUnavailable,
			wantRequests: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := int(requests.Add(1))
				response := scriptedResponse{status: http.StatusOK, body: "unexpected"}
				if n <= len(tt.responses) {
					response = tt.responses[n-1]
				}
				if response.retryAfter != "" {
					w.Header().Set("Retry-After", response.retryAfter)
				}
				w.WriteHeader(response.status)
				_, _ = io.WriteString(w, response.body)
			}))
			defer server.Close()

			ctx, cancel := context.WithTimeout(t.Context(), tt.timeout)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}

			resp, err := DoWithRetry(server.Client(), req)
			if tt.wantErr {
				if err == nil || !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("DoWithRetry error = %v, want context deadline exceeded", err)
				}
				if resp != nil {
					resp.Body.Close()
					t.Fatal("DoWithRetry returned a response with an error")
				}
			} else {
				if err != nil {
					t.Fatalf("DoWithRetry returned error: %v", err)
				}
				if resp == nil {
					t.Fatal("DoWithRetry returned nil response")
				}
				if resp.StatusCode != tt.wantStatus {
					t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
				}
				_ = resp.Body.Close()
			}

			if got := requests.Load(); got != tt.wantRequests {
				t.Fatalf("requests = %d, want %d", got, tt.wantRequests)
			}
		})
	}
}
