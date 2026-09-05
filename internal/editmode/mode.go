// Package editmode names the user-selectable composer editing dialects.
package editmode

import (
	"fmt"
	"strings"
)

type Mode string

const (
	Standard Mode = ""
	Readline Mode = "readline"
	Vim      Mode = "vim"
)

func Parse(name string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "standard":
		return Standard, nil
	case "readline", "bash", "emacs":
		return Readline, nil
	case "vim":
		return Vim, nil
	default:
		return Standard, fmt.Errorf("unknown keymap %q: choose standard, readline or vim", name)
	}
}

func (m Mode) String() string {
	if m == Standard {
		return "standard"
	}
	return string(m)
}
