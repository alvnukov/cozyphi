package tasks

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helperNote is a note exactly as mcp-ai-helper writes one: yaml.v3
// frontmatter with four-space sequences, a colon-bearing title single-quoted,
// timestamps double-quoted, then the three sections.
const helperNote = `---
id: ct-001
title: 'Release: cut v0.1.0'
status: todo
priority: high
model_level: medium
task_type: chore
tags:
    - release
acceptance_criteria:
    - Tag pushed
    - 'Notes: written'
verification_plan:
    - gh release view v0.1.0
created_at: "2026-09-02T22:52:42.806862Z"
updated_at: "2026-09-02T22:52:42.806862Z"
---

## Body

Cut the release.

**Why.** Downstream waits on the tag.

## Acceptance Criteria

- Tag pushed
- Notes: written

## Verification Plan

1. gh release view v0.1.0
`

func TestParseNoteReadsTheHelperLayout(t *testing.T) {
	task, err := parseNote([]byte(helperNote), "ct-001")
	require.NoError(t, err)

	assert.Equal(t, "ct-001", task.ID)
	assert.Equal(t, "Release: cut v0.1.0", task.Title)
	assert.Equal(t, StatusTodo, task.Status)
	assert.Equal(t, "high", task.Priority)
	assert.Equal(t, "medium", task.ModelLevel)
	assert.Equal(t, "chore", task.Type)
	assert.Equal(t, []string{"release"}, task.Tags)
	assert.Equal(t, []string{"Tag pushed", "Notes: written"}, task.AcceptanceCriteria)
	assert.Equal(t, []string{"gh release view v0.1.0"}, task.VerificationPlan)
	assert.Equal(t, "Cut the release.\n\n**Why.** Downstream waits on the tag.", task.Body)
	assert.Equal(t, time.Date(2026, 9, 2, 22, 52, 42, 806862000, time.UTC), task.CreatedAt)
}

// TestRenderNoteRoundTripsByteForByte pins the compatibility promise: a note
// read and written back is the same file, so a task that changes hands
// between the helper and cozyphi does not churn its diff.
func TestRenderNoteRoundTripsByteForByte(t *testing.T) {
	task, err := parseNote([]byte(helperNote), "ct-001")
	require.NoError(t, err)
	out, err := renderNote(task)
	require.NoError(t, err)
	assert.Equal(t, helperNote, string(out))
}

func TestParseNoteTakesSectionsOverFrontmatterLists(t *testing.T) {
	text := strings.Replace(helperNote, "- Notes: written\n", "- Notes: written\n- Changelog updated\n", 1)
	task, err := parseNote([]byte(text), "ct-001")
	require.NoError(t, err)
	assert.Equal(t, []string{"Tag pushed", "Notes: written", "Changelog updated"}, task.AcceptanceCriteria,
		"the section is what a person edits; the frontmatter copy is the helper's index")
}

func TestParseNoteRepairsAnUnquotedColon(t *testing.T) {
	text := strings.Replace(helperNote, "title: 'Release: cut v0.1.0'", "title: Release: cut v0.1.0", 1)
	task, err := parseNote([]byte(text), "ct-001")
	require.NoError(t, err)
	assert.Equal(t, "Release: cut v0.1.0", task.Title)
}

func TestParseNoteWithoutSectionsKeepsTheTextAsBody(t *testing.T) {
	text := "---\nid: plain\ntitle: Plain note\nstatus: done\n---\n\nJust a paragraph.\n"
	task, err := parseNote([]byte(text), "plain")
	require.NoError(t, err)
	assert.Equal(t, "Just a paragraph.", task.Body)
	assert.Empty(t, task.AcceptanceCriteria)
}

func TestParseNoteRejectsWhatTheHelperRejects(t *testing.T) {
	cases := []struct {
		name string
		text string
		stem string
		want string
	}{
		{"no fence", "id: x\ntitle: y\n", "x", "opening ---"},
		{"unclosed fence", "---\nid: x\ntitle: y\n", "x", "closing ---"},
		{"no title", "---\nid: x\nstatus: todo\n---\n", "x", "title is required"},
		{"bad status", "---\nid: x\ntitle: y\nstatus: soon\n---\n", "x", `invalid status "soon"`},
		{
			"bad priority",
			"---\nid: x\ntitle: y\nstatus: todo\npriority: urgent\n---\n",
			"x",
			`invalid priority "urgent"`,
		},
		{"id mismatch", "---\nid: x\ntitle: y\nstatus: todo\n---\n", "z", "does not match the file name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseNote([]byte(tc.text), tc.stem)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestParseNoteNormalizesEnumsAndTags(t *testing.T) {
	text := "---\nid: x\ntitle: y\nstatus: In-Progress\npriority: HIGH\nmodel_level: very high\ntags:\n    - Tools\n    - ' '\n---\n"
	task, err := parseNote([]byte(text), "x")
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, task.Status)
	assert.Equal(t, "high", task.Priority)
	assert.Equal(t, "very_high", task.ModelLevel)
	assert.Equal(t, []string{"tools"}, task.Tags)
}

func TestCheckBodyRefusesOnlyLevelTwoHeadings(t *testing.T) {
	require.ErrorIs(t, checkBody("text\n\n## Notes\n\nlost"), ErrHeading)
	require.ErrorIs(t, checkBody("  ## indented"), ErrHeading)
	require.NoError(t, checkBody("text\n\n### fine\n\n**Bold label.** also fine\n# top"))
}

func TestParseListReadsBulletsAndNumbers(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c 3. d"}, parseList("- a\n* b\n\n2. c 3. d\n"))
	assert.Empty(t, parseList("  \n"))
}
