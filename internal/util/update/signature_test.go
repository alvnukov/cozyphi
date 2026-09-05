package update

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubDeps records what the signature check asked the outside world to do.
type stubDeps struct {
	cosignPath  string
	cosignErr   error
	downloadErr map[string]error
	runOut      []byte
	runErr      error

	downloaded []string
	runArgs    []string
	runCalls   int
}

func (s *stubDeps) deps(t *testing.T) signatureDeps {
	t.Helper()
	return signatureDeps{
		lookPath: func(string) (string, error) {
			if s.cosignErr != nil {
				return "", s.cosignErr
			}
			return s.cosignPath, nil
		},
		download: func(_ context.Context, url, dest string) error {
			s.downloaded = append(s.downloaded, url)
			if err := s.downloadErr[filepath.Ext(url)]; err != nil {
				return err
			}
			return os.WriteFile(dest, []byte("stub"), 0o600)
		},
		run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			s.runCalls++
			s.runArgs = args
			return s.runOut, s.runErr
		},
	}
}

const (
	testBase     = "https://github.com/alvnukov/cozyphi/releases/download/v1.2.3"
	testSumsName = "checksums_1.2.3.txt"
)

func TestVerifyChecksumSignatureSkipsWhenCosignMissing(t *testing.T) {
	stub := &stubDeps{cosignErr: errors.New("exec: \"cosign\": not found")}
	var out bytes.Buffer

	err := verifyChecksumSignature(t.Context(), &out, stub.deps(t),
		testBase, testSumsName, filepath.Join(t.TempDir(), testSumsName))

	require.NoError(t, err)
	require.Zero(t, stub.runCalls, "cosign must not run when it is not installed")
	require.Empty(t, stub.downloaded, "nothing to download without cosign")
	require.Contains(t, out.String(), "cosign not installed")
}

func TestVerifyChecksumSignatureSkipsUnsignedRelease(t *testing.T) {
	stub := &stubDeps{
		cosignPath:  "/usr/local/bin/cosign",
		downloadErr: map[string]error{".sig": errors.New("http 404")},
	}
	var out bytes.Buffer

	err := verifyChecksumSignature(t.Context(), &out, stub.deps(t),
		testBase, testSumsName, filepath.Join(t.TempDir(), testSumsName))

	require.NoError(t, err, "a release published before signing must still update")
	require.Zero(t, stub.runCalls)
	require.Contains(t, out.String(), "no signature")
}

func TestVerifyChecksumSignatureVerifiesAgainstReleaseWorkflow(t *testing.T) {
	stub := &stubDeps{cosignPath: "/usr/local/bin/cosign"}
	var out bytes.Buffer
	dir := t.TempDir()
	sumsPath := filepath.Join(dir, testSumsName)

	err := verifyChecksumSignature(t.Context(), &out, stub.deps(t),
		testBase, testSumsName, sumsPath)

	require.NoError(t, err)
	require.Equal(t, 1, stub.runCalls)
	require.Equal(t, []string{
		"verify-blob",
		"--certificate", sumsPath + ".pem",
		"--signature", sumsPath + ".sig",
		"--certificate-identity-regexp", certIdentityRegexp(Repo),
		"--certificate-oidc-issuer", sigstoreIssuer,
		sumsPath,
	}, stub.runArgs)
	require.Equal(t, []string{
		testBase + "/" + testSumsName + ".sig",
		testBase + "/" + testSumsName + ".pem",
	}, stub.downloaded)
	require.Contains(t, out.String(), "signature verified")
}

func TestVerifyChecksumSignatureStopsOnBadSignature(t *testing.T) {
	stub := &stubDeps{
		cosignPath: "/usr/local/bin/cosign",
		runOut:     []byte("Error: none of the expected identities matched"),
		runErr:     errors.New("exit status 1"),
	}
	var out bytes.Buffer

	err := verifyChecksumSignature(t.Context(), &out, stub.deps(t),
		testBase, testSumsName, filepath.Join(t.TempDir(), testSumsName))

	require.Error(t, err, "a bad signature must stop the update")
	require.Contains(t, err.Error(), testSumsName)
	require.Contains(t, err.Error(), "none of the expected identities matched")
}

func TestCertIdentityRegexpAcceptsOnlyTheReleaseWorkflow(t *testing.T) {
	re, err := regexp.Compile(certIdentityRegexp(Repo))
	require.NoError(t, err)

	// GitHub writes the repository into the certificate as the repository is
	// named, which differs in case from the Repo constant.
	require.True(t, re.MatchString(
		"https://github.com/alvnukov/cozyphi/.github/workflows/release.yml@refs/tags/v1.2.3"))

	for _, identity := range []string{
		"https://github.com/attacker/cozyphi/.github/workflows/release.yml@refs/tags/v1.2.3",
		"https://github.com/alvnukov/CozyPhi/.github/workflows/ci.yml@refs/heads/main",
		"https://github.com/alvnukov/CozyPhi/.github/workflows/release.yml.evil@refs/tags/v1.2.3",
		"https://evil.example/github.com/alvnukov/CozyPhi/.github/workflows/release.yml@refs/tags/v1",
		"spoof-https://github.com/alvnukov/CozyPhi/.github/workflows/release.yml@refs/tags/v1",
	} {
		require.False(t, re.MatchString(identity), "identity must be rejected: %s", identity)
	}
}
