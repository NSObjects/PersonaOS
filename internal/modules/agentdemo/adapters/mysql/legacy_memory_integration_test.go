package mysql_test

import (
	"context"
	"strings"
	"testing"

	memorymysql "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/adapters/mysql"
	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
)

func testLegacyMemoryMigrationMySQL(t *testing.T, store *memorymysql.Store, scope memory.Scope) {
	t.Helper()
	ctx := context.Background()
	facts := []memory.Fact{
		{Key: "favorite_animal", Value: "企鹅"},
		{Key: "favorite_story", Value: "童话"},
	}
	if err := store.UpsertFacts(ctx, scope, facts); err != nil {
		t.Fatal(err)
	}
	if err := store.MigrateLegacyMemory(ctx); err != nil {
		t.Fatal(err)
	}
	batch := readPendingBatch(t, store, scope, 0)
	for _, fact := range facts {
		if !strings.Contains(batch.Core.Stable, fact.Value) {
			t.Fatalf("missing migrated preference: %+v", fact)
		}
	}
	assertEmptyScope(t, store, scope)
	if err := store.MigrateLegacyMemory(ctx); err != nil {
		t.Fatal(err)
	}
	again := readPendingBatch(t, store, scope, 0)
	if again.Core != batch.Core {
		t.Fatalf("repeat migration modified memory: %+v", again)
	}
}
