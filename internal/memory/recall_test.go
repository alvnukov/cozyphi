package memory

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storeWith(t *testing.T, files map[string]string) *Store {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		write(t, dir, name, content)
	}
	store, err := Open(dir, nil)
	require.NoError(t, err)
	return store
}

const (
	compactionMemory = `---
name: compaction-summary-ux
description: Compaction summaries render as a collapsed transcript row.
metadata:
  type: project
---
The summary row stays collapsed until the user expands it.
`
	permissionsMemory = `---
name: permission-prompts
description: The user wants the permission gate to keep asking for bash.
metadata:
  type: feedback
---
Never widen the bash permission mode without being asked.

**Why:** the gate is the last line of defense.
`
)

func TestReminderSurfacesMatchingMemory(t *testing.T) {
	store := storeWith(t, map[string]string{
		"compaction-summary-ux.md": compactionMemory,
		"permission-prompts.md":    permissionsMemory,
	})

	reminder := store.Turn().Reminder(Query{Prompt: "fix the compaction summary row"})
	assert.Contains(t, reminder, "<system-reminder>")
	assert.Contains(t, reminder, `<memory name="compaction-summary-ux" type="project"`)
	assert.Contains(t, reminder, "stays collapsed")
	assert.NotContains(t, reminder, "permission-prompts")
	assert.Contains(t, reminder, "background context", "the block must say what it is")
}

func TestReminderStaysSilentWithoutAMatch(t *testing.T) {
	store := storeWith(t, map[string]string{"compaction-summary-ux.md": compactionMemory})

	assert.Empty(t, store.Turn().Reminder(Query{Prompt: "rename the splash screen colors"}))
	assert.Empty(t, store.Turn().Reminder(Query{Prompt: ""}))
	assert.Empty(t, store.Turn().Reminder(Query{Prompt: "go on"}), "short and common words alone never match")
}

func TestReminderRepeatsNothingWithinATurn(t *testing.T) {
	store := storeWith(t, map[string]string{"compaction-summary-ux.md": compactionMemory})
	turn := store.Turn()

	first := turn.Reminder(Query{Prompt: "compaction summary"})
	require.NotEmpty(t, first)
	assert.Empty(t, turn.Reminder(Query{Prompt: "compaction summary again"}), "already surfaced this turn")

	assert.NotEmpty(t, store.Turn().Reminder(Query{Prompt: "compaction summary"}), "a new turn starts fresh")
}

func TestReminderRanksAndCapsMatches(t *testing.T) {
	files := map[string]string{"compaction-summary-ux.md": compactionMemory}
	for _, name := range []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"} {
		files[name+"-compaction.md"] = "---\nname: " + name + "-compaction\n" +
			"description: Something about compaction behavior.\nmetadata:\n  type: project\n---\nBody.\n"
	}
	store := storeWith(t, files)

	reminder := store.Turn().Reminder(Query{Prompt: "compaction"})
	assert.Equal(t, maxRecalled, strings.Count(reminder, "<memory name="))
	assert.Contains(t, reminder, `name="alpha-compaction"`, "a name hit outranks a description hit")
}

func TestReminderTruncatesALongMemory(t *testing.T) {
	body := strings.Repeat("x", maxBodyRunes)
	store := storeWith(t, map[string]string{"verbose-note.md": "---\nname: verbose-note\n" +
		"description: A very long note about telemetry.\nmetadata:\n  type: project\n---\n" + body + "\ntelemetry tail\n"})

	reminder := store.Turn().Reminder(Query{Prompt: "what did the verbose note say about telemetry"})
	assert.Contains(t, reminder, "truncated")
	assert.NotContains(t, reminder, "telemetry tail")
}

func TestStripRemindersLeavesTheUsersOwnText(t *testing.T) {
	store := storeWith(t, map[string]string{"compaction-summary-ux.md": compactionMemory})
	prompt := "fix the compaction summary row"
	sent := store.Turn().Reminder(Query{Prompt: prompt}) + "\n\n" + prompt

	assert.Equal(t, prompt, StripReminders(sent))
	assert.Equal(t, prompt, StripReminders(prompt), "a prompt without a block is untouched")

	quoted := "why does this show up: <system-reminder>foo</system-reminder>"
	assert.Equal(t, quoted, StripReminders(quoted), "a block the user typed is their text")
	assert.Empty(t, StripReminders(store.Turn().Reminder(Query{Prompt: prompt})), "a lone block leaves nothing")
}

