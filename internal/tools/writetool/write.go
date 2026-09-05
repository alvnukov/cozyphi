package writetool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alvnukov/cozyphi/internal/tools/editledger"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"

	"github.com/alvnukov/cozyphi/internal/atomicfile"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/util"
)

var writeDescription = `Write content to a file. Creates the file if it does not exist; overwrites the entire file if it does. Creates parent directories. A successful write prints the file's new TAG and live LINE#HASH anchors that authorize the next edit without re-reading; anchors from before the write are dead, and any other line needs a fresh read with mode:"edit" of that range first.`

// WriteTool returns the write tool definition + handler. An optional ledger
// lets a session registry mint the post-write edit capability: the registry
// that also owns editable reads and edits.
func WriteTool(ledgers ...*editledger.Ledger) tooldef.Tool {
	var ledger *editledger.Ledger
	if len(ledgers) > 0 {
		ledger = ledgers[0]
	}
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "write",
			Description: writeDescription,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"path": llm.Object{
						"type":        "string",
						"description": "File path to write (created or overwritten). Example: src/new.go",
					},
					"content": llm.Object{
						"type":        "string",
						"description": "Content to write to the file.",
					},
				},
				Required: []string{"path", "content"},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in writeInput
			_ = json.Unmarshal(input, &in)
			return strings.TrimSpace(in.Path)
		},
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			return runWrite(ctx, input, ledger)
		},
	}
}

type writeInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func runWrite(ctx context.Context, input json.RawMessage, ledger *editledger.Ledger) (tooldef.Result, error) {
	var in writeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tooldef.Result{}, fmt.Errorf("failed to parse write arguments: %w", err)
	}
	path := strings.TrimSpace(in.Path)
	if path == "" {
		return tooldef.Result{}, errors.New("path is required")
	}
	path, err := tooldef.ResolveToCwd(ctx, path)
	if err != nil {
		return tooldef.Result{}, err
	}

	// Captured before the write so the transcript diff card shows what this
	// call actually changed; a file that does not exist yet diffs against
	// nothing and the whole write reads as additions. The read refuses a leaf
	// symlink, so a swapped link cannot leak foreign content into the diff.
	old := ""
	if data, readErr := atomicfile.ReadNoFollow(path); readErr == nil {
		old = util.NormalizeLF(string(data))
	}

	// The swap is staged and renamed into place by the shared mutation module:
	// a symlink swapped in after the permission check is refused rather than
	// written through, the guard re-applies the permission verdict to the
	// destination so a redirected ancestor fails closed, and a torn write
	// cannot happen. The rename brings its own permissions, so an existing
	// file's mode is carried over explicitly.
	if err := atomicfile.WriteWith(path, destinationMode(path), []byte(in.Content), atomicfile.Options{
		Guard: mutationGuard(ctx),
	}); err != nil {
		return tooldef.Result{}, fmt.Errorf("failed to write file %s: %w", path, err)
	}

	display := tooldef.RelToCwd(ctx, path)
	detail := fmt.Sprintf("wrote %d bytes to %s", len(in.Content), display)
	normalized := util.NormalizeLF(in.Content)
	newRev := util.RevisionOf(normalized)

	// The model authored this content and the guarded swap above placed it
	// on disk, so this exact revision is trusted knowledge: the grant lets a
	// follow-up edit land without an intermediate read(mode:"edit"). A
	// ledger-less registry prints no anchors — a printed grant is always a
	// real one — and the anchors carry line hashes only, never content.
	var body strings.Builder
	body.WriteString(detail)
	if ledger != nil {
		lines := strings.Split(normalized, "\n")
		grant := successorGrantFor([]editledger.Span{{From: 1, To: len(lines)}}, lines, newRev.Tag())
		// Every earlier snapshot of the path is retired with the swap: none of
		// them describes the file any more, and an edit that still quotes a
		// pre-write TAG is refused by the ledger and pointed at the anchors
		// printed below.
		ledger.Supersede(path, newRev, grant.anchors)
		if grant.tag != "" {
			body.WriteString("\n" + util.FormatFileHeader(display, newRev.Tag()) + "\n")
			writeSuccessorBlock(&body, grant)
		}
	}
	diff := util.GenerateFileDiff(path, old, normalized, 3)
	return tooldef.Result{Content: body.String(), Detail: display, Output: diff}, nil
}
