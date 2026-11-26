package usecase

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

func TestRecentContextKeepsShortConversationsBeyondOldLimit(t *testing.T) {
	registry := NewConversationSessions()
	ctx := conversationTestContext()
	in := ChatInput{DeviceTypeID: 9, SessionID: uuid.NewString(), Message: "你好"}
	for i := range 30 {
		turn, err := registry.Begin(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		_, err = turn.Run(ctx, func(_ context.Context, input ChatInput) (ChatOutput, error) {
			if len(input.History) != i*2 {
				t.Errorf("turn=%d history=%d, want %d", i, len(input.History), i*2)
			}
			if err := validateChatInput(ctx, input); err != nil {
				t.Errorf("retained history rejected: %v", err)
			}
			return ChatOutput{Reply: "你好呀"}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecentContextUsesCharacterWatermarksAndWholeExchanges(t *testing.T) {
	var history []Message
	for _, ch := range []string{"甲", "乙", "丙", "丁", "戊"} {
		history = append(history,
			Message{Role: string(schema.User), Content: strings.Repeat(ch, 500)},
			Message{Role: string(schema.Assistant), Content: strings.Repeat(ch, 500)},
		)
	}
	if got := trimRecentHistory(history); len(got) != 10 {
		t.Fatalf("exact high watermark trimmed: %d messages", len(got))
	}
	history = append(history,
		Message{Role: string(schema.User), Content: "最新问题"},
		Message{Role: string(schema.Assistant), Content: "最新回复"},
	)
	got := trimRecentHistory(history)
	if len(got) != 8 || got[0].Content != history[4].Content || got[7].Content != history[11].Content {
		t.Fatalf("incorrect low watermark suffix: %d messages", len(got))
	}
	messages := buildMessages(ChatInput{History: history, Message: "继续"})
	if len(messages) != len(got)+1 || messages[0].Role != schema.User || messages[len(messages)-1].Content != "继续" {
		t.Fatalf("unexpected bounded model context: %d messages", len(messages))
	}
}

func TestRecentContextBoundsOversizedReplyWithoutBreakingUnicode(t *testing.T) {
	user := "刚才讲了什么？"
	reply := strings.Repeat("故事🌟", recentHistoryHighWater)
	got := recentReplyExcerpt(user, reply)
	if !utf8.ValidString(got) || !strings.HasSuffix(got, historyExcerptNotice) ||
		utf8.RuneCountInString(user)+utf8.RuneCountInString(got) != recentHistoryLowWater {
		t.Fatal("oversized reply did not produce a valid bounded excerpt")
	}
	if short := recentReplyExcerpt(user, "故事"); short != "故事" {
		t.Fatalf("ordinary reply altered: %q", short)
	}
	in := ChatInput{DeviceTypeID: 9, Message: user, History: []Message{
		{Role: string(schema.User), Content: user},
		{Role: string(schema.Assistant), Content: got},
	}}
	if err := validateChatInput(context.Background(), in); err != nil {
		t.Fatalf("bounded multi-byte history rejected: %v", err)
	}
}
