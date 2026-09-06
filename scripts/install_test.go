// Package scripts holds tests for the shell installers shipped in this
// directory. There is no Go code here to build, only the checks that keep
// install.sh honest.
package scripts

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const testVersion = "1.2.3"

// installEnv is one hermetic run of scripts/install.sh: a fixture release
// served by a stub curl, a stub cosign with a settable exit code, and a
// throwaway HOME and install dir.
type installEnv struct {
	fixtures   string
	stubs      string
	installDir string
	home       string
	cosignArgs string
}

func newInstallEnv(t *testing.T, withCosign bool) *installEnv {
	t.Helper()
	root := t.TempDir()
	env := &installEnv{
		fixtures:   filepath.Join(root, "release"),
		stubs:      filepath.Join(root, "stubs"),
		installDir: filepath.Join(root, "bin"),
		home:       filepath.Join(root, "home"),
		cosignArgs: filepath.Join(root, "cosign-args.txt"),
	}
	for _, dir := range []string{env.fixtures, env.stubs, env.installDir, env.home} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}

	asset := fmt.Sprintf("cozyphi_%s_%s_%s.tar.gz", testVersion, runtime.GOOS, runtime.GOARCH)
	archive := filepath.Join(env.fixtures, asset)
	writeArchive(t, archive, "cozyphi-binary-payload")

	sums := fmt.Sprintf("%s  %s\n", sha256Of(t, archive), asset)
	sumsName := "checksums_" + testVersion + ".txt"
	writeFile(t, filepath.Join(env.fixtures, sumsName), sums)
	writeFile(t, filepath.Join(env.fixtures, sumsName+".sig"), "stub-signature")
	writeFile(t, filepath.Join(env.fixtures, sumsName+".pem"), "stub-certificate")

	// Stub curl serves the fixture directory by asset name and exits like
	// `curl -f` when the release has no such file.
	writeScript(t, filepath.Join(env.stubs, "curl"), `#!/usr/bin/env bash
dest=""
url=""
while [ $# -gt 0 ]; do
	case "$1" in
	-o) dest="$2"; shift 2 ;;
	-H) shift 2 ;;
	-*) shift ;;
	*) url="$1"; shift ;;
	esac
done
src="${FIXTURES}/${url##*/}"
[ -f "$src" ] || exit 22
if [ -n "$dest" ]; then cp "$src" "$dest"; else cat "$src"; fi
`)

	if withCosign {
		writeScript(t, filepath.Join(env.stubs, "cosign"), `#!/usr/bin/env bash
printf '%s\n' "$@" >"$COSIGN_ARGS"
echo "stub cosign called" >&2
exit "${COSIGN_EXIT:-0}"
`)
	}
	return env
}

// run executes install.sh and returns its combined output.
func (e *installEnv) run(t *testing.T, extraEnv ...string) (string, error) {
	t.Helper()
	script, err := filepath.Abs("install.sh")
	require.NoError(t, err)

	cmd := exec.CommandContext(t.Context(), "bash", script)
	// /usr/local/bin is left out so a cosign installed on the machine cannot
	// stand in for the stub.
	cmd.Env = append([]string{
		"PATH=" + e.stubs + ":/usr/bin:/bin",
		"HOME=" + e.home,
		"FIXTURES=" + e.fixtures,
		"COZYPHI_VERSION=v" + testVersion,
		"COZYPHI_INSTALL_DIR=" + e.installDir,
		"COSIGN_ARGS=" + e.cosignArgs,
	}, extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (e *installEnv) installedBinary() string {
	return filepath.Join(e.installDir, "cozyphi")
}

func TestInstallScriptVerifiesSignature(t *testing.T) {
	skipWithoutBash(t)
	env := newInstallEnv(t, true)

	out, err := env.run(t)

	require.NoError(t, err, "install must succeed on a good signature:\n%s", out)
	require.Contains(t, out, "verifying signature")
	args, readErr := os.ReadFile(env.cosignArgs)
	require.NoError(t, readErr, "cosign must be asked to verify the checksums file")
	require.Contains(t, string(args), "verify-blob")
	require.Contains(t, string(args), "https://token.actions.githubusercontent.com")
	require.Contains(t, string(args), `workflows/release\.yml@`,
		"the identity must pin the release workflow")
	require.Contains(t, string(args), "checksums_"+testVersion+".txt\n",
		"cosign must verify the checksums file itself")
	body, readErr := os.ReadFile(env.installedBinary())
	require.NoError(t, readErr)
	require.Equal(t, "cozyphi-binary-payload", string(body))
}

func TestInstallScriptStopsOnBadSignature(t *testing.T) {
	skipWithoutBash(t)
	env := newInstallEnv(t, true)

	out, err := env.run(t, "COSIGN_EXIT=1")

	require.Error(t, err, "a bad signature must stop the install:\n%s", out)
	require.Contains(t, out, "signature check failed")
	require.NoFileExists(t, env.installedBinary(), "nothing may be installed from an unverified release")
}

func TestInstallScriptFallsBackWithoutCosign(t *testing.T) {
	skipWithoutBash(t)
	env := newInstallEnv(t, false)

	out, err := env.run(t)

	require.NoError(t, err, "install must work on a machine without cosign:\n%s", out)
	require.Contains(t, out, "cosign not installed")
	require.FileExists(t, env.installedBinary())
}

func TestInstallScriptFallsBackOnUnsignedRelease(t *testing.T) {
	skipWithoutBash(t)
	env := newInstallEnv(t, true)
	// A release published before signing existed carries no .sig asset.
	sumsName := "checksums_" + testVersion + ".txt"
	require.NoError(t, os.Remove(filepath.Join(env.fixtures, sumsName+".sig")))

	out, err := env.run(t)

	require.NoError(t, err, "an unsigned release must still install:\n%s", out)
	require.Contains(t, out, "no signature")
	require.FileExists(t, env.installedBinary())
}

func skipWithoutBash(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is the POSIX installer; Windows uses install.ps1")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func writeScript(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o755))
}

func writeArchive(t *testing.T, path, payload string) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "cozyphi",
		Mode: 0o755,
		Size: int64(len(payload)),
	}))
	_, err = tw.Write([]byte(payload))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
}

func sha256Of(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(body)
	return strings.ToLower(hex.EncodeToString(sum[:]))
}
