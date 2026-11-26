package usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
)

type consolidationStoreStub struct {
	batch     memory.PendingBatch
	committed []int64
	next      memory.CoreMemory
	version   uint64
	commitErr error
}

func (*consolidationStoreStub) AppendPending(context.Context, memory.Exchange) error {
	return errors.New("unexpected append")
}

func (s *consolidationStoreStub) ReadPending(context.Context, memory.Scope, int) (memory.PendingBatch, error) {
	return s.batch, nil
}

func (s *consolidationStoreStub) CommitPending(_ context.Context, _ memory.Scope, version uint64, ids []int64, next memory.CoreMemory) error {
	if s.commitErr != nil {
		return s.commitErr
	}
	s.committed, s.next, s.version = slices.Clone(ids), next, version
	return nil
}

type consolidationModelStub struct {
	fakeChatModel
	result string
	calls  int
}

func (m *consolidationModelStub) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.calls++
	m.last = messages
	if m.err != nil {
		return nil, m.err
	}
	return schema.AssistantMessage(m.result, nil), nil
}

func consolidationFixture() *consolidationStoreStub {
	return &consolidationStoreStub{batch: memory.PendingBatch{
		Core: memory.CoreState{
			CoreMemory: memory.CoreMemory{Stable: "喜欢围棋", Dynamic: "正在练习"},
			Version:    7,
		},
		Exchanges: []memory.PendingExchange{{ID: 11, Exchange: memory.Exchange{
			User: "今天下雨了", Assistant: "出门记得带伞",
		}}},
	}}
}

func TestConsolidationThresholdAndForcedNoop(t *testing.T) {
	store := consolidationFixture()
	extractor := &consolidationModelStub{result: `{"action":"noop"}`}
	c := NewMemoryConsolidator(store)
	ctx := context.Background()
	scope := memory.Scope{OwnerID: 1, CharacterID: 2}
	done, err := c.Consolidate(ctx, scope, extractor, false)
	if err != nil || done || extractor.calls != 0 || len(store.committed) != 0 {
		t.Fatal("small pending batch must not call model")
	}
	done, err = c.Consolidate(ctx, scope, extractor, true)
	if err != nil || !done || extractor.calls != 1 || !slices.Equal(store.committed, []int64{11}) ||
		store.next != store.batch.Core.CoreMemory || store.version != store.batch.Core.Version {
		t.Fatalf("noop did not acknowledge unchanged memory: done=%t err=%v store=%+v", done, err, store)
	}
}

func TestConsolidationSelectsBoundedOrderedPrefix(t *testing.T) {
	store := consolidationFixture()
	store.batch.Exchanges = make([]memory.PendingExchange, 0, 3)
	for id := int64(1); id <= 3; id++ {
		store.batch.Exchanges = append(store.batch.Exchanges, memory.PendingExchange{
			ID: id, Exchange: memory.Exchange{
				User: strings.Repeat("甲", 1500), Assistant: strings.Repeat("🌟", 3000),
			},
		})
	}
	input, ids := selectConsolidationBatch(store.batch, false)
	if !slices.Equal(ids, []int64{1, 2}) || len(input.Dialogs) != 2 {
		t.Fatalf("incorrect selected prefix: %v", ids)
	}
	for _, turn := range input.Dialogs {
		if !utf8.ValidString(turn.Assistant) || utf8.RuneCountInString(turn.Assistant) != consolidationReplyLimit {
			t.Fatal("long reply excerpt is not bounded Unicode")
		}
	}
	extractor := &consolidationModelStub{result: `{"action":"update","stable":"喜欢围棋","dynamic":"完成练习"}`}
	done, err := NewMemoryConsolidator(store).Consolidate(context.Background(), memory.Scope{}, extractor, false)
	if err != nil || !done || extractor.calls != 1 || !slices.Equal(store.committed, ids) ||
		store.next.Stable != store.batch.Core.Stable || store.next.Dynamic != "完成练习" {
		t.Fatalf("valid update failed: done=%t err=%v next=%+v", done, err, store.next)
	}
}

func TestConsolidationRejectsInvalidResultsWithoutAcknowledging(t *testing.T) {
	for _, raw := range []string{
		"not JSON",
		`{"action":"clear"}`,
		`{"action":"update","stable":"不完整"}`,
		`{"action":"update","stable":"` + strings.Repeat("甲", memory.MaxCoreMemoryLength+1) + `","dynamic":""}`,
		`{"action":"update","stable":"联系邮箱 a@example.com","dynamic":""}`,
	} {
		store := consolidationFixture()
		extractor := &consolidationModelStub{result: raw}
		done, err := NewMemoryConsolidator(store).Consolidate(context.Background(), memory.Scope{}, extractor, true)
		if err == nil || done || len(store.committed) != 0 {
			t.Fatalf("invalid result acknowledged: %q", raw)
		}
	}
}

func TestConsolidationFailureDoesNotAcknowledge(t *testing.T) {
	failure := errors.New("fixture failure")
	for _, modelFailure := range []bool{true, false} {
		store := consolidationFixture()
		extractor := &consolidationModelStub{result: `{"action":"noop"}`}
		if modelFailure {
			extractor.err = failure
		} else {
			store.commitErr = failure
		}
		done, err := NewMemoryConsolidator(store).Consolidate(context.Background(), memory.Scope{}, extractor, true)
		if !errors.Is(err, failure) || done || len(store.committed) != 0 {
			t.Fatalf("failed consolidation acknowledged: %v", err)
		}
	}
}
