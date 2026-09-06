package update

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"

	"github.com/alvnukov/cozyphi/internal/util/githubrelease"
)

// sigstoreIssuer is the OIDC issuer GitHub Actions presents to Fulcio when the
// release workflow signs. It is part of the identity cosign checks.
const sigstoreIssuer = "https://token.actions.githubusercontent.com"

// releaseWorkflow signs the release assets, so the cosign certificate carries
// it as the signer identity. Keep in sync with .github/workflows/release.yml.
const releaseWorkflow = ".github/workflows/release.yml"

// signatureDeps are the outside-world calls the signature check makes. Tests
// swap them for stubs.
type signatureDeps struct {
	lookPath func(file string) (string, error)
	download func(ctx context.Context, url, dest string) error
	run      func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func defaultSignatureDeps() signatureDeps {
	return signatureDeps{
		lookPath: exec.LookPath,
		download: githubrelease.DownloadFile,
		run:      runCommand,
	}
}

// certIdentityRegexp builds the signer identity cosign must find in the
// certificate. The match is case-insensitive: GitHub spells the repository in
// the certificate the way the repository is named, and Repo may differ in case.
func certIdentityRegexp(repo string) string {
	return `(?i)^https://github\.com/` + regexp.QuoteMeta(repo) + `/` +
		regexp.QuoteMeta(releaseWorkflow) + `@`
}

// verifyChecksumSignature checks the cosign signature over the checksums file.
// Trusting that one file is enough, because every archive is verified against
// its contents.
//
// Two cases fall back to the checksum alone, each with a notice: cosign is not
// installed, and the release carries no signature (releases published before
// signing existed). A signature that is present but does not verify stops the
// update.
func verifyChecksumSignature(
	ctx context.Context,
	out io.Writer,
	deps signatureDeps,
	baseURL, sumsName, sumsPath string,
) error {
	cosign, err := deps.lookPath("cosign")
	if err != nil {
		printf(out, "cozyphi update: cosign not installed, checking the checksum only\n")
		return nil
	}

	sigPath := sumsPath + ".sig"
	certPath := sumsPath + ".pem"
	if err := deps.download(ctx, baseURL+"/"+sumsName+".sig", sigPath); err != nil {
		printf(out, "cozyphi update: release carries no signature, checking the checksum only\n")
		return nil
	}
	if err := deps.download(ctx, baseURL+"/"+sumsName+".pem", certPath); err != nil {
		printf(out, "cozyphi update: release carries no certificate, checking the checksum only\n")
		return nil
	}

	printf(out, "cozyphi update: verifying signature...\n")
	output, err := deps.run(ctx, cosign, "verify-blob",
		"--certificate", certPath,
		"--signature", sigPath,
		"--certificate-identity-regexp", certIdentityRegexp(Repo),
		"--certificate-oidc-issuer", sigstoreIssuer,
		sumsPath,
	)
	if err != nil {
		return fmt.Errorf(
			"signature check failed for %s: %w: %s\n"+
				"the release does not match the %s release workflow",
			sumsName, err, strings.TrimSpace(string(output)), Repo,
		)
	}
	printf(out, "cozyphi update: signature verified\n")
	return nil
}
