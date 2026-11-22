package main

import "github.com/alvnukov/cozyphi/internal/project"

func codeModeEnabled(cfg *project.Config, flag string) bool {
	switch flag {
	case "--code-mode":
		return true
	case "--direct-tools":
		return false
	default:
		return cfg != nil && cfg.CodeMode.Enabled
	}
}
