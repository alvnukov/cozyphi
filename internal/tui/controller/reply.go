package controller

import "github.com/alvnukov/cozyphi/internal/tools/questiontool"

// AskReply is the user's response for a gated tool confirmation.
type AskReply struct {
	Approved        bool
	Feedback        string
	AllowSession    bool // Allow All for This Session
	AllowPersistent bool // Allow All for Every Session
}

// ContinueReply is the user's response when the tool-round budget is exhausted.
type ContinueReply struct {
	Continue bool
}

// QuestionReply carries the user's answers, one entry per question in the
// same order the model asked them. Each answer is the selected option labels.
type QuestionReply struct {
	Answers []questiontool.Answer
}
