package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
)

func TestInterruptReleasesTurnAndPreservesCompletedHistory(t *testing.T) {
	registry := NewConversationSessions()
	ctx, cancel := context.WithTimeout(conversationTestContext(), 5*time.Second)
	defer cancel()
	in := ChatInput{DeviceTypeID: 9, SessionID: uuid.NewString(), Message: streamTestMessage}
	first, err := registry.Begin(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = first.Run(ctx, func(context.Context, ChatInput) (ChatOutput, error) {
		return ChatOutput{Reply: sessionTestReply}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := registry.Begin(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, runErr := turn.Run(ctx, func(runCtx context.Context, _ ChatInput) (ChatOutput, error) {
			close(started)
			<-runCtx.Done()
			return ChatOutput{}, runCtx.Err()
		})
		done <- runErr
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := registry.Interrupt(requestctx.WithUserID(ctx, 99), in.SessionID); err == nil {
		t.Fatal("foreign actor could interrupt")
	}
	if err := registry.Interrupt(ctx, in.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("old turn error=%v", err)
	}
	next, err := registry.Begin(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = next.Run(ctx, func(_ context.Context, input ChatInput) (ChatOutput, error) {
		if len(input.History) != 2 || input.History[1].Content != sessionTestReply {
			t.Errorf("interrupted turn polluted history: %+v", input.History)
		}
		return ChatOutput{Reply: sessionTestReply}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInterruptBeforeRunPreventsExecution(t *testing.T) {
	registry := NewConversationSessions()
	ctx, cancel := context.WithTimeout(conversationTestContext(), 5*time.Second)
	defer cancel()
	in := ChatInput{DeviceTypeID: 9, SessionID: uuid.NewString(), Message: streamTestMessage}
	turn, err := registry.Begin(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	expired, stop := context.WithCancel(ctx)
	stop()
	if err := registry.Interrupt(expired, in.SessionID); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupt wait=%v", err)
	}
	_, err = turn.Run(ctx, func(context.Context, ChatInput) (ChatOutput, error) {
		t.Error("interrupted reserved turn executed")
		return ChatOutput{}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run error=%v", err)
	}
	if err := registry.Interrupt(ctx, in.SessionID); err != nil {
		t.Fatal(err)
	}
}
