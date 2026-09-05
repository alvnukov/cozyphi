package session

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// OpenLatestSession acquires the newest available session. Busy sessions are
// skipped; when none are available it returns os.ErrNotExist so callers can
// create a fresh session. Other open errors are reported, not silently skipped.
func OpenLatestSession(dir string) (*Manager, error) {
	list, err := ListSessions(dir)
	if err != nil {
		return nil, err
	}
	for _, meta := range list {
		// Do not trust Active: ownership may have changed since the probe.
		m, err := OpenSession(meta.File)
		if errors.Is(err, ErrBusy) || errors.Is(err, os.ErrNotExist) {
			continue
		}
		return m, err
	}
	return nil, os.ErrNotExist
}

// ReadSessionModel reads only the session header model, without ownership,
// repairs, or writes. It does not inspect later assistant model overrides.
func ReadSessionModel(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64*1024)
	for lineNo := 1; ; lineNo++ {
		line, terminated, err := readEntryLine(r)
		if err != nil {
			return "", fmt.Errorf("session: read header of %s: %w", path, err)
		}
		if strings.TrimSpace(string(line)) == "" {
			if terminated {
				continue
			}
			return "", fmt.Errorf("session: missing header in %s", path)
		}
		entry, err := decodeEntryLine(line, lineNo)
		if err != nil {
			return "", err
		}
		header, ok := entry.(SessionHeader)
		if !ok {
			return "", fmt.Errorf("session: first entry must be session header at %s:%d", path, lineNo)
		}
		return header.Model, nil
	}
}
