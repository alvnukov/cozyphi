#!/bin/bash
# Emit a shields.io endpoint JSON for total coverage from a go cover profile.
# Usage: scripts/coverage-badge.sh <coverprofile> [label]
# Writes one JSON object to stdout; see README's coverage badge, which renders
# this shape via https://img.shields.io/endpoint.

set -euo pipefail

PROFILE="${1:?Must provide a coverage profile (e.g. coverage.out)}"
LABEL="${2:-coverage}"

if [[ ! -f "$PROFILE" ]]; then
	echo "error: coverage profile not found: $PROFILE" >&2
	exit 1
fi

TOTAL="$(go tool cover -func="$PROFILE" | awk '$1 == "total:" { sub(/%$/, "", $3); print $3 }')"
if [[ -z "$TOTAL" ]]; then
	echo "error: no total line in $PROFILE — was it produced by go test -coverprofile?" >&2
	exit 1
fi

# Color steps down with coverage; thresholds are a readability choice.
COLOR=red
if awk -v t="$TOTAL" 'BEGIN { exit !(t >= 40) }'; then COLOR=orange; fi
if awk -v t="$TOTAL" 'BEGIN { exit !(t >= 60) }'; then COLOR=yellow; fi
if awk -v t="$TOTAL" 'BEGIN { exit !(t >= 80) }'; then COLOR=brightgreen; fi

printf '{"schemaVersion":1,"label":"%s","message":"%s%%","color":"%s"}\n' "$LABEL" "$TOTAL" "$COLOR"
