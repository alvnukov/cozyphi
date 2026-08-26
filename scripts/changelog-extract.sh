#!/bin/bash
# Extract a Keep a Changelog version section from CHANGELOG.md.
# Usage: scripts/changelog-extract.sh <version> [changelog-path]
# Version may be "v0.12.0" or "0.12.0". Writes the section body to stdout.

set -euo pipefail

VERSION="${1:?Must provide version (e.g. v0.12.0 or 0.12.0)}"
FILE="${2:-CHANGELOG.md}"

VERSION="${VERSION#v}"

if [[ ! -f "$FILE" ]]; then
	echo "error: changelog file not found: $FILE" >&2
	exit 1
fi

# Match "## [0.12.0] - date" or "## [0.12.0]"; stop before the next version
# heading, the released-section end marker, or footer reference links.
awk -v ver="$VERSION" '
	BEGIN { found = 0 }
	$0 ~ "^## \\[" ver "\\]" {
		found = 1
		next
	}
	found && (/^## \[/ || /^<!-- Released section/ || /^\[[^]]+\]:/) { exit }
	found { print }
	END {
		if (!found) {
			print "error: no CHANGELOG section for version " ver > "/dev/stderr"
			exit 1
		}
	}
' "$FILE"
