package usecase

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
)

const (
	recentHistoryHighWater = 5000
	recentHistoryLowWater  = 4000
	historyExcerptNotice   = "…（历史回复已截断）"
	// These are transport safety limits, not conversation window sizes.
	maxHistoryInputBytes = 128 * 1024
	maxHistoryInputItems = recentHistoryHighWater
)

// trimRecentHistory retains the newest contiguous conversation suffix. Session
// history contains complete user/assistant pairs; never retain only the
// assistant half of a removed pair. A caller's incomplete legacy history is
// accepted but also bounded. No tokenizer or external call is required.
func trimRecentHistory(history []Message) []Message {
	total := 0
	for _, item := range history {
		total += utf8.RuneCountInString(item.Content)
	}
	if total <= recentHistoryHighWater {
		return history
	}
	start := 0
	for start < len(history) && (total > recentHistoryLowWater || history[start].Role != string(schema.User)) {
		total -= utf8.RuneCountInString(history[start].Content)
		start++
	}
	return slices.Clone(history[start:])
}

// recentReplyExcerpt prevents a single unusually long response from evicting
// even its own question. The full reply is still delivered and archived by the
// existing memory path. Only the rolling context receives a marked excerpt.
func recentReplyExcerpt(user, reply string) string {
	available := recentHistoryLowWater - utf8.RuneCountInString(user)
	if utf8.RuneCountInString(reply) <= available {
		return reply
	}
	keep := max(0, available-utf8.RuneCountInString(historyExcerptNotice))
	count := 0
	for offset := range reply {
		if count == keep {
			return strings.Clone(reply[:offset]) + historyExcerptNotice
		}
		count++
	}
	return reply
}
