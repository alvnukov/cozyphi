package writetool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
	"github.com/alvnukov/cozyphi/internal/util"
)

// askPreviewLines caps the preview: the ask overlay scrolls, but a
// generated-megabyte diff would still ride along on every frame.
const askPreviewLines = 400

// AskPreview renders the diff a pending edit or write would apply, as
// display-only evidence for the permission ask. It is best-effort by
// design: any failure returns "" and the ask falls back to the path list,
// because the tool itself will surface the real error after approval.
func AskPreview(ctx context.Context, toolName string, args json.RawMessage) string {
	switch toolName {
	case "edit":
		return editPreview(ctx, args)
	case "write":
		return writePreview(ctx, args)
	default:
		return ""
	}
}

// editPreview replays the hashline edits in memory against the file on
// disk — the same parse, the same apply the tool will run — so the diff
// the user approves is the diff the file gets. Stale line hashes fail the
// replay, which is correct: the real run would refuse them too.
func editPreview(ctx context.Context, args json.RawMessage) string {
	param, err := parseEditInput(ctx, args)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(param.Path)
	if err != nil {
		return ""
	}
	old := util.NormalizeLF(string(data))
	updated, err := ApplyHashlineEdit(ctx, old, param)
	if err != nil || updated == old {
		return ""
	}
	return clipPreview(util.GenerateFileDiff(param.Path, old, updated, 3))
}

// writePreview diffs the incoming content against what is on disk; a file
// that does not exist yet diffs against nothing, so the preview is the
// whole new file as additions.
func writePreview(ctx context.Context, args json.RawMessage) string {
	var in writeInput
	if err := json.Unmarshal(args, &in); err != nil {
		return ""
	}
	path := strings.TrimSpace(in.Path)
	if path == "" {
		return ""
	}
	abs, err := tooldef.ResolveToCwd(ctx, path)
	if err != nil {
		return ""
	}
	old := ""
	if data, readErr := os.ReadFile(abs); readErr == nil {
		old = util.NormalizeLF(string(data))
	}
	updated := util.NormalizeLF(in.Content)
	if updated == old {
		return ""
	}
	return clipPreview(util.GenerateFileDiff(abs, old, updated, 3))
}

func clipPreview(diff string) string {
	diff = strings.TrimRight(diff, "\n")
	lines := strings.Split(diff, "\n")
	if len(lines) <= askPreviewLines {
		return diff
	}
	return strings.Join(lines[:askPreviewLines], "\n") +
		fmt.Sprintf("\n… %d more diff lines", len(lines)-askPreviewLines)
}
