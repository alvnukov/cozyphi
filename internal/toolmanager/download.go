package toolmanager

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pulseaiclub/phi/internal/util/githubrelease"
)

// BinDir returns the default directory for downloaded tool binaries.
func BinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".phi", "bin"), nil
}

func extractTarGz(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open tar.gz %q failed: %w", archivePath, err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("create gzip reader for %q failed: %w", archivePath, err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry failed: %w", err)
		}

		targetPath := filepath.Join(destDir, header.Name)
		cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)
		cleanTarget := filepath.Clean(targetPath)
		if !strings.HasPrefix(cleanTarget, cleanDest) && cleanTarget != filepath.Clean(destDir) {
			return fmt.Errorf("invalid tar path traversal: %q", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(cleanTarget, 0o755); err != nil {
				return fmt.Errorf("create extracted dir %q failed: %w", cleanTarget, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
				return fmt.Errorf("create parent dir for %q failed: %w", cleanTarget, err)
			}
			outFile, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("create extracted file %q failed: %w", cleanTarget, err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("write extracted file %q failed: %w", cleanTarget, err)
			}
			if err := outFile.Close(); err != nil {
				return fmt.Errorf("close extracted file %q failed: %w", cleanTarget, err)
			}
		}
	}
	return nil
}

func extractZip(archivePath, destDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip %q failed: %w", archivePath, err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		targetPath := filepath.Join(destDir, file.Name)
		cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)
		cleanTarget := filepath.Clean(targetPath)
		if !strings.HasPrefix(cleanTarget, cleanDest) && cleanTarget != filepath.Clean(destDir) {
			return fmt.Errorf("invalid zip path traversal: %q", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(cleanTarget, 0o755); err != nil {
				return fmt.Errorf("create extracted dir %q failed: %w", cleanTarget, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
			return fmt.Errorf("create parent dir for %q failed: %w", cleanTarget, err)
		}

		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %q failed: %w", file.Name, err)
		}
		outFile, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			rc.Close()
			return fmt.Errorf("create extracted file %q failed: %w", cleanTarget, err)
		}
		if _, err := io.Copy(outFile, rc); err != nil {
			rc.Close()
			outFile.Close()
			return fmt.Errorf("write extracted file %q failed: %w", cleanTarget, err)
		}
		if err := rc.Close(); err != nil {
			outFile.Close()
			return fmt.Errorf("close zip entry %q failed: %w", file.Name, err)
		}
		if err := outFile.Close(); err != nil {
			return fmt.Errorf("close extracted file %q failed: %w", cleanTarget, err)
		}
	}
	return nil
}

func findBinaryRecursively(rootDir, binaryFileName string) (string, error) {
	var found string
	err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == binaryFileName {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil && !errors.Is(err, filepath.SkipDir) {
		return "", err
	}
	return found, nil
}

// DownloadTool downloads the specified tool from GitHub releases and installs
// it to the phi bin directory (~/.phi/bin/).
func DownloadTool(ctx context.Context, tool string) (string, error) {
	config, ok := Tools[tool]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", tool)
	}

	if platform == "" || arch == "" {
		return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	version, err := githubrelease.GetLatestVersion(ctx, config.Repo)
	if err != nil {
		return "", err
	}

	assetName := config.GetAssetName(version)
	if assetName == "" {
		return "", fmt.Errorf("unsupported platform: %s/%s", platform, arch)
	}

	downloadURL := fmt.Sprintf(
		"https://github.com/%s/releases/download/%s%s/%s",
		config.Repo,
		config.TagPrefix,
		version,
		assetName,
	)

	toolsDir, err := BinDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return "", fmt.Errorf("create tools directory %q failed: %w", toolsDir, err)
	}

	binaryExt := ""
	if platform == PlatformWin32 {
		binaryExt = ".exe"
	}
	binaryFileName := config.BinaryName + binaryExt
	binaryPath := filepath.Join(toolsDir, binaryFileName)
	archivePath := filepath.Join(toolsDir, assetName)

	if err = githubrelease.DownloadFile(ctx, downloadURL, archivePath); err != nil {
		return "", err
	}

	extractDir, err := os.MkdirTemp(toolsDir, "extract_tmp_"+config.BinaryName+"_")
	if err != nil {
		_ = os.Remove(archivePath)
		return "", fmt.Errorf("create extract directory failed: %w", err)
	}

	defer func() {
		_ = os.Remove(archivePath)
		_ = os.RemoveAll(extractDir)
	}()

	switch {
	case strings.HasSuffix(assetName, ".tar.gz"):
		if err := extractTarGz(archivePath, extractDir); err != nil {
			return "", fmt.Errorf("failed to extract %s: %w", assetName, err)
		}
	case strings.HasSuffix(assetName, ".zip"):
		if err := extractZip(archivePath, extractDir); err != nil {
			return "", fmt.Errorf("failed to extract %s: %w", assetName, err)
		}
	default:
		return "", fmt.Errorf("unsupported archive format: %s", assetName)
	}

	extractedDir := filepath.Join(
		extractDir,
		strings.TrimSuffix(strings.TrimSuffix(assetName, ".tar.gz"), ".zip"),
	)
	candidates := []string{
		filepath.Join(extractedDir, binaryFileName),
		filepath.Join(extractDir, binaryFileName),
	}

	extractedBinary := ""
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			extractedBinary = c
			break
		}
	}
	if extractedBinary == "" {
		var findErr error
		extractedBinary, findErr = findBinaryRecursively(extractDir, binaryFileName)
		if findErr != nil {
			return "", fmt.Errorf("failed to search binary in extracted dir: %w", findErr)
		}
	}
	if extractedBinary == "" {
		return "", fmt.Errorf("binary not found in archive: expected %s under %s", binaryFileName, extractDir)
	}

	_ = os.Remove(binaryPath)
	if err := os.Rename(extractedBinary, binaryPath); err != nil {
		return "", fmt.Errorf("move binary to %q failed: %w", binaryPath, err)
	}
	if platform != PlatformWin32 {
		if err := os.Chmod(binaryPath, 0o755); err != nil {
			return "", fmt.Errorf("chmod binary %q failed: %w", binaryPath, err)
		}
	}

	return binaryPath, nil
}
