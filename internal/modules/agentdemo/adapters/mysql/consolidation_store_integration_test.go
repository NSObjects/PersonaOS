package mysql_test

import (
	"context"
	"strings"
	"testing"
	"time"

	memorymysql "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/adapters/mysql"
	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

const pendingFixtureSessionID = "73c91b65-c860-4165-9efd-dd86a20fd0a7"

func readPendingBatch(t *testing.T, store *memorymysql.Store, scope memory.Scope, want int) memory.PendingBatch {
	t.Helper()
	batch, err := store.ReadPending(context.Background(), scope, memory.MaxPendingBatchSize)
	if err != nil || len(batch.Exchanges) != want {
		t.Fatalf("pending exchanges=%d, want %d, err=%v", len(batch.Exchanges), want, err)
	}
	return batch
}

func testPendingConsolidationMySQL(t *testing.T, store *memorymysql.Store, first, other memory.Scope) {
	t.Helper()
	ctx := context.Background()
	readPendingBatch(t, store, first, 0)
	userText := strings.Repeat("学", memory.ConsolidationThreshold/2)
	for _, scope := range []memory.Scope{first, first, other} {
		turn := memory.Exchange{
			Scope: scope, SessionID: pendingFixtureSessionID,
			User: userText, Assistant: "我们可以一起学习",
		}
		if err := store.AppendPending(ctx, turn); err != nil {
			t.Fatal(err)
		}
	}
	assertPendingScopeMySQL(t, store, memory.Scope{OwnerID: first.OwnerID}, first)
	assertPendingScopeMySQL(t, store, first, other)
	assertReadyScopeThresholdMySQL(t, store, first, other)
	assertRetryScopeMySQL(t, store, first, other)
	batch := readPendingBatch(t, store, first, 2)
	otherBatch := readPendingBatch(t, store, other, 1)
	ids := []int64{batch.Exchanges[0].ID, batch.Exchanges[1].ID}
	if ids[0] >= ids[1] || batch.Exchanges[0].User != userText || batch.Exchanges[0].Scope != first {
		t.Fatalf("unexpected pending batch: %+v", batch)
	}
	next := memory.CoreMemory{Stable: "用户喜欢围棋", Dynamic: "正在学习围棋"}
	assertRejectedConsolidations(t, store, first, batch.Core.Version, ids, otherBatch.Exchanges[0].ID, next)
	readPendingBatch(t, store, first, 2)
	if err := store.CommitPending(ctx, first, batch.Core.Version, ids[:1], next); err != nil {
		t.Fatal(err)
	}
	current := readPendingBatch(t, store, first, 1)
	if current.Core.CoreMemory != next || current.Core.Version != batch.Core.Version+1 ||
		current.Core.ThroughID != ids[0] || current.Exchanges[0].ID != ids[1] {
		t.Fatalf("unexpected committed state: %+v", current)
	}
	if err := store.CommitPending(ctx, first, batch.Core.Version, ids[1:], next); err == nil {
		t.Fatal("stale revision accepted")
	}
	if err := store.CommitPending(ctx, first, current.Core.Version, ids[1:], current.Core.CoreMemory); err != nil {
		t.Fatal(err)
	}
	final := readPendingBatch(t, store, first, 0)
	if final.Core.CoreMemory != next || final.Core.ThroughID != ids[1] || final.Core.Version != current.Core.Version+1 {
		t.Fatalf("noop consolidation changed memory: %+v", final)
	}
	readPendingBatch(t, store, other, 1)
	assertPendingScopeMySQL(t, store, memory.Scope{OwnerID: first.OwnerID}, other)
}

func assertRejectedConsolidations(t *testing.T, store *memorymysql.Store, first memory.Scope, version uint64, ids []int64, otherID int64, next memory.CoreMemory) {
	t.Helper()
	ctx := context.Background()
	for _, invalid := range [][]int64{ids[1:], {otherID}} {
		err := store.CommitPending(ctx, first, version, invalid, next)
		if code, ok := apperr.Parse(err); !ok || code.Code() != apperr.ErrConflict {
			t.Fatalf("invalid prefix accepted: %v", err)
		}
	}
	oversize := memory.CoreMemory{Stable: strings.Repeat("甲", memory.MaxCoreMemoryLength+1)}
	if err := store.CommitPending(ctx, first, version, ids, oversize); err == nil {
		t.Fatal("oversized memory accepted")
	}
}

func assertPendingScopeMySQL(t *testing.T, store *memorymysql.Store, after, want memory.Scope) {
	t.Helper()
	page, err := store.ListReadyScopes(context.Background(), after, time.Now().Add(time.Hour), 1)
	if err != nil || len(page) != 1 || page[0].Scope != want || page[0].LastPendingAt.IsZero() || page[0].UserChars < memory.ConsolidationIdleMinimum {
		t.Fatalf("pending scope page=%+v want=%+v err=%v", page, want, err)
	}
}

func assertReadyScopeThresholdMySQL(t *testing.T, store *memorymysql.Store, first, other memory.Scope) {
	t.Helper()
	ctx := context.Background()
	tiny := memory.Scope{OwnerID: first.OwnerID, CharacterID: other.CharacterID + 1}
	if err := store.AppendPending(ctx, memory.Exchange{
		Scope: tiny, SessionID: pendingFixtureSessionID,
		User: "您好", Assistant: strings.Repeat("讲", memory.ConsolidationThreshold),
	}); err != nil {
		t.Fatal(err)
	}
	after := memory.Scope{OwnerID: first.OwnerID}
	active, err := store.ListReadyScopes(ctx, after, time.Now().Add(-5*time.Minute), 3)
	if err != nil || len(active) != 1 || active[0].Scope != first || active[0].UserChars != memory.ConsolidationThreshold {
		t.Fatalf("active threshold: %+v err=%v", active, err)
	}
	idle, err := store.ListReadyScopes(ctx, after, time.Now().Add(time.Hour), 3)
	if err != nil || len(idle) != 2 || idle[0].Scope != first || idle[1].Scope != other {
		t.Fatalf("idle threshold: %+v err=%v", idle, err)
	}
}

func assertRetryScopeMySQL(t *testing.T, store *memorymysql.Store, first, other memory.Scope) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	after := memory.Scope{OwnerID: first.OwnerID}
	page, err := store.ListReadyScopes(ctx, after, now, 3)
	if err != nil || len(page) != 1 || page[0].Scope != first {
		t.Fatalf("ready before retry: %+v err=%v", page, err)
	}
	item := page[0]
	retryAt := now.Add(time.Minute)
	if err := store.DeferPendingScope(ctx, item, retryAt); err != nil {
		t.Fatal(err)
	}
	page, err = store.ListReadyScopes(ctx, after, now, 3)
	if err != nil || len(page) != 0 {
		t.Fatalf("retry cooldown was ignored: %+v err=%v", page, err)
	}
	page, err = store.ListReadyScopes(ctx, after, retryAt.Add(time.Second), 3)
	if err != nil || len(page) != 1 || page[0].Scope != first || page[0].RetryCount != 1 {
		t.Fatalf("deferred scope did not become eligible: %+v err=%v", page, err)
	}
	// The other scope is independent and will become ready on idle.
	page, err = store.ListReadyScopes(ctx, first, now.Add(time.Hour), 1)
	if err != nil || len(page) != 1 || page[0].Scope != other || page[0].RetryCount != 0 {
		t.Fatalf("retry escaped scope: %+v err=%v", page, err)
	}
}
