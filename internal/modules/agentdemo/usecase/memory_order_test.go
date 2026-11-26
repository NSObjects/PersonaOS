package usecase

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
)

const (
	orderedFirst  = "蓝色"
	orderedSecond = "橙色"
	orderedOther  = "绿色"
)

type orderedMemoryWrite struct {
	scope memory.Scope
	fact  memory.Fact
}

type orderedMemoryStore struct {
	memoryStoreStub
	writes chan orderedMemoryWrite
}

func (s *orderedMemoryStore) UpsertFacts(_ context.Context, scope memory.Scope, facts []memory.Fact) error {
	for _, fact := range facts {
		s.writes <- orderedMemoryWrite{scope: scope, fact: fact}
	}
	return nil
}

type orderedMemoryModel struct {
	fakeChatModel
	started chan string
	release chan struct{}
}

func (m *orderedMemoryModel) Generate(ctx context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	value := messages[len(messages)-1].Content
	m.started <- value
	if value == orderedFirst {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-m.release:
		}
	}
	return schema.AssistantMessage(fmt.Sprintf(`{"facts":[{"key":"favorite_color","value":%q}]}`, value), nil), nil
}

func receiveOrderedWrite(t *testing.T, writes <-chan orderedMemoryWrite, scope memory.Scope, value string) {
	t.Helper()
	select {
	case got := <-writes:
		if got.scope != scope || got.fact.Value != value {
			t.Fatalf("unexpected memory write: %+v; want scope=%+v value=%q", got, scope, value)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("memory write timed out")
	}
}

func TestMemoryExtractionPreservesScopeOrderWithoutBlockingOtherScopes(t *testing.T) {
	store := &orderedMemoryStore{
		memoryStoreStub: memoryStoreStub{seen: make(chan memory.Exchange, 3)},
		writes:          make(chan orderedMemoryWrite, 3),
	}
	extractor := &orderedMemoryModel{started: make(chan string, 3), release: make(chan struct{})}
	defer func() {
		select {
		case <-extractor.release:
		default:
			close(extractor.release)
		}
	}()
	service := NewMemoryService(store, extractor)
	scope := memory.Scope{OwnerID: 1, CharacterID: 9}
	other := memory.Scope{OwnerID: 2, CharacterID: 9}
	if !service.Remember(memory.Exchange{Scope: scope, User: orderedFirst}) {
		t.Fatal("first memory job not queued")
	}
	select {
	case got := <-extractor.started:
		if got != orderedFirst {
			t.Fatalf("unexpected first extraction: %s", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first extraction did not start")
	}
	if !service.Remember(memory.Exchange{Scope: scope, User: orderedSecond}) ||
		!service.Remember(memory.Exchange{Scope: other, User: orderedOther}) {
		t.Fatal("subsequent memory jobs not queued")
	}
	// The first extraction is stalled. Only the independent scope can write.
	receiveOrderedWrite(t, store.writes, other, orderedOther)
	close(extractor.release)
	receiveOrderedWrite(t, store.writes, scope, orderedFirst)
	receiveOrderedWrite(t, store.writes, scope, orderedSecond)
}
