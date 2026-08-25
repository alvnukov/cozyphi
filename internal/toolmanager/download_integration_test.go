package toolmanager

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alvnukov/cozyphi/internal/util/githubrelease"
)

func TestDownloadToolsFromGitHub(t *testing.T) {
	if os.Getenv("COZYPHI_RUN_NETWORK_TESTS") != "1" {
		t.Skip("set COZYPHI_RUN_NETWORK_TESTS=1 to test real GitHub release downloads")
	}

	for _, test := range []struct {
		tool       string
		versionOut string
	}{
		{tool: "fd", versionOut: "fd"},
		{tool: "rg", versionOut: "ripgrep"},
	} {
		t.Run(test.tool, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)
			if runtime.GOOS == "windows" {
				t.Setenv("USERPROFILE", homeDir)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			path, err := DownloadTool(ctx, test.tool)
			if err != nil {
				t.Fatal(err)
			}

			commandCtx, commandCancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer commandCancel()
			out, err := exec.CommandContext(commandCtx, path, "--version").CombinedOutput()
			if err != nil {
				t.Fatalf("run downloaded %s: %v: %s", test.tool, err, out)
			}
			if !strings.Contains(string(out), test.versionOut) {
				t.Fatalf("unexpected %s version output: %s", test.tool, out)
			}
		})
	}
}

func TestSelectCompatibleFdReleaseFromGitHub(t *testing.T) {
	if os.Getenv("COZYPHI_RUN_NETWORK_TESTS") != "1" {
		t.Skip("set COZYPHI_RUN_NETWORK_TESTS=1 to query real GitHub releases")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	releases, err := githubrelease.FetchRecent(ctx, Tools["fd"].Repo, compatibleReleaseLookback)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := selectCompatibleAsset(
		Tools["fd"],
		releases,
		PlatformDarwin,
		ArchAMD64,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asset.Name, "x86_64-apple-darwin") {
		t.Fatalf("unexpected Intel macOS fd asset: %s", asset.Name)
	}
}
