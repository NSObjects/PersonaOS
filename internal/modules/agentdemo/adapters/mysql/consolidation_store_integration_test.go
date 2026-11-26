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
	assertPublishedCoreMySQL(t, store, first, memory.CoreMemory{})
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
	batch := readPendingBatch(t, store, first, 2)
	otherBatch := readPendingBatch(t, store, other, 1)
	ids := []int64{batch.Exchanges[0].ID, batch.Exchanges[1].ID}
	if ids[0] >= ids[1] || batch.Exchanges[0].User != userText || batch.Exchanges[0].Scope != first {
		t.Fatalf("unexpected pending batch: %+v", batch)
	}
	next := memory.CoreMemory{Stable: "用户喜欢围棋", Dynamic: "正在学习围棋"}
	assertRejectedClaims(t, store, first, batch.Core.Version, ids, otherBatch.Exchanges[0].ID)
	readPendingBatch(t, store, first, 2)
	if err := store.ClaimPending(ctx, first, batch.Core.Version, ids[:1]); err != nil {
		t.Fatal(err)
	}
	current := readPendingBatch(t, store, first, 1)
	if current.Core.CoreMemory != batch.Core.CoreMemory || current.Core.Version != batch.Core.Version+1 ||
		current.Core.ThroughID != ids[0] || current.Exchanges[0].ID != ids[1] {
		t.Fatalf("unexpected claimed state: %+v", current)
	}
	assertClaimPublicationMySQL(t, store, first, current, next, ids[1])
	readPendingBatch(t, store, other, 1)
	assertPendingScopeMySQL(t, store, memory.Scope{OwnerID: first.OwnerID}, other)
}

func assertRejectedClaims(t *testing.T, store *memorymysql.Store, first memory.Scope, version uint64, ids []int64, otherID int64) {
	t.Helper()
	ctx := context.Background()
	for _, invalid := range [][]int64{ids[1:], {otherID}} {
		err := store.ClaimPending(ctx, first, version, invalid)
		if code, ok := apperr.Parse(err); !ok || code.Code() != apperr.ErrConflict {
			t.Fatalf("invalid prefix accepted: %v", err)
		}
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

func assertClaimPublicationMySQL(t *testing.T, store *memorymysql.Store, scope memory.Scope, claimed memory.PendingBatch, next memory.CoreMemory, remainingID int64) {
	t.Helper()
	ctx := context.Background()
	oversize := memory.CoreMemory{Stable: strings.Repeat("甲", memory.MaxCoreMemoryLength+1)}
	if err := store.PublishCore(ctx, scope, claimed.Core.Version, oversize); err == nil {
		t.Fatal("oversized memory accepted")
	}
	if err := store.PublishCore(ctx, scope, claimed.Core.Version, next); err != nil {
		t.Fatal(err)
	}
	current := readPendingBatch(t, store, scope, 1)
	assertPublishedCoreMySQL(t, store, scope, next)
	if current.Core.CoreMemory != next || current.Core.Version != claimed.Core.Version+1 {
		t.Fatalf("unexpected published state: %+v", current)
	}
	if err := store.ClaimPending(ctx, scope, claimed.Core.Version, []int64{remainingID}); err == nil {
		t.Fatal("stale claim accepted")
	}
	if err := store.ClaimPending(ctx, scope, current.Core.Version, []int64{remainingID}); err != nil {
		t.Fatal(err)
	}
	// No publication simulates a failed model call. A fresh store read sees
	// no queued work and retains the last successfully published memory.
	final := readPendingBatch(t, store, scope, 0)
	assertPublishedCoreMySQL(t, store, scope, next)
	if final.Core.CoreMemory != next || final.Core.ThroughID != remainingID || final.Core.Version != current.Core.Version+1 {
		t.Fatalf("failed attempt changed core memory: %+v", final)
	}
	if err := store.PublishCore(ctx, scope, claimed.Core.Version, memory.CoreMemory{}); err == nil {
		t.Fatal("stale memory publication accepted")
	}
}

func assertPublishedCoreMySQL(t *testing.T, store *memorymysql.Store, scope memory.Scope, want memory.CoreMemory) {
	t.Helper()
	got, err := store.ReadCore(context.Background(), scope)
	if err != nil || got != want {
		t.Fatalf("published core=%+v want=%+v err=%v", got, want, err)
	}
}
