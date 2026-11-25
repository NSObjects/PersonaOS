package providerconfig

import (
	"context"
	"os"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

//nolint:gocyclo // Integration test covers the entire persistence and override lifecycle.
func TestProviderStoreMySQL(t *testing.T) {
	dsn := os.Getenv("PERSONAOS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("requires disposable MySQL integration database")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store, err := NewStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	// Roll back changes even if an assertion fails. The test never replaces
	// real credentials in a preexisting database.
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error(err)
		}
	})
	store.db = tx
	const id = "deepseek"
	const secret = "test-only-web-search-secret"
	if err := tx.Delete(&storedService{}, "service = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEEPSEEK_API_KEY", "legacy-env-key")
	values, source, err := store.Load(ctx, id)
	if err != nil || source != "environment" || values[FieldAPIKey] != "legacy-env-key" {
		t.Fatal("environment fallback failed")
	}
	if err := store.Replace(ctx, id, Values{FieldAPIKey: secret}); err != nil {
		t.Fatal(err)
	}
	reloaded := &Store{db: tx, getenv: os.Getenv}
	values, source, err = reloaded.Load(ctx, id)
	if err != nil || source != "database" || values[FieldAPIKey] != secret {
		t.Fatal("stored configuration did not override environment")
	}
	before, err := reloaded.Snapshot(ctx)
	if err != nil || before.Get("DEEPSEEK_API_KEY") != secret {
		t.Fatal("runtime did not read saved key")
	}
	if err := store.Replace(ctx, id, Values{FieldAPIKey: ""}); err != nil {
		t.Fatal(err)
	}
	after, err := reloaded.Snapshot(ctx)
	if err != nil || after.Get("DEEPSEEK_API_KEY") != "" || before.Get("DEEPSEEK_API_KEY") != secret {
		t.Fatal("runtime snapshots did not preserve old and new configuration")
	}
	values, source, err = reloaded.Load(ctx, id)
	if err != nil || source != "database" || values[FieldAPIKey] != "" {
		t.Fatal("explicit clear resurrected environment credential")
	}
}
