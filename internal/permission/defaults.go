package permission

var defaultBashAllow = []string{
	`^git (status|diff|log|show|branch|rev-parse|describe)\b`,
	`^go (test|build|vet|fmt|mod|list|env|version)\b`,
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
