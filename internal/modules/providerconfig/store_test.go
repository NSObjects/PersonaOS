package providerconfig

import (
	"bytes"
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
	c := testCipher(t, 1)
	store, err := NewStore(ctx, db, c)
	if err != nil {
		t.Fatal(err)
	}

	// Roll back changes even if an assertion fails. The test never replaces
	// real credentials in a preexisting database.
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
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
	var row storedService
	if err := tx.Take(&row, "service = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if len(row.Ciphertext) == 0 || bytes.Contains(row.Ciphertext, []byte(secret)) {
		t.Fatal("plaintext credential persisted")
	}
	reloaded := &Store{db: tx, cipher: c, getenv: os.Getenv}
	values, source, err = reloaded.Load(ctx, id)
	if err != nil || source != "database" || values[FieldAPIKey] != secret {
		t.Fatal("stored configuration did not override environment")
	}
	wrongKey := &Store{db: tx, cipher: testCipher(t, 2), getenv: os.Getenv}
	if _, _, err := wrongKey.Load(ctx, id); err == nil {
		t.Fatal("wrong deployment key silently fell back to environment")
	}
	if err := store.Replace(ctx, id, Values{FieldAPIKey: ""}); err != nil {
		t.Fatal(err)
	}
	values, source, err = reloaded.Load(ctx, id)
	if err != nil || source != "database" || values[FieldAPIKey] != "" {
		t.Fatal("explicit clear resurrected environment credential")
	}
	withoutKey := &Store{db: tx, getenv: os.Getenv}
	if err := withoutKey.Replace(ctx, id, Values{FieldAPIKey: secret}); err == nil {
		t.Fatal("write accepted without encryption key")
	}
}
