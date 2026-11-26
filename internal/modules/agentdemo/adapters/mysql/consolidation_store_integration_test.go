package mysql_test

import (
	"context"
	"strings"
	"testing"

	memorymysql "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/adapters/mysql"
	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

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
	for _, scope := range []memory.Scope{first, first, other} {
		turn := memory.Exchange{
			Scope: scope, SessionID: "73c91b65-c860-4165-9efd-dd86a20fd0a7",
			User: "我想学习围棋", Assistant: "我们可以一起学习",
		}
		if err := store.AppendPending(ctx, turn); err != nil {
			t.Fatal(err)
		}
	}
	batch := readPendingBatch(t, store, first, 2)
	otherBatch := readPendingBatch(t, store, other, 1)
	ids := []int64{batch.Exchanges[0].ID, batch.Exchanges[1].ID}
	if ids[0] >= ids[1] || batch.Exchanges[0].User != "我想学习围棋" || batch.Exchanges[0].Scope != first {
		t.Fatalf("unexpected pending batch: %+v", batch)
	}
	next := memory.CoreMemory{Stable: "用户喜欢围棋", Dynamic: "正在学习围棋"}
	for _, invalid := range [][]int64{ids[1:], {otherBatch.Exchanges[0].ID}} {
		err := store.CommitPending(ctx, first, batch.Core.Version, invalid, next)
		if code, ok := apperr.Parse(err); !ok || code.Code() != apperr.ErrConflict {
			t.Fatalf("invalid prefix accepted: %v", err)
		}
	}
	oversize := memory.CoreMemory{Stable: strings.Repeat("甲", memory.MaxCoreMemoryLength+1)}
	if err := store.CommitPending(ctx, first, batch.Core.Version, ids, oversize); err == nil {
		t.Fatal("oversized memory accepted")
	}
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
}
