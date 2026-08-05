#!/usr/bin/env bash
#
# Usage: ./scripts/bump.sh <new-version>
#
# Creates an annotated git tag for a new release. Pushing the tag triggers
# .github/workflows/release.yml (GoReleaser); the changelog is generated from
# commit history, so there is no version file to update.
# Example: ./scripts/bump.sh v0.2.0

set -euo pipefail

if [ $# -ne 1 ]; then
    echo "Usage: $0 <new-version>" >&2
    echo "Example: $0 v0.2.0" >&2
    exit 1
fi

NEW_VERSION="$1"

# Validate version format (v<major>.<minor>.<patch>, optionally with a prerelease suffix)
if ! echo "$NEW_VERSION" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$'; then
    echo "Error: version must match 'v<major>.<minor>.<patch>[-prerelease]' (e.g., v0.2.0 or v1.2.0-rc1)" >&2
    exit 1
fi

# Check for uncommitted changes (excluding untracked files)
if ! git diff --quiet HEAD; then
    echo "Error: there are uncommitted changes. Please commit or stash them first." >&2
    exit 1
fi

# Create the tag
git tag -a "$NEW_VERSION" -m "Release $NEW_VERSION"

echo ""
echo "Created tag: $NEW_VERSION"
echo "To push: git push origin main --follow-tags"
