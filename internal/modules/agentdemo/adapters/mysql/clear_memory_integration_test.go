package mysql_test

import (
	"context"
	"testing"

	"gorm.io/gorm"

	memorymysql "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/adapters/mysql"
	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
)

func testClearMemoryMySQL(t *testing.T, db *gorm.DB, store *memorymysql.Store, scope, other memory.Scope) {
	t.Helper()
	ctx := context.Background()
	turn := memory.Exchange{
		Scope: scope, SessionID: pendingFixtureSessionID,
		User: "喜欢画画", Assistant: "一起画画吧",
	}
	for range 2 {
		if err := store.AppendPending(ctx, turn); err != nil {
			t.Fatal(err)
		}
	}
	batch := readPendingBatch(t, store, scope, 2)
	if err := store.ClaimPending(ctx, scope, batch.Core.Version, []int64{batch.Exchanges[0].ID}); err != nil {
		t.Fatal(err)
	}
	inFlight := readPendingBatch(t, store, scope, 1)
	before := archivedMemoryCount(t, db, scope)
	if err := store.ClearMemory(ctx, scope); err != nil {
		t.Fatal(err)
	}
	after := readPendingBatch(t, store, scope, 0)
	if after.Core.Version != inFlight.Core.Version+1 || after.Core.CoreMemory != (memory.CoreMemory{}) {
		t.Fatalf("memory was not cleared: %+v", after)
	}
	if err := store.PublishCore(ctx, scope, inFlight.Core.Version, batch.Core.CoreMemory); err == nil {
		t.Fatal("stale model result survived clear")
	}
	assertEmptyScope(t, store, scope)
	if archivedMemoryCount(t, db, scope) != before {
		t.Fatal("clear removed archived conversations")
	}
	readPendingBatch(t, store, other, 1)
	if err := store.AppendPending(ctx, turn); err != nil {
		t.Fatal(err)
	}
	readPendingBatch(t, store, scope, 1)
}

func archivedMemoryCount(t *testing.T, db *gorm.DB, scope memory.Scope) int64 {
	t.Helper()
	var count int64
	if err := db.Table("conversation_messages").
		Where("owner_id = ? AND character_id = ?", scope.OwnerID, scope.CharacterID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}
