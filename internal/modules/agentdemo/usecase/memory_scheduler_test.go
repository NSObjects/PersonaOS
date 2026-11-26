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

func (s *schedulerStoreStub) ListReadyScopes(_ context.Context, after memory.Scope, idleBefore time.Time, _ int) ([]memory.PendingScope, error) {
	s.after = after
	ready := make([]memory.PendingScope, 0, len(s.scopes))
	for _, item := range s.scopes {
		if item.UserChars >= memory.ConsolidationThreshold ||
			(item.UserChars >= memory.ConsolidationIdleMinimum && !item.LastPendingAt.After(idleBefore)) {
			ready = append(ready, item)
		}
	}
	return ready, nil
}

func TestMemorySchedulerFlushesIdleBatchAndRetriesFailedCommit(t *testing.T) {
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
	if err := scheduler.scan(ctx, now); err != nil || resolutions != 0 || extractor.calls != 0 {
		t.Fatalf("small active batch triggered model: resolutions=%d err=%v", resolutions, err)
	}
	store.commitErr = errors.New("fixture commit failure")
	if err := scheduler.scan(ctx, now.Add(memoryIdleFlush)); err != nil || extractor.calls != 1 || len(store.committed) != 0 {
		t.Fatalf("failed idle flush acknowledged: calls=%d err=%v", extractor.calls, err)
	}
	store.commitErr = nil
	if err := scheduler.scan(ctx, now.Add(memoryIdleFlush+memoryPollInterval)); err != nil || extractor.calls != 2 ||
		resolutions != 2 || len(store.committed) != 1 {
		t.Fatalf("pending batch was not retried: calls=%d err=%v", extractor.calls, err)
	}
	// Even a stale ready-scope discovery cannot resolve a model after all
	// pending exchanges have been acknowledged.
	store.batch.Exchanges = nil
	if err := scheduler.scan(ctx, now.Add(2*memoryIdleFlush)); err != nil || extractor.calls != 2 {
		t.Fatalf("empty pending batch triggered another model: calls=%d err=%v", extractor.calls, err)
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
			Scope: memory.Scope{OwnerID: 1, CharacterID: 2}, LastPendingAt: now.Add(-24 * time.Hour),
			UserChars: memory.ConsolidationIdleMinimum - 1,
		}},
	}
	scheduler := NewMemoryScheduler(store, func(context.Context) (model.BaseChatModel, error) {
		t.Error("tiny idle batch resolved model")
		return nil, errors.New("unexpected model resolution")
	})
	if err := scheduler.scan(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(store.committed) != 0 {
		t.Fatal("tiny idle batch was acknowledged")
	}
}
