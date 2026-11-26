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
