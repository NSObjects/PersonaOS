package usecase

import (
	"context"
	"testing"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
)

func TestCoreMemoryManagementClearsOnlyAuthenticatedScope(t *testing.T) {
	store, cache := coreCacheFixture()
	scope := memory.Scope{OwnerID: 31, CharacterID: 9}
	other := memory.Scope{OwnerID: 32, CharacterID: 9}
	old := store.batch.Core.CoreMemory
	store.values[scope], store.values[other] = old, old
	service := NewMemoryService(cache)
	cfg := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Character: domain.Character{ID: scope.CharacterID, Name: "Core Demo"},
	}}
	u := NewChatUsecase(cfg, nil).WithMemory(service)
	ctx := requestctx.WithUserID(context.Background(), scope.OwnerID)
	snapshot, err := u.InspectMemory(ctx, 7)
	if err != nil || snapshot.Stable != old.Stable || snapshot.Dynamic != old.Dynamic {
		t.Fatalf("core snapshot=%+v err=%v", snapshot, err)
	}
	const clearKey = "all"
	if err := u.DeleteMemoryFact(context.Background(), 7, clearKey); err == nil {
		t.Fatal("unauthenticated core clear accepted")
	}
	_, done, ok := service.reserveWrite(scope)
	if !ok {
		t.Fatal("could not reserve pending write")
	}
	err = u.DeleteMemoryFact(ctx, 7, clearKey)
	if code, ok := apperr.Parse(err); !ok || code.Code() != apperr.ErrConflict {
		t.Fatalf("clear raced pending write: %v", err)
	}
	service.releaseWrite(scope, done)
	if err := u.DeleteMemoryFact(ctx, 7, clearKey); err != nil {
		t.Fatal(err)
	}
	snapshot, err = u.InspectMemory(ctx, 7)
	if err != nil || snapshot.Stable != "" || snapshot.Dynamic != "" || store.values[other] != old {
		t.Fatalf("clear escaped scope or retained memory: %+v err=%v", snapshot, err)
	}
}
