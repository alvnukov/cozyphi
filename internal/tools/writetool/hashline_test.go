package writetool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/util"
)

func TestApplyHashlineEdit(t *testing.T) {
	betaHash := util.ComputeLineHash("beta")
	gammaHash := util.ComputeLineHash("gamma")
	staleBetaHash := differentHash(betaHash)
	staleGammaHash := differentHash(gammaHash)

	canceledCtx, cancel := context.WithCancel(t.Context())
	cancel()

	tests := []struct {
		name         string
		ctx          context.Context // nil means t.Context()
		fileContent  string
		edits        []FlatEdit
		want         string
		wantErrIs    error
		wantMismatch *HashMismatch // expect HashlineMismatchError with exactly this mismatch
		wantErr      string        // expect error message containing this
	}{
		{
			name:        "replaces a single line",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From:    hashlineRef(2, "beta"),
				To:      hashlineRef(2, "beta"),
				Content: new("BETA"),
			}},
			want: "alpha\nBETA\ngamma",
		},
		{
			name:        "replaces an inclusive range",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From:    hashlineRef(2, "beta"),
				To:      hashlineRef(3, "gamma"),
				Content: new("combined"),
			}},
			want: "alpha\ncombined",
		},
		{
			name:        "deletes a range when content is nil",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From: hashlineRef(2, "beta"),
				To:   hashlineRef(3, "gamma"),
			}},
			want: "alpha",
		},
		{
			name:        "uses original anchors for multiple edits",
			fileContent: "one\ntwo\nthree\nfour\nfive",
			edits: []FlatEdit{
				{
					From:    hashlineRef(2, "two"),
					To:      hashlineRef(2, "two"),
					Content: new("two-a\ntwo-b\ntwo-c"),
				},
				{
					From: hashlineRef(5, "five"),
					To:   hashlineRef(5, "five"),
				},
			},
			want: "one\ntwo-a\ntwo-b\ntwo-c\nthree\nfour",
		},
		{
			name:        "deduplicates identical edits",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{
				{
					From:    hashlineRef(2, "beta"),
					To:      hashlineRef(2, "beta"),
					Content: new("beta-a\nbeta-b"),
				},
				{
					From:    hashlineRef(2, "beta"),
					To:      hashlineRef(2, "beta"),
					Content: new("beta-a\nbeta-b"),
				},
			},
			want: "alpha\nbeta-a\nbeta-b\ngamma",
		},
		{
			name:        "rejects a stale hash",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From:    fmt.Sprintf("2#%s", staleBetaHash),
				To:      hashlineRef(2, "beta"),
				Content: new("BETA"),
			}},
			wantMismatch: &HashMismatch{Line: 2, Expected: staleBetaHash, Actual: betaHash},
		},
		{
			name:        "rejects a reversed range",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From:    hashlineRef(3, "gamma"),
				To:      hashlineRef(2, "beta"),
				Content: new("unused"),
			}},
			wantErr: "range start line 3 must be <= end line 2",
		},
		{
			name:        "rejects an out-of-bounds range",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From:    hashlineRef(1, "alpha"),
				To:      "4#aaa",
				Content: new("unused"),
			}},
			wantErr: "line range 1-4 is out of bounds",
		},
		{
			name:        "returns a canceled context",
			ctx:         canceledCtx,
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From:    hashlineRef(2, "beta"),
				To:      hashlineRef(2, "beta"),
				Content: new("BETA"),
			}},
			wantErrIs: context.Canceled,
		},
		{
			name:        "validates all edits before applying",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{
				{
					From:    hashlineRef(1, "alpha"),
					To:      hashlineRef(1, "alpha"),
					Content: new("ALPHA"),
				},
				{
					From:    fmt.Sprintf("3#%s", staleGammaHash),
					To:      hashlineRef(3, "gamma"),
					Content: new("ALPHA"),
				},
			},
			wantMismatch: &HashMismatch{Line: 3, Expected: staleGammaHash, Actual: gammaHash},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.ctx
			if ctx == nil {
				ctx = t.Context()
			}
			got, err := ApplyHashlineEdit(ctx, tt.fileContent, EditInput{Edits: tt.edits})

			switch {
			case tt.wantErrIs != nil:
				require.ErrorIs(t, err, tt.wantErrIs)
				require.Empty(t, got)
			case tt.wantMismatch != nil:
				require.Error(t, err)
				require.Empty(t, got)
				var mismatchErr *HashlineMismatchError
				require.ErrorAs(t, err, &mismatchErr)
				require.Equal(t, []HashMismatch{*tt.wantMismatch}, mismatchErr.mismatches)
			case tt.wantErr != "":
				require.Error(t, err)
				require.Empty(t, got)
				require.Contains(t, err.Error(), tt.wantErr)
			default:
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestRunEditFileHash(t *testing.T) {
	original := "alpha\nbeta\ngamma"
	replacement := "BETA"
	edits := []FlatEdit{{
		From:    hashlineRef(2, "beta"),
		To:      hashlineRef(2, "beta"),
		Content: &replacement,
	}}

	tests := []struct {
		name    string
		hash    string // empty means "use the current file hash"
		wantErr string
		want    string
	}{
		{name: "matching file hash applies edit", want: "alpha\nBETA\ngamma"},
		{
			name: "leading hash sigil is stripped",
			hash: "#" + util.ComputeFileHash(original),
			want: "alpha\nBETA\ngamma",
		},
		{
			name: "full @file header is accepted",
			hash: "@file sample.txt#" + util.ComputeFileHash(original),
			want: "alpha\nBETA\ngamma",
		},
		{name: "stale file hash is rejected", hash: "DEAD", wantErr: "file TAG mismatch", want: original},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "sample.txt")
			require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

			hash := tt.hash
			if hash == "" {
				hash = util.ComputeFileHash(original)
			}
			raw, err := json.Marshal(EditInput{Path: path, Hash: hash, Edits: edits})
			require.NoError(t, err)

			res, err := runEdit(t.Context(), raw)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, res.Content, "@file ")
				require.Contains(t, res.Content, "Re-read this file before another edit")
			}
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(got))
		})
	}
}

func hashlineRef(line int, content string) string {
	return fmt.Sprintf("%d#%s", line, util.ComputeLineHash(content))
}

func TestNormalizeFileTag(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"A1B2", "A1B2"},
		{"a1b2", "A1B2"},
		{"#A1B2", "A1B2"},
		{"@file src/app.py#A1B2", "A1B2"},
		{"  @file src/app.py#a1b2  ", "A1B2"},
		{"", ""},
		{"#", ""},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, normalizeFileTag(tt.in), tt.in)
	}
}

func TestParseLineRef(t *testing.T) {
	line, hash, err := parseLineRef("5#abc|content")
	require.NoError(t, err)
	require.Equal(t, 5, line)
	require.Equal(t, "abc", hash)

	_, _, err = parseLineRef("1#pix|.idea/\n2#qwr|/phi")
	require.Error(t, err)
	require.Contains(t, err.Error(), "single LINE#HASH")
}

func differentHash(current string) string {
	if current == "aaa" {
		return "aab"
	}
	return "aaa"
}
