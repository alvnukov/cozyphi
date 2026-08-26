package mcp

import (
	"testing"
	"time"
)

func TestTimeoutDuration(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ServerConfig
		want    time.Duration
		wantErr bool
	}{
		{name: "default", cfg: ServerConfig{}, want: defaultTimeout},
		{name: "seconds", cfg: ServerConfig{Timeout: "300s"}, want: 300 * time.Second},
		{name: "minutes", cfg: ServerConfig{Timeout: "5m"}, want: 5 * time.Minute},
		{name: "whitespace", cfg: ServerConfig{Timeout: "  30s  "}, want: 30 * time.Second},
		{name: "zero falls back", cfg: ServerConfig{Timeout: "0s"}, want: defaultTimeout},
		{name: "negative falls back", cfg: ServerConfig{Timeout: "-5s"}, want: defaultTimeout},
		{name: "invalid", cfg: ServerConfig{Timeout: "soon"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cfg.TimeoutDuration()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil (%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
