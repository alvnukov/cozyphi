package tooldef

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveToCwd returns an absolute path: absolute inputs unchanged, relative
// paths joined with the process working directory.
func ResolveToCwd(filePath string) (string, error) {
	if filepath.IsAbs(filePath) {
		return filePath, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}
	return filepath.Join(cwd, filePath), nil
}
