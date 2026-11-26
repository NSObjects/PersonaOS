package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
)

type schedulerStoreStub struct {
	*consolidationStoreStub
	scopes []memory.PendingScope
	after  memory.Scope
}

func (s *schedulerStoreStub) ListReadyScopes(_ context.Context, after memory.Scope, now time.Time, _ int) ([]memory.PendingScope, error) {
	s.after = after
	ready := make([]memory.PendingScope, 0, len(s.scopes))
	for _, item := range s.scopes {
		if item.UserChars >= memory.ConsolidationThreshold ||
			(item.UserChars >= memory.ConsolidationIdleMinimum && !item.LastPendingAt.After(now.Add(-memory.ConsolidationIdleAge))) {
			ready = append(ready, item)
		}
	}
	return ready, nil
}

func TestMemorySchedulerWaitsForNewWorkAfterFailure(t *testing.T) {
	now := time.Now()
	scope := memory.Scope{OwnerID: 1, CharacterID: 2}
	store := &schedulerStoreStub{
		consolidationStoreStub: consolidationFixture(),
		scopes:                 []memory.PendingScope{{Scope: scope, LastPendingAt: now, UserChars: memory.ConsolidationIdleMinimum}},
	}
	store.batch.Exchanges[0].User = strings.Repeat("甲", memory.ConsolidationIdleMinimum)
	extractor := &consolidationModelStub{result: `{"action": "noop"}`}
	resolutions := 0
	scheduler := NewMemoryScheduler(store, func(context.Context) (model.BaseChatModel, error) {
		resolutions++
		return extractor, nil
	})
	ctx := context.Background()
	if err := scheduler.scan(ctx, now); err != nil || resolutions != 0 {
		t.Fatalf("small active batch triggered model: resolutions=%d err=%v", resolutions, err)
	}
	extractor.err = errors.New("provider unavailable")
	if err := scheduler.scan(ctx, now.Add(memory.ConsolidationIdleAge)); err != nil || extractor.calls != 1 ||
		len(store.claimed) != 1 || len(store.batch.Exchanges) != 0 || len(store.committed) != 0 {
		t.Fatalf("failed batch not consumed once: calls=%d err=%v", extractor.calls, err)
	}
	extractor.err = nil
	for _, elapsed := range []time.Duration{time.Minute, time.Hour, 24 * time.Hour} {
		if err := scheduler.scan(ctx, now.Add(memory.ConsolidationIdleAge+elapsed)); err != nil || extractor.calls != 1 {
			t.Fatalf("failed batch repeated: calls=%d err=%v", extractor.calls, err)
		}
	}
	store.batch.Exchanges = []memory.PendingExchange{{ID: 12, Exchange: memory.Exchange{
		User: strings.Repeat("学", memory.ConsolidationThreshold),
	}}}
	store.scopes[0].UserChars = memory.ConsolidationThreshold
	if err := scheduler.scan(ctx, now.Add(25*time.Hour)); err != nil || extractor.calls != 2 ||
		len(store.claimed) != 1 || store.claimed[0] != 12 || len(store.batch.Exchanges) != 0 {
		t.Fatalf("new eligible work not handled: calls=%d err=%v", extractor.calls, err)
	}
}

func TestMemorySchedulerAdvancesPageAndRestartsScan(t *testing.T) {
	now := time.Now()
	store := &schedulerStoreStub{
		consolidationStoreStub: consolidationFixture(),
		scopes:                 make([]memory.PendingScope, 0, memoryScopePage),
	}
	for i := range memoryScopePage {
		store.scopes = append(store.scopes, memory.PendingScope{
			Scope: memory.Scope{OwnerID: int64(i + 1), CharacterID: 1}, LastPendingAt: now, UserChars: memory.ConsolidationThreshold,
		})
	}
	store.batch.Exchanges = nil
	last := store.scopes[len(store.scopes)-1].Scope
	scheduler := NewMemoryScheduler(store, func(context.Context) (model.BaseChatModel, error) {
		t.Error("empty batch unexpectedly resolved model")
		return nil, errors.New("unexpected resolution")
	})
	ctx := context.Background()
	if err := scheduler.scan(ctx, now); err != nil || scheduler.after != last {
		t.Fatalf("page cursor=%+v err=%v", scheduler.after, err)
	}
	store.scopes = nil
	if err := scheduler.scan(ctx, now); err != nil || store.after != last || scheduler.after != (memory.Scope{}) {
		t.Fatalf("scan did not wrap: previous=%+v next=%+v err=%v", store.after, scheduler.after, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := scheduler.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled scheduler returned %v", err)
	}
}

func TestMemorySchedulerSkipsTinyIdleScopes(t *testing.T) {
	now := time.Now()
	store := &schedulerStoreStub{
		consolidationStoreStub: consolidationFixture(),
		scopes:                 []memory.PendingScope{{
			Scope:         memory.Scope{OwnerID: 1, CharacterID: 2},
			LastPendingAt: now.Add(-24 * time.Hour),
			UserChars:     memory.ConsolidationIdleMinimum - 1,
		}},
	}
	scheduler := NewMemoryScheduler(store, func(context.Context) (model.BaseChatModel, error) {
		t.Error("tiny idle batch resolved model")
		return nil, errors.New("unexpected model resolution")
	})
	if err := scheduler.scan(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(store.claimed) != 0 {
		t.Fatal("tiny idle batch was acknowledged")
	}
}

