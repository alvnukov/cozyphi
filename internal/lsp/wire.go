package lsp

import "encoding/json"

// Wire types mirror only the LSP shapes this package consumes. They are
// decoded at the client boundary and never escape it: every result is
// normalized into the bounded model-facing types before entering Result.

type wirePosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type wireRange struct {
	Start wirePosition `json:"start"`
	End   wirePosition `json:"end"`
}

type wireLocation struct {
	URI   string    `json:"uri"`
	Range wireRange `json:"range"`
}

type wireLocationLink struct {
	TargetURI            string    `json:"targetUri"`
	TargetRange          wireRange `json:"targetRange"`
	TargetSelectionRange wireRange `json:"targetSelectionRange"`
}

// wireDocumentSymbol is the hierarchical documentSymbol reply item.
type wireDocumentSymbol struct {
	Name           string               `json:"name"`
	Detail         string               `json:"detail"`
	Kind           int                  `json:"kind"`
	Range          wireRange            `json:"range"`
	SelectionRange wireRange            `json:"selectionRange"`
	Children       []wireDocumentSymbol `json:"children"`
}

// wireSymbolInformation is the flat symbol form used by legacy documentSymbol
// and workspace/symbol replies. Workspace symbols may omit the location.
type wireSymbolInformation struct {
	Name          string        `json:"name"`
	Kind          int           `json:"kind"`
	Location      *wireLocation `json:"location"`
	ContainerName string        `json:"containerName"`
}

// wireCallItem is one prepared call-hierarchy item. Data is the server's
// opaque continuation token: it is replayed verbatim on the follow-up
// incoming/outgoing request and never normalized or exposed.
type wireCallItem struct {
	Name           string          `json:"name"`
	Kind           int             `json:"kind"`
	Detail         string          `json:"detail,omitempty"`
	URI            string          `json:"uri"`
	Range          wireRange       `json:"range"`
	SelectionRange wireRange       `json:"selectionRange"`
	Data           json.RawMessage `json:"data,omitempty"`
}

type wireIncomingCall struct {
	From       wireCallItem `json:"from"`
	FromRanges []wireRange  `json:"fromRanges"`
}

type wireOutgoingCall struct {
	To         wireCallItem `json:"to"`
	FromRanges []wireRange  `json:"fromRanges"`
}
