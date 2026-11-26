package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
)

type coreCacheStoreStub struct {
	*schedulerStoreStub
	values  map[memory.Scope]memory.CoreMemory
	reads   int
	readErr error
}

func (s *coreCacheStoreStub) ReadCore(_ context.Context, scope memory.Scope) (memory.CoreMemory, error) {
	s.reads++
	return s.values[scope], s.readErr
}

func (s *coreCacheStoreStub) PublishCore(_ context.Context, scope memory.Scope, _ uint64, next memory.CoreMemory) error {
	if s.commitErr != nil {
		return s.commitErr
	}
	s.values[scope] = next
	return nil
}

func coreCacheFixture() (*coreCacheStoreStub, *CachedMemoryStore) {
	store := &coreCacheStoreStub{
		schedulerStoreStub: &schedulerStoreStub{consolidationStoreStub: consolidationFixture()},
		values:             make(map[memory.Scope]memory.CoreMemory),
	}
	return store, NewCachedMemoryStore(store)
}

func assertCachedCore(t *testing.T, cache *CachedMemoryStore, scope memory.Scope, want memory.CoreMemory) {
	t.Helper()
	got, err := cache.ReadCore(context.Background(), scope)
	if err != nil || got != want {
		t.Fatalf("core=%+v want=%+v err=%v", got, want, err)
	}
}

func TestCoreCacheIsScopedAndInvalidatesOnPublication(t *testing.T) {
	store, cache := coreCacheFixture()
	ctx := context.Background()
	scope := memory.Scope{OwnerID: 1, CharacterID: 2}
	other := memory.Scope{OwnerID: 2, CharacterID: 2}
	old := store.batch.Core.CoreMemory
	store.values[scope] = old
	assertCachedCore(t, cache, scope, old)
	assertCachedCore(t, cache, scope, old)
	assertCachedCore(t, cache, other, memory.CoreMemory{})
	assertCachedCore(t, cache, other, memory.CoreMemory{})
	if store.reads != 2 {
		t.Fatalf("cache did not reuse scoped reads: %d", store.reads)
	}
	next := memory.CoreMemory{Stable: "喜欢阅读"}
	store.commitErr = errors.New("publication failed")
	if err := cache.PublishCore(ctx, scope, 1, next); err == nil {
		t.Fatal("expected publication failure")
	}
	assertCachedCore(t, cache, scope, old)
	if store.reads != 2 {
		t.Fatal("failed publication invalidated cache")
	}
	store.commitErr = nil
	if err := cache.PublishCore(ctx, scope, 1, next); err != nil {
		t.Fatal(err)
	}
	assertCachedCore(t, cache, scope, next)
	assertCachedCore(t, cache, other, memory.CoreMemory{})
	if store.reads != 3 {
		t.Fatalf("publication invalidated wrong scope: %d", store.reads)
	}
}

func TestCoreCacheBoundsEntriesAndRefreshesExpiredValues(t *testing.T) {
	store, cache := coreCacheFixture()
	scope := memory.Scope{OwnerID: 1, CharacterID: 1}
	ctx := context.Background()
	store.readErr = errors.New("lookup failed")
	if _, err := cache.ReadCore(ctx, scope); err == nil || len(cache.entries) != 0 {
		t.Fatal("failed read was cached")
	}
	store.readErr = nil
	for i := range coreCacheCapacity + 1 {
		assertCachedCore(t, cache, memory.Scope{OwnerID: 1, CharacterID: int64(i + 1)}, memory.CoreMemory{})
	}
	if len(cache.entries) != coreCacheCapacity {
		t.Fatalf("unbounded cache: %d", len(cache.entries))
	}
	cache.entries[scope] = coreCacheEntry{expiresAt: time.Now().Add(-time.Second)}
	next := memory.CoreMemory{Dynamic: "正在学画画"}
	store.values[scope] = next
	assertCachedCore(t, cache, scope, next)
}

func (s *coreCacheStoreStub) ClearMemory(_ context.Context, scope memory.Scope) error {
	if s.commitErr != nil {
		return s.commitErr
	}
	delete(s.values, scope)
	return nil
}

func TestCoreCacheClearInvalidatesOnlyAfterCommit(t *testing.T) {
	store, cache := coreCacheFixture()
	ctx := context.Background()
	scope := memory.Scope{OwnerID: 3, CharacterID: 1}
	other := memory.Scope{OwnerID: 3, CharacterID: 2}
	old := store.batch.Core.CoreMemory
	store.values[scope], store.values[other] = old, old
	assertCachedCore(t, cache, scope, old)
	assertCachedCore(t, cache, other, old)
	store.commitErr = errors.New("clear failed")
	if err := cache.ClearMemory(ctx, scope); err == nil {
		t.Fatal("expected clear failure")
	}
	assertCachedCore(t, cache, scope, old)
	if store.reads != 2 {
		t.Fatal("failed clear invalidated cache")
	}
	store.commitErr = nil
	if err := cache.ClearMemory(ctx, scope); err != nil {
		t.Fatal(err)
	}
	assertCachedCore(t, cache, scope, memory.CoreMemory{})
	assertCachedCore(t, cache, other, old)
	if store.reads != 3 {
		t.Fatal("clear invalidated wrong scope")
	}
}
