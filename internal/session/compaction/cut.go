package compaction

import (
	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
)

// CutPointResult identifies where to cut the session history: the index of
// the first entry to keep and, when the cut splits a turn, that turn's
// starting index.
type CutPointResult struct {
	/** Index of first entry to keep */
	firstKeptEntryIndex int
	/** Index of user message that starts the turn being split, or -1 if not splitting */
	turnStartIndex int
	/** Whether this cut splits a turn (cut point is not a user message) */
	isSplitTurn bool
}

func findCutPoint(
	entirs []session.MessageEntry,
	startIndex int,
	endIndex int,
	keepRecentTokens int,
) CutPointResult {
	cutpoint := findValidCutPoints(entirs, startIndex, endIndex)
	if len(cutpoint) == 0 {
		return CutPointResult{
			firstKeptEntryIndex: startIndex,
			turnStartIndex:      -1,
			isSplitTurn:         false,
		}
	}

	cutIndex := findCutIndex(
		entirs,
		startIndex,
		endIndex,
		keepRecentTokens,
		cutpoint,
	)

	// find the first compaction entry before the cut index
	for cutIndex > startIndex {
		entry := entirs[cutIndex-1]
		if entry.GetType() == session.EntryCompaction {
			break
		}
		if entry.GetType() == session.EntryMessage {
			break
		}
		cutIndex--
	}

	// Entry at the cut point;
	// used to tell if we cut at a user message (cutting there does not split a turn).
	cutEntry := entirs[cutIndex]
	isUserMessage := cutEntry.GetType() == session.EntryMessage &&
		cutEntry.(session.SessionMessageEntry).Message.Role == llm.RoleUser

	// [userMessage, cutIndex) is the turn
	turnStartIndex := -1
	if !isUserMessage {
		turnStartIndex = findTurnStartIndex(entirs, cutIndex, startIndex)
	}

	return CutPointResult{
		firstKeptEntryIndex: cutIndex,
		turnStartIndex:      turnStartIndex,
		isSplitTurn:         !isUserMessage && turnStartIndex != -1,
	}
}

// findCutIndex walks entries backward from endIndex and estimates each
// message's own serialized size. Provider usage cannot be used here: every
// assistant counter describes the cumulative prompt, so summing it double
// counts history and makes the cut depend on the number of prior turns.
func findCutIndex(
	entries []session.MessageEntry,
	startIndex int,
	endIndex int,
	keepRecentTokens int,
	cutPoints []int,
) int {
	accumulatedTokens := 0
	cutIndex := cutPoints[0]

	for i := endIndex - 1; i >= startIndex; i-- {
		entry := entries[i]
		if entry.GetType() != session.EntryMessage {
			continue
		}

		msgEntry := entry.(session.SessionMessageEntry)
		accumulatedTokens += estimateMessageTokens(msgEntry.Message)

		// [startIndex, endIndex] find first cut point i
		// we need to find the first valid cut point
		if accumulatedTokens > keepRecentTokens {
			// find the first valid cut point
			for c := range cutPoints {
				if cutPoints[c] >= i {
					cutIndex = cutPoints[c]
					break
				}
			}
			break
		}
	}

	return cutIndex
}

// estimateMessageTokens delegates to the shared estimator in the session
// package so cut math and the context browser agree on every token number.
func estimateMessageTokens(msg llm.Message) int {
	return session.EstimateMessageTokens(msg)
}

// findValidCutPoints scans the history to find valid cut points for compaction.
// Rules:
// - Any user/assistant message is a candidate cut point;
// - Any branch summary (EntryBranchSummary) is also a candidate cut point.
// It returns the indices of all candidate entries in chronological order.
func findValidCutPoints(entries []session.MessageEntry, startIndex, endIndex int) []int {
	var cutPoints []int

	for i := startIndex; i < endIndex; i++ {
		e := entries[i]
		ty := e.GetType()

		switch ty {
		case session.EntryMessage:
			msgEntry := e.(session.SessionMessageEntry)
			role := msgEntry.Message.Role
			if role == llm.RoleUser || role == llm.RoleAssistant {
				cutPoints = append(cutPoints, i)
			}
		case session.EntryBranchSummary:
			cutPoints = append(cutPoints, i)
		default:
			continue
		}
	}
	return cutPoints
}

// findTurnStartIndex walks backward from entryIndex to find the summary or user message
// that starts the current turn.
func findTurnStartIndex(entries []session.MessageEntry, entryIndex, startIndex int) int {
	for i := entryIndex; i >= startIndex; i-- {
		entry := entries[i]
		ty := entry.GetType()

		if ty == session.EntryBranchSummary {
			return i
		}

		if ty == session.EntryMessage {
			msgEntry := entry.(session.SessionMessageEntry)
			if msgEntry.Message.Role == llm.RoleUser {
				return i
			}
		}
	}
	return -1
}
