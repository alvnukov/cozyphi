#!/usr/bin/env bash
#
# Install the latest phi release from GitHub.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/pulseaiclub/phi/main/scripts/install.sh | bash
#
# Options (env):
#   PHI_VERSION      release tag to install (default: latest), e.g. v0.1.0
#   PHI_INSTALL_DIR  install directory (default: /usr/local/bin, else ~/.local/bin)
#   PHI_REPO         GitHub repo (default: pulseaiclub/phi)
#   GITHUB_TOKEN     optional; raises API rate limits
#
set -euo pipefail

REPO="${PHI_REPO:-pulseaiclub/phi}"
API="https://api.github.com/repos/${REPO}"
DOWNLOAD_BASE="https://github.com/${REPO}/releases/download"

red() { printf '\033[31m%s\033[0m\n' "$*" >&2; }
info() { printf '%s\n' "$*" >&2; }

need_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		red "error: required command not found: $1"
		exit 1
	fi
}

need_cmd curl
need_cmd tar
need_cmd mktemp
need_cmd uname

# ---- platform ----
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os" in
linux | darwin) ;;
mingw* | msys* | cygwin*)
	red "error: use the Windows .zip from GitHub Releases, or install via WSL"
	exit 1
	;;
*)
	red "error: unsupported OS: $os"
	exit 1
	;;
esac
case "$arch" in
x86_64 | amd64) arch="amd64" ;;
arm64 | aarch64) arch="arm64" ;;
*)
	red "error: unsupported CPU arch: $arch"
	exit 1
	;;
esac

# ---- install dir ----
if [ -n "${PHI_INSTALL_DIR:-}" ]; then
	BIN_DIR="$PHI_INSTALL_DIR"
elif [ -w /usr/local/bin ] 2>/dev/null || [ "$(id -u)" -eq 0 ]; then
	BIN_DIR="/usr/local/bin"
elif mkdir -p "${HOME}/.local/bin" 2>/dev/null && [ -w "${HOME}/.local/bin" ]; then
	BIN_DIR="${HOME}/.local/bin"
else
	red "error: no writable install directory (set PHI_INSTALL_DIR)"
	exit 1
fi
mkdir -p "$BIN_DIR"

# ---- resolve version ----
auth_header=()
if [ -n "${GITHUB_TOKEN:-}" ]; then
	auth_header=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
fi

if [ -n "${PHI_VERSION:-}" ]; then
	TAG="$PHI_VERSION"
	case "$TAG" in
	v*) ;;
	*) TAG="v${TAG}" ;;
	esac
else
	info "phi install: querying latest release..."
	json="$(curl -fsSL \
		-H "Accept: application/vnd.github+json" \
		-H "X-GitHub-Api-Version: 2022-11-28" \
		"${auth_header[@]}" \
		"${API}/releases/latest")" || {
		red "error: failed to query ${API}/releases/latest"
		red "hint: publish a release first, or set PHI_VERSION=vX.Y.Z"
		exit 1
	}
	TAG="$(printf '%s' "$json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
	if [ -z "$TAG" ]; then
		red "error: could not parse tag_name from GitHub API response"
		exit 1
	fi
fi

# GoReleaser .Version strips the leading v from the tag.
VERSION="${TAG#v}"
ASSET="phi_${VERSION}_${os}_${arch}.tar.gz"
SUMS="checksums_${VERSION}.txt"
ASSET_URL="${DOWNLOAD_BASE}/${TAG}/${ASSET}"
SUMS_URL="${DOWNLOAD_BASE}/${TAG}/${SUMS}"

info "phi install: ${TAG} (${os}/${arch})"
info "phi install: ${ASSET_URL}"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/phi-install.XXXXXX")"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

# ---- download ----
info "phi install: downloading checksums..."
curl -fsSL "${auth_header[@]}" -o "${TMP}/${SUMS}" "$SUMS_URL"

info "phi install: downloading archive..."
curl -fsSL "${auth_header[@]}" -o "${TMP}/${ASSET}" "$ASSET_URL"

# ---- verify ----
info "phi install: verifying checksum..."
want="$(awk -v f="$ASSET" '$2 == f { print $1; exit }' "${TMP}/${SUMS}")"
if [ -z "$want" ]; then
	red "error: checksum for ${ASSET} not found in ${SUMS}"
	exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
	got="$(sha256sum "${TMP}/${ASSET}" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
	got="$(shasum -a 256 "${TMP}/${ASSET}" | awk '{print $1}')"
else
	red "error: need sha256sum or shasum to verify the download"
	exit 1
fi

if [ "$(printf '%s' "$got" | tr '[:upper:]' '[:lower:]')" != "$(printf '%s' "$want" | tr '[:upper:]' '[:lower:]')" ]; then
	red "error: checksum mismatch for ${ASSET}"
	red "  want: ${want}"
	red "  got:  ${got}"
	exit 1
fi

# ---- extract + install ----
info "phi install: extracting..."
tar -xzf "${TMP}/${ASSET}" -C "$TMP"
if [ ! -f "${TMP}/phi" ]; then
	red "error: archive did not contain a phi binary"
	exit 1
fi
chmod 755 "${TMP}/phi"

DEST="${BIN_DIR}/phi"
info "phi install: installing to ${DEST}"
# Prefer atomic replace when possible.
if mv -f "${TMP}/phi" "$DEST" 2>/dev/null; then
	:
else
	# /usr/local/bin may need sudo
	if command -v sudo >/dev/null 2>&1; then
		sudo mv -f "${TMP}/phi" "$DEST"
		sudo chmod 755 "$DEST"
	else
		red "error: cannot write to ${DEST} (try PHI_INSTALL_DIR=\$HOME/.local/bin)"
		exit 1
	fi
fi

info "phi install: installed ${TAG} -> ${DEST}"
if ! command -v phi >/dev/null 2>&1; then
	info "phi install: note: ${BIN_DIR} is not on PATH; add it to your shell profile"
fi
info ""
info "Next steps:"
info "  1. phi config          # add a model + api_key (opens in browser)"
info "  2. phi                 # start the TUI"
info ""
info "Or set PHI_MODEL and PHI_API_KEY, then run 'phi'."
