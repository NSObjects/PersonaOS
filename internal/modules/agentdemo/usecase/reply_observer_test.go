package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

func TestSessionStreamsReplyBeforeModelCompletes(t *testing.T) {
	ctx, cancel := context.WithTimeout(conversationTestContext(), 5*time.Second)
	defer cancel()
	release := make(chan struct{})
	config := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Character: domain.Character{Name: streamTestCharacter, Persona: streamTestPersona},
	}}
	chat := NewChatUsecase(config, &incrementalModel{release: release})
	registry := NewConversationSessions()
	in := ChatInput{DeviceTypeID: 7, SessionID: uuid.NewString(), Message: streamTestMessage}
	turn, err := registry.Begin(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, runErr := turn.Run(ctx, chat.Chat)
		done <- runErr
	}()
	var cursor uint64
	var preview string
	for preview == "" {
		batch, err := registry.Read(ctx, in.SessionID, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range batch.Events {
			cursor = event.Sequence
			if event.Type == sessionReplyPreview {
				preview = event.Preview
			}
			if event.Type == sessionTurnDone || event.Type == sessionTurnError {
				t.Fatalf("turn ended before preview: %+v", event)
			}
		}
	}
	if preview != streamTestFirstReply {
		t.Fatalf("preview=%q, want first increment", preview)
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
	batch, err := registry.Read(ctx, in.SessionID, cursor)
	if err != nil {
		t.Fatal(err)
	}
	last := batch.Events[len(batch.Events)-1]
	if last.Type != sessionTurnDone || last.Reply == nil || last.Reply.Reply != streamTestFullReply {
		t.Fatalf("terminal event=%+v", last)
	}
}
