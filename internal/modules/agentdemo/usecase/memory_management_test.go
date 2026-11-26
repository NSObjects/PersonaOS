package usecase

import (
	"context"
	"testing"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
)

func TestMemoryManagementDerivesScopeAndRejectsPendingExtraction(t *testing.T) {
	const key = "favorite_color"
	store := &memoryStoreStub{facts: []memory.Fact{{Key: key, Value: "purple"}}}
	service := NewMemoryService(store, nil)
	config := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Character: domain.Character{ID: 9, Name: "Memory Demo"},
	}}
	u := NewChatUsecase(config, nil).WithMemory(service)
	ctx := requestctx.WithUserID(context.Background(), 31)
	scope := memory.Scope{OwnerID: 31, CharacterID: 9}
	snapshot, err := u.InspectMemory(ctx, 7)
	if err != nil || snapshot.CharacterID != 9 || len(snapshot.Facts) != 1 || store.scope != scope {
		t.Fatalf("snapshot=%+v scope=%+v err=%v", snapshot, store.scope, err)
	}
	if _, err := u.InspectMemory(context.Background(), 7); err == nil {
		t.Fatal("unauthenticated memory access")
	}
	_, done, ok := service.reserveExtraction(scope)
	if !ok {
		t.Fatal("could not reserve extraction")
	}
	err = u.DeleteMemoryFact(ctx, 7, key)
	if code, ok := apperr.Parse(err); !ok || code.Code() != apperr.ErrConflict {
		t.Fatalf("deletion during extraction: %v", err)
	}
	service.releaseExtraction(scope, done)
	if err := u.DeleteMemoryFact(ctx, 7, key); err != nil {
		t.Fatal(err)
	}
	snapshot, err = u.InspectMemory(ctx, 7)
	if err != nil || len(snapshot.Facts) != 0 || store.scope != scope {
		t.Fatalf("after delete snapshot=%+v err=%v", snapshot, err)
	}
}

func TestCoreMemoryManagementClearsOnlyAuthenticatedScope(t *testing.T) {
	store, cache := coreCacheFixture()
	scope := memory.Scope{OwnerID: 31, CharacterID: 9}
	other := memory.Scope{OwnerID: 32, CharacterID: 9}
	old := store.batch.Core.CoreMemory
	store.values[scope], store.values[other] = old, old
	service := NewCoreMemoryService(&memoryStoreStub{}, cache)
	cfg := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Character: domain.Character{ID: scope.CharacterID, Name: "Core Demo"},
	}}
	u := NewChatUsecase(cfg, nil).WithMemory(service)
	ctx := requestctx.WithUserID(context.Background(), scope.OwnerID)
	snapshot, err := u.InspectMemory(ctx, 7)
	if err != nil || !snapshot.CoreMode || snapshot.Stable != old.Stable || snapshot.Dynamic != old.Dynamic {
		t.Fatalf("core snapshot=%+v err=%v", snapshot, err)
	}
	const clearKey = "all"
	if err := u.DeleteMemoryFact(context.Background(), 7, clearKey); err == nil {
		t.Fatal("unauthenticated core clear accepted")
	}
	_, done, ok := service.reserveExtraction(scope)
	if !ok {
		t.Fatal("could not reserve pending write")
	}
	err = u.DeleteMemoryFact(ctx, 7, clearKey)
	if code, ok := apperr.Parse(err); !ok || code.Code() != apperr.ErrConflict {
		t.Fatalf("clear raced pending write: %v", err)
	}
	service.releaseExtraction(scope, done)
	if err := u.DeleteMemoryFact(ctx, 7, clearKey); err != nil {
		t.Fatal(err)
	}
	snapshot, err = u.InspectMemory(ctx, 7)
	if err != nil || snapshot.Stable != "" || snapshot.Dynamic != "" || store.values[other] != old {
		t.Fatalf("clear escaped scope or retained memory: %+v err=%v", snapshot, err)
	}
}
