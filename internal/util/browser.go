package util

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

// OpenBrowser opens a validated HTTP(S) URL in the user's default browser.
func OpenBrowser(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("browser: invalid HTTP(S) URL")
	}

	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{rawURL}
	case "linux":
		command, args = "xdg-open", []string{rawURL}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		return fmt.Errorf("browser: unsupported platform %q", runtime.GOOS)
	}

	cmd := exec.CommandContext(ctx, command, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("browser: start %s: %w", command, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
