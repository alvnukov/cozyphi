package compaction

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
)

func TestExtractPathFromArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantPath string
	}{
		{"path field", `{"path":"a/b.go"}`, "a/b.go"},
		{"file_path field", `{"file_path":"x/y.txt","content":""}`, "x/y.txt"},
		{"path takes precedence over file_path", `{"path":"p","file_path":"fp"}`, "p"},
		{"use file_path when path is empty", `{"path":"","file_path":"fp"}`, "fp"},
		{"empty string", `""`, ""},
		{"invalid JSON", `{path}`, ""},
		{"empty object", `{}`, ""},
		{"path is not a string", `{"path":123}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPathFromArgs(tt.args)
			assert.Equal(t, tt.wantPath, got)
		})
	}
}

func TestFileOperation_extractMessageContent(t *testing.T) {
	t.Run("skip non-assistant messages", func(t *testing.T) {
		f := &FileOperation{}
		msg := llm.Message{
			Role: llm.RoleUser,
			ToolCalls: []llm.ToolCall{{
				Function: llm.Function{Name: "read", Arguments: `{"path":"x"}`},
			}},
		}
		f.extractMessageContent(msg)
		assert.Empty(t, f.read)
		assert.Empty(t, f.written)
		assert.Empty(t, f.edited)
	})

	t.Run("skip when no ToolCalls", func(t *testing.T) {
		f := &FileOperation{}
		msg := llm.Message{Role: llm.RoleAssistant, ToolCalls: nil}
		f.extractMessageContent(msg)
		assert.Empty(t, f.read)
	})

	t.Run("categorize paths by tool name", func(t *testing.T) {
		f := &FileOperation{}
		msg := llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{Function: llm.Function{Name: "read", Arguments: `{"path":"a.go"}`}},
				{Function: llm.Function{Name: "read", Arguments: `{"path":"b.go"}`}},
				{Function: llm.Function{Name: "write", Arguments: `{"file_path":"c.go","content":"x"}`}},
				{Function: llm.Function{Name: "edit", Arguments: `{"path":"d.go","edits":[]}`}},
			},
		}
		f.extractMessageContent(msg)
		assert.Equal(t, []string{"a.go", "b.go"}, f.read)
		assert.Equal(t, []string{"c.go"}, f.written)
		assert.Equal(t, []string{"d.go"}, f.edited)
	})

	t.Run("skip tool calls without path", func(t *testing.T) {
		f := &FileOperation{}
		msg := llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{Function: llm.Function{Name: "read", Arguments: `{}`}},
				{Function: llm.Function{Name: "read", Arguments: `{"path":"ok.go"}`}},
			},
		}
		f.extractMessageContent(msg)
		assert.Equal(t, []string{"ok.go"}, f.read)
	})

	t.Run("skip non-file tools", func(t *testing.T) {
		f := &FileOperation{}
		msg := llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{Function: llm.Function{Name: "search", Arguments: `{"path":"x"}`}},
				{Function: llm.Function{Name: "bash", Arguments: `{}`}},
			},
		}
		f.extractMessageContent(msg)
		assert.Empty(t, f.read)
		assert.Empty(t, f.written)
		assert.Empty(t, f.edited)
	})

	t.Run("accumulate across multiple calls", func(t *testing.T) {
		f := &FileOperation{}
		f.extractMessageContent(llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{Function: llm.Function{Name: "read", Arguments: `{"path":"first.go"}`}},
			},
		})
		f.extractMessageContent(llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{Function: llm.Function{Name: "read", Arguments: `{"path":"second.go"}`}},
			},
		})
		assert.Equal(t, []string{"first.go", "second.go"}, f.read)
	})
}

func TestExtractFileOperations(t *testing.T) {
	ts := time.Now()
	trueVal := true
	falseVal := false

	t.Run("no previous compaction", func(t *testing.T) {
		messages := []llm.Message{
			{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{
					{Function: llm.Function{Name: "read", Arguments: `{"path":"a.go"}`}},
				},
			},
		}
		got := extractFileOperations(messages, nil, -1)
		assert.Equal(t, []string{"a.go"}, got.read)
		assert.Empty(t, got.written)
		assert.Empty(t, got.edited)
	})

	t.Run("prevCompactionIndex >= 0 but FromExtension is nil", func(t *testing.T) {
		entries := []session.MessageEntry{
			session.CompactionEntry{
				SessionBaseEntry: session.SessionBaseEntry{ID: "c1", Type: session.EntryCompaction, Timestamp: ts},
				Compaction: session.Compaction{
					Details: session.CompactionDetails{
						ReadFiles:     []string{"prev.go"},
						ModifiedFiles: []string{"prev2.go"},
					},
					FromExtension: nil,
				},
			},
		}
		got := extractFileOperations(nil, entries, 0)
		assert.Empty(t, got.read)
		assert.Empty(t, got.written)
	})

	t.Run("prevCompactionIndex >= 0 but Details is not CompactionDetails", func(t *testing.T) {
		entries := []session.MessageEntry{
			session.CompactionEntry{
				SessionBaseEntry: session.SessionBaseEntry{ID: "c1", Type: session.EntryCompaction, Timestamp: ts},
				Compaction: session.Compaction{
					Details:       "other type",
					FromExtension: &trueVal,
				},
			},
		}
		got := extractFileOperations(nil, entries, 0)
		assert.Empty(t, got.read)
		assert.Empty(t, got.written)
	})

	t.Run("merge from previous compaction details", func(t *testing.T) {
		entries := []session.MessageEntry{
			session.CompactionEntry{
				SessionBaseEntry: session.SessionBaseEntry{ID: "c1", Type: session.EntryCompaction, Timestamp: ts},
				Compaction: session.Compaction{
					Details: session.CompactionDetails{
						ReadFiles:     []string{"r1.go", "r2.go"},
						ModifiedFiles: []string{"w1.go"},
					},
					FromExtension: &trueVal,
				},
			},
		}
		got := extractFileOperations(nil, entries, 0)
		assert.Equal(t, []string{"r1.go", "r2.go"}, got.read)
		assert.Equal(t, []string{"w1.go"}, got.written)
		assert.Empty(t, got.edited)
	})

	t.Run("merge previous details and current messages", func(t *testing.T) {
		entries := []session.MessageEntry{
			session.CompactionEntry{
				SessionBaseEntry: session.SessionBaseEntry{ID: "c1", Type: session.EntryCompaction, Timestamp: ts},
				Compaction: session.Compaction{
					Details: session.CompactionDetails{
						ReadFiles:     []string{"prev_read.go"},
						ModifiedFiles: []string{"prev_written.go"},
					},
					FromExtension: &trueVal,
				},
			},
		}
		messages := []llm.Message{
			{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{
					{Function: llm.Function{Name: "read", Arguments: `{"path":"msg_read.go"}`}},
					{Function: llm.Function{Name: "edit", Arguments: `{"path":"msg_edit.go"}`}},
				},
			},
		}
		got := extractFileOperations(messages, entries, 0)
		assert.Equal(t, []string{"prev_read.go", "msg_read.go"}, got.read)
		assert.Equal(t, []string{"prev_written.go"}, got.written)
		assert.Equal(t, []string{"msg_edit.go"}, got.edited)
	})

	t.Run("FromExtension non-nil uses details regardless of true/false", func(t *testing.T) {
		entries := []session.MessageEntry{
			session.CompactionEntry{
				SessionBaseEntry: session.SessionBaseEntry{ID: "c1", Type: session.EntryCompaction, Timestamp: ts},
				Compaction: session.Compaction{
					Details: session.CompactionDetails{
						ReadFiles:     []string{"x.go"},
						ModifiedFiles: []string{"y.go"},
					},
					FromExtension: &falseVal,
				},
			},
		}
		got := extractFileOperations(nil, entries, 0)
		assert.Equal(t, []string{"x.go"}, got.read)
		assert.Equal(t, []string{"y.go"}, got.written)
	})
}

func TestComputeFileLists(t *testing.T) {
	t.Run("no overlap between read and modified", func(t *testing.T) {
		f := &FileOperation{
			read:    []string{"a.go", "b.go"},
			written: []string{"c.go"},
			edited:  []string{"d.go"},
		}

		readFiles, modifiedFiles := computeFileLists(f)

		assert.Equal(t, []string{"a.go", "b.go"}, readFiles)
		assert.Equal(t, []string{"c.go", "d.go"}, modifiedFiles)
	})

	t.Run("read files that are later modified are excluded from readFiles", func(t *testing.T) {
		f := &FileOperation{
			read:    []string{"a.go", "b.go", "c.go"},
			written: []string{"b.go"},
			edited:  []string{"d.go"},
		}

		readFiles, modifiedFiles := computeFileLists(f)

		assert.Equal(t, []string{"a.go", "c.go"}, readFiles)
		assert.Equal(t, []string{"b.go", "d.go"}, modifiedFiles)
	})

	t.Run("deduplicates modified files and sorts", func(t *testing.T) {
		f := &FileOperation{
			read:    []string{"z.go", "a.go"},
			written: []string{"b.go", "b.go"},
			edited:  []string{"a.go", "c.go"},
		}

		readFiles, modifiedFiles := computeFileLists(f)

		// a.go is modified, so it should not appear in readFiles
		assert.Equal(t, []string{"z.go"}, readFiles)
		assert.Equal(t, []string{"a.go", "b.go", "c.go"}, modifiedFiles)
	})

	t.Run("empty FileOperation yields empty slices", func(t *testing.T) {
		f := &FileOperation{}

		readFiles, modifiedFiles := computeFileLists(f)

		assert.Empty(t, readFiles)
		assert.Empty(t, modifiedFiles)
	})
}
