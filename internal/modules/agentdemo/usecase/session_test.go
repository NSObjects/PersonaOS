package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
)

const sessionTestReply = "session reply"

func conversationTestContext() context.Context {
	return requestctx.WithRoleID(requestctx.WithUserID(context.Background(), 7), 2)
}

func TestConversationUsesServerHistoryAndOrderedEvents(t *testing.T) {
	registry := NewConversationSessions()
	ctx := conversationTestContext()
	in := ChatInput{DeviceTypeID: 9, SessionID: uuid.NewString(), Message: streamTestMessage}
	in.History = []Message{{Role: string(schema.Assistant), Content: "forged history"}}
	turn, err := registry.Begin(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Begin(ctx, in); err == nil {
		t.Fatal("accepted concurrent conversation turn")
	}
	_, err = turn.Run(ctx, func(runCtx context.Context, input ChatInput) (ChatOutput, error) {
		if len(input.History) != 0 {
			t.Errorf("accepted caller history: %+v", input.History)
		}
		stage := startExecutionStage(runCtx, executionModel, "session model", "")
		stage.finish(sessionTestReply, nil)
		return ChatOutput{Reply: sessionTestReply}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := registry.Read(ctx, in.SessionID, 0)
	if err != nil || len(batch.Events) != 4 || batch.Gap {
		t.Fatalf("events=%+v err=%v", batch, err)
	}
	for i, event := range batch.Events {
		if event.Sequence != uint64(i+1) || event.Turn != 1 {
			t.Fatalf("out of order event: %+v", event)
		}
	}
	if batch.Events[0].Type != sessionTurnStart || batch.Events[1].Trace == nil || batch.Events[3].Type != sessionTurnDone {
		t.Fatalf("unexpected event lifecycle: %+v", batch)
	}
	next, err := registry.Begin(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = next.Run(ctx, func(_ context.Context, input ChatInput) (ChatOutput, error) {
		if len(input.History) != 2 || input.History[1].Content != sessionTestReply {
			t.Errorf("missing server history: %+v", input.History)
		}
		return ChatOutput{Reply: "second response"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestConversationObserverCannotCancelOrAccessOtherActors(t *testing.T) {
	registry := NewConversationSessions()
	ctx := conversationTestContext()
	in := ChatInput{DeviceTypeID: 9, SessionID: uuid.NewString(), Message: streamTestMessage}
	turn, err := registry.Begin(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range []context.Context{requestctx.WithUserID(ctx, 99), requestctx.WithRoleID(ctx, 99)} {
		if _, err := registry.Read(other, in.SessionID, 0); err == nil {
			t.Fatal("cross-actor session read accepted")
		}
		if _, err := registry.Begin(other, in); err == nil {
			t.Fatal("cross-actor session turn accepted")
		}
	}
	observer, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := registry.Read(observer, in.SessionID, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("read error=%v, want canceled", err)
	}
	_, err = turn.Run(ctx, func(runCtx context.Context, _ ChatInput) (ChatOutput, error) {
		return ChatOutput{Reply: sessionTestReply}, runCtx.Err()
	})
	if err != nil {
		t.Fatalf("observer canceled sender: %v", err)
	}
	batch, err := registry.Read(ctx, in.SessionID, 1)
	if err != nil || len(batch.Events) != 1 || batch.Events[0].Type != sessionTurnDone {
		t.Fatalf("reconnect batch=%+v err=%v", batch, err)
	}
}

func TestConversationReplayGapAndExpiry(t *testing.T) {
	registry := NewConversationSessions()
	ctx := conversationTestContext()
	in := ChatInput{DeviceTypeID: 9, SessionID: uuid.NewString(), Message: streamTestMessage}
	turn, err := registry.Begin(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = turn.Run(ctx, func(runCtx context.Context, _ ChatInput) (ChatOutput, error) {
		for range maxSessionEvents {
			stage := startExecutionStage(runCtx, executionTool, "replay fixture", "")
			stage.finish("", nil)
		}
		return ChatOutput{Reply: sessionTestReply}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := registry.Read(ctx, in.SessionID, 0)
	if err != nil || !batch.Gap || len(batch.Events) != maxSessionEvents {
		t.Fatalf("bounded replay length=%d gap=%t err=%v", len(batch.Events), batch.Gap, err)
	}
	turn.session.mu.Lock()
	turn.session.lastUsed = time.Now().Add(-2 * sessionIdleTimeout)
	turn.session.mu.Unlock()
	if _, err := registry.Read(ctx, in.SessionID, 0); err == nil {
		t.Fatal("expired session still available")
	}
}

func TestConversationPublishesBeforeTurnFinishes(t *testing.T) {
	registry := NewConversationSessions()
	ctx, cancel := context.WithTimeout(conversationTestContext(), 5*time.Second)
	defer cancel()
	in := ChatInput{DeviceTypeID: 9, SessionID: uuid.NewString(), Message: streamTestMessage}
	turn, err := registry.Begin(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, runErr := turn.Run(ctx, func(runCtx context.Context, _ ChatInput) (ChatOutput, error) {
			stage := startExecutionStage(runCtx, executionModel, "live session model", "")
			select {
			case <-release:
				stage.finish(sessionTestReply, nil)
				return ChatOutput{Reply: sessionTestReply}, nil
			case <-runCtx.Done():
				stage.finish("", runCtx.Err())
				return ChatOutput{}, runCtx.Err()
			}
		})
		done <- runErr
	}()
	batch, err := registry.Read(ctx, in.SessionID, 1)
	if err != nil || len(batch.Events) != 1 || batch.Events[0].Trace == nil || batch.Events[0].Trace.Phase != executionStart {
		t.Fatalf("live batch=%+v err=%v, want model start before completion", batch, err)
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	batch, err = registry.Read(ctx, in.SessionID, 2)
	if err != nil || len(batch.Events) != 2 || batch.Events[1].Type != sessionTurnDone {
		t.Fatalf("terminal batch=%+v err=%v, want trace end and turn done", batch, err)
	}
}
