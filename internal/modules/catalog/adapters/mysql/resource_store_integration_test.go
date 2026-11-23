package mysql_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	catalogmysql "github.com/NSObjects/PersonaOS/internal/modules/catalog/adapters/mysql"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// TestResourceStoreMySQLLifecycle exercises real resource uniqueness, persistence and updates.
func TestResourceStoreMySQLLifecycle(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	store, err := catalogmysql.NewResourceStore(ctx, db)
	if err != nil {
		t.Fatalf("NewResourceStore() error = %v", err)
	}
	code := fmt.Sprintf("d2_%d", time.Now().UnixNano())
	voice, err := store.Create(ctx, domain.Resource{
		Kind: domain.ResourceVoice, Code: code, Name: "Original voice",
		Provider: "aliyun", Reference: "voice-1", Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create(voice) error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DELETE FROM capability_resources WHERE id = ?", voice.ID).Error; err != nil {
			t.Errorf("cleanup voice: %v", err)
		}
	})
	if voice.ID <= 0 {
		t.Fatalf("voice.ID = %d, want positive", voice.ID)
	}
	_, err = store.Create(ctx, domain.Resource{
		Kind: domain.ResourceVoice, Code: code, Name: "Duplicate",
		Provider: "aliyun", Reference: "voice-2",
	})
	assertAppError(t, err, apperr.ErrConflict)
	tool, err := store.Create(ctx, domain.Resource{
		Kind: domain.ResourceTool, Code: code, Name: "Same code in other kind",
		Reference: "tools.demo",
	})
	if err != nil {
		t.Fatalf("Create(tool) with reused code error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DELETE FROM capability_resources WHERE id = ?", tool.ID).Error; err != nil {
			t.Errorf("cleanup tool: %v", err)
		}
	})
	reopened, err := catalogmysql.NewResourceStore(ctx, db)
	if err != nil {
		t.Fatalf("reopen store error = %v", err)
	}
	stored, err := reopened.Find(ctx, voice.ID)
	if err != nil || stored.Reference != "voice-1" {
		t.Fatalf("Find() = %+v, %v, want voice-1", stored, err)
	}
	stored.Name = "Updated voice"
	stored.Enabled = false
	updated, err := reopened.Update(ctx, stored)
	if err != nil || updated.Name != "Updated voice" || updated.Enabled {
		t.Fatalf("Update() = %+v, %v, want disabled updated voice", updated, err)
	}
	voices, err := reopened.List(ctx, domain.ResourceVoice)
	if err != nil {
		t.Fatalf("List(voice) error = %v", err)
	}
	for _, row := range voices {
		if row.ID == tool.ID {
			t.Fatalf("List(voice) includes tool %d", tool.ID)
		}
	}
	if err := reopened.Delete(ctx, voice.ID); err != nil {
		t.Fatalf("Delete(voice) error = %v", err)
	}
	_, err = reopened.Find(ctx, voice.ID)
	assertAppError(t, err, apperr.ErrNotFound)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.Create(cancelled, domain.Resource{
		Kind: domain.ResourceTool, Code: "cancelled", Name: "Cancelled", Reference: "tools.cancelled",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Create() error = %v, want context.Canceled", err)
	}
}
