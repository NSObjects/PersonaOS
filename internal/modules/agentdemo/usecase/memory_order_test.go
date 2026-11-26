package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
)

const (
	orderedFirst  = "蓝色"
	orderedSecond = "橙色"
	orderedOther  = "绿色"
)

type orderedMemoryStore struct {
	*coreCacheStoreStub
	started chan memory.Exchange
	writes  chan memory.Exchange
	release chan struct{}
}

func (s *orderedMemoryStore) AppendPending(ctx context.Context, turn memory.Exchange) error {
	s.started <- turn
	if turn.User == orderedFirst {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.release:
		}
	}
	s.writes <- turn
	return nil
}

func receiveOrderedWrite(t *testing.T, writes <-chan memory.Exchange, scope memory.Scope, value string) {
	t.Helper()
	select {
	case got := <-writes:
		if got.Scope != scope || got.User != value {
			t.Fatalf("unexpected memory write: %+v; want scope=%+v value=%q", got, scope, value)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("memory write timed out")
	}
}

func TestMemoryWritesPreserveScopeOrderWithoutBlockingOtherScopes(t *testing.T) {
	base, _ := coreCacheFixture()
	store := &orderedMemoryStore{
		coreCacheStoreStub: base,
		started:            make(chan memory.Exchange, 3),
		writes:             make(chan memory.Exchange, 3),
		release:            make(chan struct{}),
	}
	defer func() {
		select {
		case <-store.release:
		default:
			close(store.release)
		}
	}()
	service := NewMemoryService(NewCachedMemoryStore(store))
	scope := memory.Scope{OwnerID: 1, CharacterID: 9}
	other := memory.Scope{OwnerID: 2, CharacterID: 9}
	if !service.Remember(memory.Exchange{Scope: scope, User: orderedFirst}) {
		t.Fatal("first memory job not queued")
	}
	receiveOrderedWrite(t, store.started, scope, orderedFirst)
	if !service.Remember(memory.Exchange{Scope: scope, User: orderedSecond}) ||
		!service.Remember(memory.Exchange{Scope: other, User: orderedOther}) {
		t.Fatal("subsequent memory jobs not queued")
	}
	receiveOrderedWrite(t, store.writes, other, orderedOther)
	close(store.release)
	receiveOrderedWrite(t, store.writes, scope, orderedFirst)
	receiveOrderedWrite(t, store.writes, scope, orderedSecond)
}