func TestRecallWeighsRareWordsOverCommonOnes(t *testing.T) {
	files := map[string]string{
		"kerberos-staging.md": "---\nname: kerberos-staging\ndescription: Staging authenticates the " +
			"session with kerberos.\nmetadata:\n  type: project\n---\nThe ticket expires nightly.\n",
	}
	for _, name := range []string{"alpha", "beta", "gamma", "delta", "epsilon"} {
		files[name+"-session.md"] = "---\nname: " + name + "-session\ndescription: A session note about " +
			name + ".\nmetadata:\n  type: project\n---\nSession session session.\n"
	}
	store := storeWith(t, files)

	reminder := store.Turn().Reminder(Query{Prompt: "how does the session authenticate against staging"})

	first := strings.Index(reminder, `name="kerberos-staging"`)
	require.Positive(t, first, "the rare word wins over the one every memory carries")
	for _, name := range []string{"alpha", "beta", "gamma", "delta", "epsilon"} {
		assert.NotContains(t, reminder, `name="`+name+`-session"`, "a common-word match is not a match")
	}
}

func TestRecallMatchesAnInflectedWord(t *testing.T) {
	store := storeWith(t, map[string]string{
		"compaction-row.md": "---\nname: compaction-row\ndescription: Компакция схлопывает строку " +
			"транскрипта.\nmetadata:\n  type: project\n---\nСтрока раскрывается по клику.\n",
	})

	reminder := store.Turn().Reminder(Query{Prompt: "почему компакции ломают строку"})

	assert.Contains(t, reminder, `name="compaction-row"`, "a prefix survives the inflected ending")
}

func TestRecallLiftsAPathTheSessionIsWorkingIn(t *testing.T) {
	store := storeWith(t, map[string]string{
		"gate-exemption.md": "---\nname: gate-exemption\ndescription: internal/permission/gate.go keeps " +
			"the memory directory writable.\nmetadata:\n  type: project\n---\nThe exemption is narrow.\n",
		"splash-colors.md": "---\nname: splash-colors\ndescription: The splash palette lives in the " +
			"components package.\nmetadata:\n  type: project\n---\nIt follows the terminal theme.\n",
	})

	reminder := store.Turn().Reminder(Query{
		Prompt: "why is this failing",
		Recent: []string{`{"path":"internal/permission/gate.go"}`},
	})

	assert.Contains(t, reminder, `name="gate-exemption"`, "the file being worked in names the memory")
	assert.NotContains(t, reminder, `name="splash-colors"`)
}

func TestRecallDropsAWeakMatchNextToAStrongOne(t *testing.T) {
	store := storeWith(t, map[string]string{
		"plan-pane-clear.md": "---\nname: plan-pane-clear\ndescription: The plan pane has a clear " +
			"action.\nmetadata:\n  type: project\n---\nIt resets the revision counter.\n",
		"footer-tokens.md": "---\nname: footer-tokens\ndescription: The footer shows token labels." +
			"\nmetadata:\n  type: project\n---\nThe plan is unrelated here.\n",
	})

	reminder := store.Turn().Reminder(Query{Prompt: "clear the plan pane"})

	assert.Contains(t, reminder, `name="plan-pane-clear"`)
	assert.NotContains(t, reminder, `name="footer-tokens"`, "one strong match beats two vague ones")
}

func TestIndexIsRebuiltOnlyWhenTheDirectoryChanges(t *testing.T) {
	store := storeWith(t, map[string]string{"compaction-summary-ux.md": compactionMemory})

	first := store.index()
	assert.Same(t, first, store.index(), "an unchanged directory is never re-parsed")

	write(t, store.Dir(), "permission-prompts.md", permissionsMemory)
	assert.NotSame(t, first, store.index(), "a new file rebuilds it")
}
