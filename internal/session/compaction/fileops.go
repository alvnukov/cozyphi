package compaction

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
)

type FileOperation struct {
	read    []string
	written []string
	edited  []string
}

func extractPathFromArgs(args string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil || m == nil {
		return ""
	}
	if p, ok := m["path"].(string); ok && p != "" {
		return p
	}
	if p, ok := m["file_path"].(string); ok && p != "" {
		return p
	}
	return ""
}

func (f *FileOperation) extractMessageContent(message llm.Message) {
	if message.Role != llm.RoleAssistant {
		return
	}
	if len(message.ToolCalls) == 0 {
		return
	}
	for _, toolCall := range message.ToolCalls {
		name := toolCall.Function.Name
		path := extractPathFromArgs(toolCall.Function.Arguments)
		if path == "" {
			continue
		}
		switch name {
		case "read":
			f.read = append(f.read, path)
		case "write":
			f.written = append(f.written, path)
		case "edit":
			f.edited = append(f.edited, path)
		}
	}
}

func extractFileOperations(
	messages []llm.Message,
	entries []session.MessageEntry,
	prevCompactionIndex int,
) *FileOperation {
	fileOps := &FileOperation{}
	// Collect from previous compaction's details (if pi-generated)
	if prevCompactionIndex >= 0 {
		comp := entries[prevCompactionIndex].(session.CompactionEntry)
		if comp.Compaction.FromExtension != nil {
			details, ok := comp.Compaction.Details.(session.CompactionDetails)
			if ok {
				fileOps.read = append(fileOps.read, details.ReadFiles...)
				fileOps.written = append(fileOps.written, details.ModifiedFiles...)
			}
		}
	}

	for _, msg := range messages {
		fileOps.extractMessageContent(msg)
	}
	return fileOps
}

func formatFileOperations(readFiles, modifiedFiles []string) string {
	sections := []string{}
	if len(readFiles) > 0 {
		sections = append(sections, "<read-files>\n"+strings.Join(readFiles, "\n")+"\n</read-files>")
	}
	if len(modifiedFiles) > 0 {
		sections = append(sections, "<modified-files>\n"+strings.Join(modifiedFiles, "\n")+"\n</modified-files>")
	}
	if len(sections) == 0 {
		return ""
	}
	return "\n\n" + strings.Join(sections, "\n\n")
}

func computeFileLists(fileOps *FileOperation) ([]string, []string) {
	modifiedSet := make(map[string]struct{})
	readFiles := []string{}
	modifiedFiles := []string{}

	for _, p := range fileOps.edited {
		modifiedSet[p] = struct{}{}
	}
	for _, p := range fileOps.written {
		modifiedSet[p] = struct{}{}
	}

	for _, p := range fileOps.read {
		if _, exists := modifiedSet[p]; !exists {
			readFiles = append(readFiles, p)
		}
	}

	for p := range modifiedSet {
		modifiedFiles = append(modifiedFiles, p)
	}

	sort.Strings(readFiles)
	sort.Strings(modifiedFiles)

	return readFiles, modifiedFiles
}
