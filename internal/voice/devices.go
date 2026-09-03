package voice

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/proc"
	"github.com/alvnukov/cozyphi/internal/redact"
)

// deviceListTimeout bounds the probe: listing devices must never hang the UI.
const deviceListTimeout = 5 * time.Second

// avfoundationDevice matches ffmpeg's "[0] MacBook Pro Microphone" rows.
var avfoundationDevice = regexp.MustCompile(`\[(\d+)\]\s+(.+?)\s*$`)

// ListDevices names the microphones the current platform can offer, in the
// spelling voice.capture.device expects.
func ListDevices(ctx context.Context, env ResolveEnv) ([]string, error) {
	goos := env.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	lookBin := env.LookBin
	if lookBin == nil {
		return nil, errors.New("cannot look up binaries")
	}
	switch goos {
	case "darwin":
		return listDarwinDevices(ctx, lookBin)
	case "linux":
		return listLinuxDevices(ctx, lookBin)
	default:
		return nil, fmt.Errorf(
			"listing capture devices is not supported on %s — set voice.capture.device by hand",
			goos,
		)
	}
}

// listDarwinDevices asks ffmpeg's avfoundation input for its device table.
// The probe always "fails" (there is nothing to record), so only the output
// matters.
func listDarwinDevices(ctx context.Context, lookBin func(string) (string, error)) ([]string, error) {
	ffmpeg, err := lookBin("ffmpeg")
	if err != nil {
		return nil, errors.New("ffmpeg is not installed — install it to list capture devices")
	}
	out, err := runProbe(ctx, []string{
		ffmpeg, "-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", "",
	})
	if err != nil {
		return nil, err
	}
	var (
		devices []string
		inAudio bool
	)
	for line := range strings.Lines(out) {
		line = strings.TrimSpace(line)
		switch {
		case strings.Contains(line, "audio devices:"):
			inAudio = true
			continue
		case strings.Contains(line, "video devices:"):
			inAudio = false
			continue
		case !inAudio:
			continue
		}
		// Drop ffmpeg's "[AVFoundation indev @ 0x...] " prefix before matching.
		if i := strings.Index(line, "] "); i >= 0 {
			line = line[i+2:]
		}
		if m := avfoundationDevice.FindStringSubmatch(line); m != nil {
			devices = append(devices, fmt.Sprintf("%s (%s)", m[1], m[2]))
		}
	}
	if len(devices) == 0 {
		return nil, errors.New("ffmpeg listed no audio devices — allow microphone access for your terminal")
	}
	return devices, nil
}

// listLinuxDevices prefers PulseAudio source names and falls back to ALSA.
func listLinuxDevices(ctx context.Context, lookBin func(string) (string, error)) ([]string, error) {
	if pactl, err := lookBin("pactl"); err == nil {
		out, runErr := runProbe(ctx, []string{pactl, "list", "short", "sources"})
		if runErr != nil {
			return nil, runErr
		}
		var devices []string
		for line := range strings.Lines(out) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				devices = append(devices, fields[1])
			}
		}
		if len(devices) > 0 {
			return devices, nil
		}
	}
	arecord, err := lookBin("arecord")
	if err != nil {
		return nil, errors.New("neither pactl nor arecord is installed — install one to list capture devices")
	}
	out, err := runProbe(ctx, []string{arecord, "-l"})
	if err != nil {
		return nil, err
	}
	var devices []string
	for line := range strings.Lines(out) {
		if strings.HasPrefix(strings.TrimSpace(line), "card ") {
			devices = append(devices, strings.TrimSpace(line))
		}
	}
	if len(devices) == 0 {
		return nil, errors.New("no ALSA capture devices found — check that a microphone is connected")
	}
	return devices, nil
}

// runProbe runs a short listing command and returns its combined output. A
// non-zero exit is not an error here: ffmpeg exits 1 after printing the table.
func runProbe(ctx context.Context, argv []string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, deviceListTimeout)
	defer cancel()
	res, err := proc.Run(probeCtx, proc.Spec{Argv: argv}, proc.Limit{Bytes: commandOutputLimit})
	if err != nil {
		return "", fmt.Errorf("cannot run %s: %s", argv[0], redact.Redact(err.Error()))
	}
	if res.Canceled {
		return "", errors.New("listing capture devices timed out")
	}
	return res.Output, nil
}
