// Package toolmanager provides functionality for downloading and managing
// external tools (e.g., ripgrep, fd) from GitHub releases.
package toolmanager

import (
	"fmt"
	"runtime"
)

// Platform constants for cross-platform tool downloads.
const (
	PlatformDarwin = "darwin"
	PlatformLinux  = "linux"
	PlatformWin32  = "win32"
)

// Architecture constants for tool downloads.
const (
	ArchAMD64 = "amd64"
	ArchARM64 = "arm64"
	ArchX86   = "x86"
)

var defaultArchMap = map[string]map[string]string{
	PlatformDarwin: {
		ArchARM64: "aarch64",
		ArchAMD64: "x86_64",
	},
	PlatformLinux: {
		ArchARM64: "aarch64",
		ArchAMD64: "x86_64",
	},
	PlatformWin32: {
		ArchARM64: "aarch64",
		ArchAMD64: "x86_64",
	},
}

// ToolConfig defines the configuration for a downloadable tool.
type ToolConfig struct {
	// Name is the display name of the tool.
	Name string
	// Repo is the GitHub repository in "owner/repo" format.
	Repo string
	// BinaryName is the name of the executable binary.
	BinaryName string
	// TagPrefix is the prefix used in version tags (e.g., "v" for "v1.0.0").
	TagPrefix string
	// GetAssetName returns the asset filename for the given version.
	// Platform and architecture are detected automatically from current runtime.
	GetAssetName func(version string) string
}

// Tools is a registry of downloadable tool configurations.
// Each entry maps a tool identifier to its ToolConfig.
var Tools = map[string]ToolConfig{
	"fd": {
		Name:       "fd",
		Repo:       "sharkdp/fd",
		BinaryName: "fd",
		TagPrefix:  "v",
		GetAssetName: AssetName{
			toolName:      "fd",
			versionPrefix: "v",
			archMap:       defaultArchMap,
			darwinSuffix:  "-apple-darwin.tar.gz",
			linuxSuffix:   "-unknown-linux-gnu.tar.gz",
			winSuffix:     "-pc-windows-msvc.zip",
		}.GetAssetName,
	},
	"rg": {
		Name:       "ripgrep",
		Repo:       "BurntSushi/ripgrep",
		BinaryName: "rg",
		TagPrefix:  "",
		GetAssetName: AssetName{
			toolName:      "ripgrep",
			versionPrefix: "",
			archMap:       defaultArchMap,
			darwinSuffix:  "-apple-darwin.tar.gz",
			linuxSuffix:   "-unknown-linux-gnu.tar.gz",
			winSuffix:     "-pc-windows-msvc.zip",
		}.GetAssetName,
	},
}

// archMapping maps architecture constants to platform-specific arch names.
type archMapping map[string]map[string]string

// AssetName builds the full asset filename for a tool release.
type AssetName struct {
	toolName      string
	versionPrefix string
	archMap       archMapping
	darwinSuffix  string
	linuxSuffix   string
	winSuffix     string
}

func normalizePlatform(goos string) string {
	switch goos {
	case "darwin":
		return PlatformDarwin
	case "linux":
		return PlatformLinux
	case "windows":
		return PlatformWin32
	default:
		return ""
	}
}

func normalizeArch(goarch string) string {
	switch goarch {
	case "amd64":
		return ArchAMD64
	case "arm64":
		return ArchARM64
	case "386":
		return ArchX86
	default:
		return ""
	}
}

var (
	platform = normalizePlatform(runtime.GOOS)
	arch     = normalizeArch(runtime.GOARCH)
)

// GetAssetName returns the full asset filename for the given version,
// based on the current platform and architecture.
func (a AssetName) GetAssetName(version string) string {
	if platform == "" || arch == "" {
		return ""
	}

	platformArchs, ok := a.archMap[platform]
	if !ok {
		return ""
	}
	archName, ok := platformArchs[arch]
	if !ok {
		return ""
	}

	fullVersion := version
	if a.versionPrefix != "" {
		fullVersion = a.versionPrefix + version
	}

	switch platform {
	case PlatformDarwin:
		return fmt.Sprintf("%s-%s-%s%s", a.toolName, fullVersion, archName, a.darwinSuffix)
	case PlatformLinux:
		return fmt.Sprintf("%s-%s-%s%s", a.toolName, fullVersion, archName, a.linuxSuffix)
	case PlatformWin32:
		return fmt.Sprintf("%s-%s-%s%s", a.toolName, fullVersion, archName, a.winSuffix)
	default:
		return ""
	}
}
