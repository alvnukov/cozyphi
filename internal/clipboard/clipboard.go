package clipboard

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Image is an image read from the system clipboard.
type Image struct {
	// Data holds the raw image bytes; clipboard images are PNG-encoded.
	Data []byte
	// MediaType is the image media type (image/png).
	MediaType string
}

// execOutput runs a command and returns its stdout.
func execOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// ReadImage reads an image from the system clipboard. ok is false when the
// clipboard holds no image (rather than an error): a text clipboard is not a
// failure for paste, the caller treats it as "no image to attach".
func ReadImage() (Image, bool, error) {
	return readImage(execOutput, runtime.GOOS)
}

// readImage dispatches to the platform-specific clipboard image reads so the
// logic is testable with a stubbed runner.
func readImage(run func(string, ...string) ([]byte, error), goos string) (Image, bool, error) {
	switch goos {
	case "darwin":
		return readDarwin(run)
	case "windows":
		return readWindows(run)
	case "linux":
		return readLinux(run)
	default:
		return Image{}, false, nil
	}
}

func readDarwin(run func(string, ...string) ([]byte, error)) (Image, bool, error) {
	tmp := filepath.Join(os.TempDir(), "cozyphi-clipboard.png")
	script := fmt.Sprintf(`set imageData to the clipboard as "PNGf"
set fileRef to open for access POSIX file "%s" with write permission
set eof fileRef to 0
write imageData to fileRef
close access fileRef`, tmp)
	if _, err := run("osascript", "-e", script); err != nil {
		return Image{}, false, nil
	}
	data, err := os.ReadFile(tmp)
	_ = os.Remove(tmp)
	if err != nil || len(data) == 0 {
		return Image{}, false, nil
	}
	return Image{Data: data, MediaType: "image/png"}, true, nil
}

func readWindows(run func(string, ...string) ([]byte, error)) (Image, bool, error) {
	script := "Add-Type -AssemblyName System.Windows.Forms; $img = [System.Windows.Forms.Clipboard]::GetImage(); if ($img) { $ms = New-Object System.IO.MemoryStream; $img.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png); [System.Convert]::ToBase64String($ms.ToArray()) }"
	out, err := run("powershell.exe", "-NonInteractive", "-NoProfile", "-Command", script)
	if err != nil {
		return Image{}, false, nil
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil || len(data) == 0 {
		return Image{}, false, nil
	}
	return Image{Data: data, MediaType: "image/png"}, true, nil
}

func readLinux(run func(string, ...string) ([]byte, error)) (Image, bool, error) {
	if out, err := run("wl-paste", "-t", "image/png"); err == nil && len(out) > 0 {
		return Image{Data: out, MediaType: "image/png"}, true, nil
	}
	if out, err := run("xclip", "-selection", "clipboard", "-t", "image/png", "-o"); err == nil && len(out) > 0 {
		return Image{Data: out, MediaType: "image/png"}, true, nil
	}
	return Image{}, false, nil
}
