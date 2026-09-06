package permission

var defaultBashAllow = []string{
	`^git (status|diff|log|show|branch|rev-parse|describe)\b`,
	// Read-only go commands stay auto-allowed, and `go list` only flagless:
	// build flags (-export, -toolexec) run the toolchain over repository code.
	// The rest of the go tool executes code (test, build, vet, run, generate),
	// rewrites files (fmt, mod tidy) or the user's go env (env — cutting just
	// -w by regex isn't worth it). Opt back in with e.g. `^go test\b` in
	// permissions.bash.allow.
	`^go version\b`,
	`^go list( [^-][^ ]*)*$`,
	`^ls\b`,
	`^pwd\b`,
	`^echo\b`,
	`^cat\b`,
	`^head\b`,
	`^tail\b`,
	`^wc\b`,
	`^which\b`,
	`^type\b`,
	`^true\b`,
	`^false\b`,
}

var defaultBashDeny = []string{
	`\bsudo\b`,
	`\bsu\b`,
	// Any recursive/force rm (not only rm -rf /).
	`\brm\s+-[a-zA-Z]*[rf][a-zA-Z]*\b`,
	`\brm\s+--recursive\b`,
	`\brm\s+--force\b`,
	`>\s*/etc/`,
	`curl\s+.*\|\s*(ba)?sh`,
	`wget\s+.*\|\s*(ba)?sh`,
	`mkfs\b`,
	`dd\s+if=`,
	`:(){ :\|:& };:`,
}
