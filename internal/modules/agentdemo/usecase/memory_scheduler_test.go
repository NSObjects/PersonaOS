package usecase

import (
	"context"
	"errors"
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

func (s *schedulerStoreStub) ListPendingScopes(_ context.Context, after memory.Scope, _ int) ([]memory.PendingScope, error) {
	s.after = after
	return s.scopes, nil
}

func TestMemorySchedulerFlushesIdleBatchAndRetriesFailedCommit(t *testing.T) {
	now := time.Now()
	scope := memory.Scope{OwnerID: 1, CharacterID: 2}
	store := &schedulerStoreStub{
		consolidationStoreStub: consolidationFixture(),
		scopes: []memory.PendingScope{{Scope: scope, LastPendingAt: now}},
	}
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
}

func TestMemorySchedulerAdvancesPageAndRestartsScan(t *testing.T) {
	now := time.Now()
	store := &schedulerStoreStub{
		consolidationStoreStub: consolidationFixture(),
		scopes: make([]memory.PendingScope, 0, memoryScopePage),
	}
	for i := range memoryScopePage {
		store.scopes = append(store.scopes, memory.PendingScope{
			Scope: memory.Scope{OwnerID: int64(i + 1), CharacterID: 1}, LastPendingAt: now,
		})
	}
	last := store.scopes[len(store.scopes)-1].Scope
	scheduler := NewMemoryScheduler(store, func(context.Context) (model.BaseChatModel, error) {
		t.Error("small batch unexpectedly resolved model")
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
