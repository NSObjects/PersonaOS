package mysql_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	catalogmysql "github.com/NSObjects/PersonaOS/internal/modules/catalog/adapters/mysql"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// TestCapabilityDraftMySQLLifecycle validates real foreign keys and rollback.
//
//nolint:funlen // One integration scenario covers both device types and atomic replacement.
func TestCapabilityDraftMySQLLifecycle(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	catalog, err := catalogmysql.NewStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := catalogmysql.NewResourceStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := catalogmysql.NewCapabilityDraftStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	uc := usecase.New(catalog)
	nonce := fmt.Sprintf("d2_%d", time.Now().UnixNano())
	character, err := uc.CreateCharacter(ctx, usecase.CharacterInput{
		Code: nonce, Name: "Shared", Persona: "Friendly",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DELETE FROM device_type_capability_drafts WHERE device_type_id IN (SELECT id FROM device_types WHERE character_id = ?)", character.ID).Error; err != nil {
			t.Errorf("cleanup draft links: %v", err)
		}
		if err := db.Exec("DELETE FROM device_types WHERE character_id = ?", character.ID).Error; err != nil {
			t.Errorf("cleanup device types: %v", err)
		}
		if err := db.Exec("DELETE FROM characters WHERE id = ?", character.ID).Error; err != nil {
			t.Errorf("cleanup character: %v", err)
		}
	})
	a3, err := uc.CreateDeviceType(ctx, usecase.DeviceTypeInput{Code: "A3_" + nonce, Name: "A3", CharacterID: character.ID, HasSpeaker: true})
	if err != nil {
		t.Fatal(err)
	}
	a4, err := uc.CreateDeviceType(ctx, usecase.DeviceTypeInput{Code: "A4_" + nonce, Name: "A4", CharacterID: character.ID, HasSpeaker: true, HasCamera: true})
	if err != nil {
		t.Fatal(err)
	}
	voice, err := resources.Create(ctx, domain.Resource{Kind: domain.ResourceVoice, Code: nonce, Name: "Voice", Provider: "aliyun", Reference: "voice-a", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	tool, err := resources.Create(ctx, domain.Resource{Kind: domain.ResourceTool, Code: nonce, Name: "Vision", Reference: "tools.observe", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DELETE FROM capability_resources WHERE id IN (?, ?)", voice.ID, tool.ID).Error; err != nil {
			t.Errorf("cleanup capability resources: %v", err)
		}
	})
	a3Draft, err := drafts.Save(ctx, domain.DeviceTypeCapabilities{DeviceTypeID: a3.ID, VoiceID: voice.ID})
	if err != nil || a3Draft.VoiceID != voice.ID {
		t.Fatalf("Save A3 = %+v, %v", a3Draft, err)
	}
	a4Draft, err := drafts.Save(ctx, domain.DeviceTypeCapabilities{DeviceTypeID: a4.ID, VoiceID: voice.ID, ToolIDs: []int64{tool.ID}})
	if err != nil || len(a4Draft.ToolIDs) != 1 {
		t.Fatalf("Save A4 = %+v, %v", a4Draft, err)
	}
	stored, err := drafts.Find(ctx, a3.ID)
	if err != nil || len(stored.ToolIDs) != 0 || stored.VoiceID != voice.ID {
		t.Fatalf("A3 changed by A4 configuration: %+v, %v", stored, err)
	}
	_, err = drafts.Save(ctx, domain.DeviceTypeCapabilities{DeviceTypeID: a4.ID, ToolIDs: []int64{voice.ID}})
	assertAppError(t, err, apperr.ErrBadRequest)
	again, err := drafts.Find(ctx, a4.ID)
	if err != nil || again.VoiceID != voice.ID || len(again.ToolIDs) != 1 {
		t.Fatalf("failed replacement damaged old draft: %+v, %v", again, err)
	}
	assertAppError(t, resources.Delete(ctx, voice.ID), apperr.ErrConflict)
	_, err = drafts.Save(ctx, domain.DeviceTypeCapabilities{DeviceTypeID: a4.ID, ToolIDs: []int64{999999999}})
	assertAppError(t, err, apperr.ErrBadRequest)
	_, err = drafts.Save(ctx, domain.DeviceTypeCapabilities{DeviceTypeID: a4.ID})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := drafts.Find(ctx, a4.ID)
	if err != nil || empty.VoiceID != 0 || len(empty.ToolIDs) != 0 {
		t.Fatalf("clearing draft failed: %+v, %v", empty, err)
	}
	if err := db.Exec("DELETE FROM device_type_capability_drafts WHERE device_type_id = ?", a3.ID).Error; err != nil {
		t.Fatal(err)
	}
}
