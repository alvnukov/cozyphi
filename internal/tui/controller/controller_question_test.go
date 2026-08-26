package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
)

func TestControllerAskQuestionPublishesAndWaitsForReply(t *testing.T) {
	ctrl := newReadyController(t)
	ctx := t.Context()

	var got []questiontool.Answer
	done := make(chan struct{})
	go func() {
		defer close(done)
		answers, err := ctrl.askQuestion(ctx, []questiontool.Question{{Question: "q", Header: "h", Custom: true}})
		require.NoError(t, err)
		got = answers
	}()

	// Wait until the QuestionAskMsg reaches the bus, then answer it.
	var reply chan QuestionReply
	var received []questiontool.Question
	for reply == nil {
		for _, m := range ctrl.bus.Drain() {
			if q, ok := m.(QuestionAskMsg); ok {
				reply = q.Reply
				received = q.Questions
			}
		}
		if reply == nil {
			select {
			case <-ctrl.bus.Chan():
			case <-time.After(5 * time.Second):
				t.Fatal("question ask never reached the bus")
			}
		}
	}

	assert.Len(t, received, 1)
	assert.Equal(t, "q", received[0].Question)

	reply <- QuestionReply{Answers: []questiontool.Answer{{"Build"}}}
	<-done
	assert.Equal(t, []questiontool.Answer{{"Build"}}, got)
}

func TestControllerAskQuestionDismissOnContextCancel(t *testing.T) {
	ctrl := newReadyController(t)
	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := ctrl.askQuestion(ctx, []questiontool.Question{{Question: "q", Header: "h"}})
		require.Error(t, err)
	}()

	// Wait for the ask on the bus, then cancel the context.
	var saw bool
	for !saw {
		for _, m := range ctrl.bus.Drain() {
			if _, ok := m.(QuestionAskMsg); ok {
				saw = true
			}
		}
		if !saw {
			select {
			case <-ctrl.bus.Chan():
			case <-time.After(5 * time.Second):
				t.Fatal("question ask never reached the bus")
			}
		}
	}

	cancel()
	<-done

	// A dismiss must have been published for the UI to clear the overlay.
	var dismissed bool
	for _, m := range ctrl.bus.Drain() {
		if _, ok := m.(QuestionDismissMsg); ok {
			dismissed = true
		}
	}
	assert.True(t, dismissed)
}
