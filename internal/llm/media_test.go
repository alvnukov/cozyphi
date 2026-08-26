package llm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateMedia(t *testing.T) {
	tests := []struct {
		name     string
		media    Media
		wantMIME string
		wantData string
		wantErr  string
	}{
		{
			name:     "png base64",
			media:    Media{MediaType: "image/png", Data: "AAECAw=="},
			wantMIME: "image/png",
			wantData: "AAECAw==",
		},
		{
			name:     "jpeg base64",
			media:    Media{MediaType: "image/jpeg", Data: "/9j/"},
			wantMIME: "image/jpeg",
			wantData: "/9j/",
		},
		{
			name:     "webp base64",
			media:    Media{MediaType: "image/webp", Data: "UklG"},
			wantMIME: "image/webp",
			wantData: "UklG",
		},
		{
			name:     "mime type is case-insensitive",
			media:    Media{MediaType: "image/PNG", Data: "AAECAw=="},
			wantMIME: "image/png",
			wantData: "AAECAw==",
		},
		{
			name:     "data url is decoded",
			media:    Media{MediaType: "image/png", Data: "data:image/png;base64,AAECAw=="},
			wantMIME: "image/png",
			wantData: "AAECAw==",
		},
		{
			name:    "unsupported svg rejected",
			media:   Media{MediaType: "image/svg+xml", Data: "PHN2Zz4="},
			wantErr: "does not support media type",
		},
		{
			name:    "mismatched data url mime rejected",
			media:   Media{MediaType: "image/png", Data: "data:image/jpeg;base64,/9j/"},
			wantErr: "does not match data URL type",
		},
		{
			name:    "malformed base64 rejected",
			media:   Media{MediaType: "image/png", Data: "not-base64"},
			wantErr: "valid base64",
		},
		{
			name:    "non-canonical base64 rejected",
			media:   Media{MediaType: "image/png", Data: "AB=="},
			wantErr: "canonical base64",
		},
		{
			name:    "empty data rejected",
			media:   Media{MediaType: "image/png", Data: ""},
			wantErr: "valid base64",
		},
		{
			name:    "oversized encoded rejected",
			media:   Media{MediaType: "image/png", Data: strings.Repeat("A", (28*1024*1024)+4)},
			wantErr: "encoded limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateMedia(tt.media)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantMIME, got.MIME)
			require.Equal(t, tt.wantData, got.Base64)
			require.Equal(t, "data:"+tt.wantMIME+";base64,"+tt.wantData, got.DataURL)
		})
	}
}
