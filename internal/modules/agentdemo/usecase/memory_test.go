package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
)

type memoryStoreStub struct {
	facts []memory.Fact
	seen  chan memory.Exchange
	scope memory.Scope
}

func (s *memoryStoreStub) ListFacts(_ context.Context, scope memory.Scope, _ int) ([]memory.Fact, error) {
	s.scope = scope
	return s.facts, nil
}

func (s *memoryStoreStub) SaveExchange(_ context.Context, turn memory.Exchange) error {
	s.seen <- turn
	return nil
}

func (s *memoryStoreStub) UpsertFacts(_ context.Context, _ memory.Scope, _ []memory.Fact) error {
	return nil
}

func TestOptInMemoryUsesAuthenticatedCharacterScope(t *testing.T) {
	store := &memoryStoreStub{
		facts: []memory.Fact{{Key: "favorite_game", Value: "积木"}},
		seen:  make(chan memory.Exchange, 1),
	}
	config := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Character: domain.Character{ID: 9, Name: "玩具角色", Persona: "温柔"},
	}}
	dialogueModel := &fakeChatModel{}
	extractorModel := &fakeChatModel{}
	u := NewChatUsecase(config, dialogueModel).WithMemory(NewMemoryService(store, extractorModel))
	ctx := requestctx.WithUserID(context.Background(), 31)
	session := "550e8400-e29b-41d4-a716-446655440000"
	out, err := u.Chat(ctx, ChatInput{
		DeviceTypeID: 8, Message: "我喜欢积木", Remember: true, SessionID: session,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.MemoryQueued {
		t.Fatal("expected best-effort memory enqueue")
	}
	if store.scope.OwnerID != 31 || store.scope.CharacterID != 9 {
		t.Fatalf("memory scope = %+v", store.scope)
	}
	if len(dialogueModel.last) == 0 ||
		!strings.Contains(dialogueModel.last[0].Content, "积木") {
		t.Fatalf("memory missing from agent instruction: %+v", dialogueModel.last)
	}
	select {
	case saved := <-store.seen:
		if saved.Scope != store.scope || saved.SessionID != session ||
			saved.User != "我喜欢积木" || saved.Assistant != out.Reply {
			t.Fatalf("saved exchange = %+v", saved)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("background conversation write did not run")
	}
}

func TestChatWithoutConsentDoesNotAccessMemory(t *testing.T) {
	// A nil channel would deadlock if conversation persistence was triggered.
	store := &memoryStoreStub{seen: nil}
	config := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Character: domain.Character{ID: 9, Name: "Demo", Persona: "Friendly"},
	}}
	u := NewChatUsecase(config, &fakeChatModel{}).WithMemory(
		NewMemoryService(store, &fakeChatModel{}),
	)
	out, err := u.Chat(context.Background(), ChatInput{
		DeviceTypeID: 7, Message: "hello",
	})
	if err != nil || out.MemoryQueued || store.scope != (memory.Scope{}) {
		t.Fatalf("opt-out should not touch memory: %+v %v", out, err)
	}
}

func TestInvalidMemoryConsentIsRejected(t *testing.T) {
	config := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Character: domain.Character{ID: 9, Name: "Demo", Persona: "Friendly"},
	}}
	u := NewChatUsecase(config, &fakeChatModel{}).WithMemory(
		NewMemoryService(&memoryStoreStub{}, &fakeChatModel{}),
	)
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

func TestPreferenceExtractionRejectsSensitiveAndUnknownFields(t *testing.T) {
	raw := `{"facts":[{"key":"favorite_game","value":"桌游"},` +
		`{"key":"phone","value":"13812345678"},` +
		`{"key":"likes","value":"12345678"},` +
		`{"key":"favorite_story","value":"童话"}]}`
	facts := parsePreferenceFacts(raw)
	if len(facts) != 2 || facts[0].Value != "桌游" || facts[1].Value != "童话" {
		t.Fatalf("unexpected extracted preferences: %+v", facts)
	}
	if found := parsePreferenceFacts("not-json"); len(found) != 0 {
		t.Fatalf("invalid JSON must not become memory: %+v", found)
	}
}
