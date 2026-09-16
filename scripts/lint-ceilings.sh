#!/usr/bin/env bash
# Reprint the complexity ceilings .golangci.yml pins, and the function that sets
# each one.
#
# The ceilings are a ratchet: every number is this repository's current worst,
# written down so that nothing may get worse. That only works if lowering one is
# a chore anyone can do, so this script measures them again instead of leaving
# the numbers to archaeology. Run it after decomposing something and copy what
# it prints into .golangci.yml and internal/arch/boundaries_test.go.
#
# It reports: it never edits a file, and a number being over its ceiling is the
# answer rather than a failure.
set -euo pipefail

if ! command -v golangci-lint >/dev/null; then
	echo "golangci-lint is not installed; run: make lint-install" >&2
	exit 1
fi

cd "$(dirname "$0")/.."
module=$(go list -m)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Each linter is asked about every function rather than only the ones over a
# threshold. uniq-by-line — on by default — has to go: it keeps one finding per
# source line, and all of these linters report the line a function opens on, so
# with it the longest function in the repository stays invisible behind whichever
# linter names it first.
write_config() {
	cat >"$1" <<YML
version: "2"
run:
  issues-exit-code: 0
  tests: true
  relative-path-mode: gomod
linters:
  default: none
  enable: [$2]
  settings:
    funlen: {lines: $3, statements: $4, ignore-comments: true}
    gocognit: {min-complexity: 1}
    cyclop: {max-complexity: 1, package-average: 0.1}
    nestif: {min-complexity: 1}
    interfacebloat: {max: 1}
  exclusions:
    generated: lax
issues:
  uniq-by-line: false
  max-issues-per-linter: 0
  max-same-issues: 0
YML
}
# funlen stops at the first thing wrong with a function, so one over the line
# budget never reports its statement count. Statements get their own pass.
write_config "$work/all.yml" "funlen, gocognit, cyclop, nestif, interfacebloat" 1 1000000
write_config "$work/statements.yml" "funlen" 1000000 1

echo "measuring — two lint passes over the whole module, a few minutes…" >&2
golangci-lint run --config "$work/all.yml" ./... >"$work/out" 2>&1
golangci-lint run --config "$work/statements.yml" ./... >>"$work/out" 2>&1

# A report turns the findings it recognizes into "<value> <where>" lines and
# keeps the largest. The sed expression is the only part that knows the shape of
# a linter's message.
report() {
	local label=$1 expr=$2 top value
	top=$(sed -nE "$expr" "$work/out" | sort -rn | awk 'NR == 1')
	if [ -z "$top" ]; then
		printf '%-22s %s\n' "$label" "(nothing reported)"
		return
	fi
	value=$(echo "${top%% *}" | sed -E 's#\.0*$|(\.[0-9]*[1-9])0+$#\1#')
	printf '%-22s %-7s %s\n' "$label" "$value" "${top#* }"
}

echo
printf '%-22s %-7s %s\n' "ceiling" "value" "set by"
report "funlen.lines" 's#^([^:]+):[0-9]+:[0-9]+: Function .+ is too long \(([0-9]+) >.*#\2 \1#p'
report "funlen.statements" 's#^([^:]+):[0-9]+:[0-9]+: Function .+ has too many statements \(([0-9]+) >.*#\2 \1#p'
report "gocognit.min" 's#^([^:]+):[0-9]+:[0-9]+: cognitive complexity ([0-9]+) of func (.+) is high.*#\2 \1 \3#p'
report "cyclop.max" 's#^([^:]+):[0-9]+:[0-9]+: calculated cyclomatic complexity for function (.+) is ([0-9]+),.*#\3 \1 \2#p'
report "nestif.min" 's#^([^:]+):[0-9]+:[0-9]+: .* has complex nested blocks \(complexity: ([0-9]+)\).*#\2 \1#p'
report "interfacebloat.max" 's#^([^:]+):[0-9]+:[0-9]+: the interface has more than [0-9]+ methods: ([0-9]+).*#\2 \1#p'
report "cyclop.package-average" 's#.*average complexity for the package ([^ ]+) is ([0-9.]+).*#\2 \1#p'

# Fan-out is a property of a package rather than of a function, and it is the
# architecture test that holds it, so it is counted off the import graph.
go list -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}' ./... |
	awk -v m="$module/" '{
		n = 0
		for (i = 2; i <= NF; i++) if (index($i, m) == 1) n++
		if (n > best) { best = n; where = substr($1, length(m) + 1) }
	}
	END { printf "%-22s %-7d %s\n", "arch.fanOutCeiling", best, where }'
echo
echo "nestif reports at its threshold, so .golangci.yml pins one above what is printed."
