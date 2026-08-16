package writetool

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/util"
)

func TestApplyHashlineEditReplacesSingleLine(t *testing.T) {
	fileContent := "alpha\nbeta\ngamma"
	replacement := "BETA"

	got, err := ApplyHashlineEdit(t.Context(), fileContent, EditInput{
		Edits: []FlatEdit{
			{
				From:    hashlineRef(2, "beta"),
				To:      hashlineRef(2, "beta"),
				Content: &replacement,
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "alpha\nBETA\ngamma", got)
}

func TestApplyHashlineEditUsesInclusiveRange(t *testing.T) {
	fileContent := "alpha\nbeta\ngamma"
	replacement := "combined"

	got, err := ApplyHashlineEdit(t.Context(), fileContent, EditInput{
		Edits: []FlatEdit{
			{
				From:    hashlineRef(2, "beta"),
				To:      hashlineRef(3, "gamma"),
				Content: &replacement,
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "alpha\ncombined", got)
}

func TestApplyHashlineEditDeletesRangeWhenContentIsNil(t *testing.T) {
	fileContent := "alpha\nbeta\ngamma"

	got, err := ApplyHashlineEdit(t.Context(), fileContent, EditInput{
		Edits: []FlatEdit{
			{
				From: hashlineRef(2, "beta"),
				To:   hashlineRef(3, "gamma"),
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "alpha", got)
}

func TestApplyHashlineEditUsesOriginalAnchorsForMultipleEdits(t *testing.T) {
	fileContent := "one\ntwo\nthree\nfour\nfive"
	expanded := "two-a\ntwo-b\ntwo-c"

	got, err := ApplyHashlineEdit(t.Context(), fileContent, EditInput{
		Edits: []FlatEdit{
			{
				From:    hashlineRef(2, "two"),
				To:      hashlineRef(2, "two"),
				Content: &expanded,
			},
			{
				From: hashlineRef(5, "five"),
				To:   hashlineRef(5, "five"),
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "one\ntwo-a\ntwo-b\ntwo-c\nthree\nfour", got)
}

func TestApplyHashlineEditDeduplicatesIdenticalEdits(t *testing.T) {
	fileContent := "alpha\nbeta\ngamma"
	replacement := "beta-a\nbeta-b"
	edit := FlatEdit{
		From:    hashlineRef(2, "beta"),
		To:      hashlineRef(2, "beta"),
		Content: &replacement,
	}

	got, err := ApplyHashlineEdit(t.Context(), fileContent, EditInput{
		Edits: []FlatEdit{edit, edit},
	})

	require.NoError(t, err)
	require.Equal(t, "alpha\nbeta-a\nbeta-b\ngamma", got)
}

func TestApplyHashlineEditRejectsStaleHash(t *testing.T) {
	fileContent := "alpha\nbeta\ngamma"
	actualHash := util.ComputeLineHash("beta")
	staleHash := differentHash(actualHash)
	replacement := "BETA"

	got, err := ApplyHashlineEdit(t.Context(), fileContent, EditInput{
		Edits: []FlatEdit{
			{
				From:    fmt.Sprintf("2#%s", staleHash),
				To:      hashlineRef(2, "beta"),
				Content: &replacement,
			},
		},
	})

	require.Error(t, err)
	require.Empty(t, got)

	var mismatchErr *HashlineMismatchError
	require.ErrorAs(t, err, &mismatchErr)
	require.Len(t, mismatchErr.mismatches, 1)
	require.Equal(t, 2, mismatchErr.mismatches[0].Line)
	require.Equal(t, mismatchErr.mismatches[0].Expected, staleHash)
	require.Equal(t, actualHash, mismatchErr.mismatches[0].Actual)
}

func TestApplyHashlineEditRejectsReversedRange(t *testing.T) {
	fileContent := "alpha\nbeta\ngamma"
	replacement := "unused"

	got, err := ApplyHashlineEdit(t.Context(), fileContent, EditInput{
		Edits: []FlatEdit{
			{
				From:    hashlineRef(3, "gamma"),
				To:      hashlineRef(2, "beta"),
				Content: &replacement,
			},
		},
	})

	require.Error(t, err)
	require.Empty(t, got)
	require.Contains(t, err.Error(), "range start line 3 must be <= end line 2")
}

func TestApplyHashlineEditRejectsOutOfBoundsRange(t *testing.T) {
	fileContent := "alpha\nbeta\ngamma"
	replacement := "unused"

	got, err := ApplyHashlineEdit(t.Context(), fileContent, EditInput{
		Edits: []FlatEdit{
			{
				From:    hashlineRef(1, "alpha"),
				To:      "4#00",
				Content: &replacement,
			},
		},
	})

	require.Error(t, err)
	require.Empty(t, got)
	require.Contains(t, err.Error(), "line range 1-4 is out of bounds")
}

func TestApplyHashlineEditReturnsCanceledContext(t *testing.T) {
	fileContent := "alpha\nbeta\ngamma"
	replacement := "BETA"

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	got, err := ApplyHashlineEdit(ctx, fileContent, EditInput{
		Edits: []FlatEdit{
			{
				From:    hashlineRef(2, "beta"),
				To:      hashlineRef(2, "beta"),
				Content: &replacement,
			},
		},
	})

	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, got)
}

func TestApplyHashlineEditValidatesAllEditsBeforeApplying(t *testing.T) {
	fileContent := "alpha\nbeta\ngamma"
	replacement := "ALPHA"
	actualHash := util.ComputeLineHash("gamma")
	staleHash := differentHash(actualHash)

	got, err := ApplyHashlineEdit(t.Context(), fileContent, EditInput{
		Edits: []FlatEdit{
			{
				From:    hashlineRef(1, "alpha"),
				To:      hashlineRef(1, "alpha"),
				Content: &replacement,
			},
			{
				From:    fmt.Sprintf("3#%s", staleHash),
				To:      hashlineRef(3, "gamma"),
				Content: &replacement,
			},
		},
	})

	require.Error(t, err)
	require.Empty(t, got)
}

func hashlineRef(line int, content string) string {
	return fmt.Sprintf("%d#%s", line, util.ComputeLineHash(content))
}

func differentHash(current string) string {
	if current == "00" {
		return "01"
	}
	return "00"
}
