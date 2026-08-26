package clipboard

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadImage(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G'}
	base64PNG := "iVBORg=="

	tests := []struct {
		name     string
		goos     string
		run      func(string, ...string) ([]byte, error)
		wantOK   bool
		wantMIME string
		wantErr  bool
	}{
		{
			name: "linux wayland returns png",
			goos: "linux",
			run: func(name string, args ...string) ([]byte, error) {
				if name == "wl-paste" {
					return png, nil
				}
				return nil, errors.New("no such command")
			},
			wantOK: true, wantMIME: "image/png",
		},
		{
			name: "linux xclip fallback",
			goos: "linux",
			run: func(name string, args ...string) ([]byte, error) {
				if name == "wl-paste" {
					return nil, errors.New("no wayland")
				}
				return png, nil
			},
			wantOK: true, wantMIME: "image/png",
		},
		{
			name: "linux no image",
			goos: "linux",
			run: func(name string, args ...string) ([]byte, error) {
				return nil, errors.New("no clipboard tools")
			},
			wantOK: false,
		},
		{
			name: "windows powershell png",
			goos: "windows",
			run: func(name string, args ...string) ([]byte, error) {
				return []byte(base64PNG), nil
			},
			wantOK: true, wantMIME: "image/png",
		},
		{
			name: "windows no image",
			goos: "windows",
			run: func(name string, args ...string) ([]byte, error) {
				return nil, errors.New("no image")
			},
			wantOK: false,
		},
		{
			name:   "unsupported os",
			goos:   "freebsd",
			run:    func(string, ...string) ([]byte, error) { return nil, nil },
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, ok, err := readImage(tt.run, tt.goos)
			require.NoError(t, err)
			require.Equal(t, tt.wantOK, ok)
			if ok {
				require.Equal(t, tt.wantMIME, img.MediaType)
				require.NotEmpty(t, img.Data)
			}
		})
	}
}
