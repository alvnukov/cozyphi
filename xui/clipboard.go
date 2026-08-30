package xui

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/pulseaiclub/xui/render"
)

// CopyToClipboard writes text to the system clipboard.
// Prefers OSC 52 when writing to the TTY, then falls back to platform tools
// (pbcopy / wl-copy / xclip / clip).
func (vx *XUI) CopyToClipboard(text string) error {
	if text == "" {
		return fmt.Errorf("xui: empty clipboard text")
	}
	oscOK := false
	if vx != nil {
		if err := vx.copyOSC52(text); err == nil {
			oscOK = true
		}
	}
	if err := copyPlatformClipboard(text); err == nil {
		return nil
	} else if oscOK {
		return nil
	} else {
		return err
	}
}

func (vx *XUI) copyOSC52(text string) error {
	enc := base64.StdEncoding.EncodeToString([]byte(text))
	seq := render.SeqOSC52ClipboardPrefix + enc + render.SeqOSC52ClipboardSuffix
	_, err := vx.WriteRaw([]byte(seq))
	return err
}

func copyPlatformClipboard(text string) error {
	switch runtime.GOOS {
	case "darwin":
		return pipeToCmd(text, "pbcopy")
	case "windows":
		if err := pipeToCmd(text, "clip"); err == nil {
			return nil
		}
		return pipeToCmd(text, "powershell", "-NoProfile", "-Command", "$Input | Set-Clipboard")
	default:
		if lookPath("wl-copy") {
			if err := pipeToCmd(text, "wl-copy"); err == nil {
				return nil
			}
		}
		if lookPath("xclip") {
			return pipeToCmd(text, "xclip", "-selection", "clipboard")
		}
		return fmt.Errorf("xui: no clipboard tool (install wl-clipboard or xclip)")
	}
}

func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func pipeToCmd(text, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return err
	}
	_, werr := stdin.Write([]byte(text))
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		return err
	}
	return werr
}
