package mysql_test

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	memorymysql "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/adapters/mysql"
	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
)

func TestMemoryStoreMySQL(t *testing.T) {
	dsn := os.Getenv("PERSONAOS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set PERSONAOS_TEST_MYSQL_DSN for real MySQL integration")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store, err := memorymysql.NewStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	owner := time.Now().UnixNano()
	first := memory.Scope{OwnerID: owner, CharacterID: owner + 1}
	otherCharacter := memory.Scope{OwnerID: owner, CharacterID: owner + 2}
	otherOwner := memory.Scope{OwnerID: owner + 3, CharacterID: owner + 1}
	t.Cleanup(func() {
		for _, table := range []string{"memory_pending_exchanges", "memory_core_states", "conversation_messages", "memory_facts"} {
			err := db.Exec("DELETE FROM "+table+" WHERE owner_id = ? OR owner_id = ?", owner, otherOwner.OwnerID).Error
			if err != nil {
				t.Errorf("cleanup %s: %v", table, err)
			}
		}
	})
	err = store.SaveExchange(ctx, memory.Exchange{
		Scope: first, SessionID: "550e8400-e29b-41d4-a716-446655440000",
		User: "我喜欢积木", Assistant: "我们玩积木",
	})
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	err = db.Table("conversation_messages").
		Where("owner_id = ? AND character_id = ?", first.OwnerID, first.CharacterID).
		Count(&count).Error
	if err != nil || count != 2 {
		t.Fatalf("conversation rows = %d, error = %v", count, err)
	}
	for _, scope := range []memory.Scope{first, otherCharacter, otherOwner} {
		assertEmptyScope(t, store, scope)
	}
	if err := store.UpsertFacts(ctx, first, []memory.Fact{
		{Key: "favorite_game", Value: "积木"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFacts(ctx, first, []memory.Fact{
		{Key: "favorite_game", Value: "拼图"},
	}); err != nil {
		t.Fatal(err)
	}
	facts, err := store.ListFacts(ctx, first, 8)
	if err != nil || len(facts) != 1 || facts[0].Value != "拼图" {
		t.Fatalf("upsert facts = %+v, error = %v", facts, err)
	}
	for _, scope := range []memory.Scope{otherCharacter, otherOwner} {
		assertEmptyScope(t, store, scope)
	}
	testPendingConsolidationMySQL(t, store, first, otherCharacter)
	testClearMemoryMySQL(t, db, store, first, otherCharacter)
	testLegacyMemoryMigrationMySQL(t, store, otherOwner)
}

func TestDeleteMemoryFactMySQL(t *testing.T) {
	dsn := os.Getenv("PERSONAOS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("requires MySQL integration")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store, err := memorymysql.NewStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	owner := time.Now().UnixNano()
	scopes := []memory.Scope{
		{OwnerID: owner, CharacterID: 1},
		{OwnerID: owner, CharacterID: 2},
		{OwnerID: owner + 1, CharacterID: 1},
	}
	const key = "likes"
	for _, scope := range scopes {
		if err := store.UpsertFacts(ctx, scope, []memory.Fact{{Key: key, Value: "test"}}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := store.DeleteFact(ctx, scope, key); err != nil {
				t.Error(err)
			}
		})
	}
	if err := store.DeleteFact(ctx, scopes[0], key); err != nil {
		t.Fatal(err)
	}
	assertEmptyScope(t, store, scopes[0])
	for _, scope := range scopes[1:] {
		facts, err := store.ListFacts(ctx, scope, 8)
		if err != nil || len(facts) != 1 {
			t.Fatalf("deletion escaped scope: %+v err=%v", facts, err)
		}
	}
}

func assertEmptyScope(t *testing.T, store *memorymysql.Store, scope memory.Scope) {
	t.Helper()
	items, err := store.ListFacts(context.Background(), scope, 8)
	if err != nil || len(items) != 0 {
		t.Fatalf("memory leaked into scope %+v: %+v %v", scope, items, err)
	}
}
