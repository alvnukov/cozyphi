package funclib

import (
	"context"
	"errors"
	"strings"
)

// Versions lists immutable release tags for one package. Source and test files
// are loaded only when the caller selects a specific release.
func (s *Store) Versions(ctx context.Context, name string) ([]Pin, error) {
	if !nameOK(name) {
		return nil, errors.New("invalid package name")
	}
	exists, err := s.exists()
	if err != nil || !exists {
		return []Pin{}, err
	}
	prefix := "refs/tags/" + name + "/"
	raw, err := s.git(
		ctx,
		"",
		"for-each-ref",
		"--sort=-version:refname",
		"--count=4097",
		"--format=%(objectname) %(refname)",
		prefix,
	)
	if err != nil {
		return nil, err
	}
	versions := make([]Pin, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		digest, ref, ok := strings.Cut(line, " ")
		version, match := strings.CutPrefix(ref, prefix+"v")
		if !ok || !match || !validOID.MatchString(digest) || !versionOK(version) {
			return nil, errors.New("invalid package release tag")
		}
		versions = append(versions, Pin{Version: version, Digest: digest})
	}
	if len(versions) > maxPackages {
		return nil, errors.New("package release count exceeds the catalog limit")
	}
	return versions, nil
}
