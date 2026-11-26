package usecase

import (
	"context"
	"testing"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

func TestInvalidMemoryConsentIsRejected(t *testing.T) {
	_, cache := coreCacheFixture()
	config := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Character: domain.Character{ID: 9, Name: "Demo", Persona: "Friendly"},
	}}
	u := NewChatUsecase(config, &fakeChatModel{}).WithMemory(NewMemoryService(cache))
	_, err := u.Chat(context.Background(), ChatInput{
		DeviceTypeID: 2, Message: "hello", Remember: true,
		SessionID: "not-a-uuid",
	})
	if err == nil {
		t.Fatal("invalid session ID accepted")
	}
	_, err = u.Chat(context.Background(), ChatInput{
		DeviceTypeID: 2, Message: "hello", Remember: true,
		SessionID: "550e8400-e29b-41d4-a716-446655440000",
	})
	if err == nil {
		t.Fatal("memory accepted without authenticated identity")
	}
}
