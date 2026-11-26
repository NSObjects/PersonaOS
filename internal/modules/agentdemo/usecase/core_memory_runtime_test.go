package usecase

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
)

type coreRuntimeStoreStub struct {
	*coreCacheStoreStub
	queued chan memory.Exchange
}

func (s *coreRuntimeStoreStub) AppendPending(_ context.Context, turn memory.Exchange) error {
	s.queued <- turn
	return nil
}

type countedMemoryChatModel struct {
	fakeChatModel
	calls atomic.Int32
}

func (m *countedMemoryChatModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.calls.Add(1)
	return m.fakeChatModel.Generate(ctx, messages, opts...)
}

func TestAgentCoreMemoryReadsCacheAndQueuesWithoutExtraction(t *testing.T) {
	scope := memory.Scope{OwnerID: 41, CharacterID: 15}
	store, _ := coreCacheFixture()
	store.values[scope] = memory.CoreMemory{Stable: "喜欢陶艺", Dynamic: "正在准备展览"}
	pending := &coreRuntimeStoreStub{coreCacheStoreStub: store, queued: make(chan memory.Exchange, 2)}
	service := NewMemoryService(NewCachedMemoryStore(pending))
	cfg := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Character: domain.Character{ID: scope.CharacterID, Name: "陪伴角色", Persona: "自然"},
	}}
	dialog := &countedMemoryChatModel{}
	u := NewChatUsecase(cfg, dialog).WithMemory(service)
	ctx := requestctx.WithUserID(context.Background(), scope.OwnerID)
	in := ChatInput{
		DeviceTypeID: 1, Message: "今天很开心", Remember: true,
		SessionID: "6cd7b268-c143-4a8e-96e6-fb0872e3efcb",
	}
	for range 2 {
		out, err := u.Chat(ctx, in)
		if err != nil || !out.MemoryQueued {
			t.Fatalf("chat did not enqueue: %+v err=%v", out, err)
		}
		if !strings.Contains(dialog.last[0].Content, store.values[scope].Stable) ||
			!strings.Contains(dialog.last[0].Content, store.values[scope].Dynamic) {
			t.Fatal("published memory missing from agent instruction")
		}
		assertQueuedCoreExchange(t, pending.queued, scope, in, out.Reply)
	}
	if store.reads != 1 || dialog.calls.Load() != 2 {
		t.Fatalf("unexpected calls: database=%d model=%d", store.reads, dialog.calls.Load())
	}
}

func assertQueuedCoreExchange(t *testing.T, queued <-chan memory.Exchange, scope memory.Scope, in ChatInput, reply string) {
	t.Helper()
	select {
	case turn := <-queued:
		if turn.Scope != scope || turn.SessionID != in.SessionID || turn.User != in.Message || turn.Assistant != reply {
			t.Fatalf("incorrect pending exchange: %+v", turn)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending exchange was not stored")
	}
}

func TestCoreMemoryOptOutAndLookupFailure(t *testing.T) {
	store, _ := coreCacheFixture()
	store.readErr = errors.New("core lookup unavailable")
	pending := &coreRuntimeStoreStub{coreCacheStoreStub: store, queued: make(chan memory.Exchange, 1)}
	service := NewMemoryService(NewCachedMemoryStore(pending))
	cfg := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Character: domain.Character{ID: 15, Name: "角色"},
	}}
	u := NewChatUsecase(cfg, &fakeChatModel{}).WithMemory(service)
	in := ChatInput{DeviceTypeID: 1, Message: "早上好"}
	out, err := u.Chat(context.Background(), in)
	if err != nil || out.MemoryQueued || store.reads != 0 {
		t.Fatalf("opt-out touched memory: %+v err=%v", out, err)
	}
	in.Remember = true
	in.SessionID = "5085462f-2edc-420a-a4aa-6cee39d62295"
	scope := memory.Scope{OwnerID: 41, CharacterID: 15}
	out, err = u.Chat(requestctx.WithUserID(context.Background(), scope.OwnerID), in)
	if err != nil || !out.MemoryQueued || store.reads != 1 {
		t.Fatalf("core lookup failure blocked chat: %+v err=%v", out, err)
	}
	assertQueuedCoreExchange(t, pending.queued, scope, in, out.Reply)
}
