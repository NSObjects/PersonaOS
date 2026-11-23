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

// TestActiveConfigDemoMySQL verifies the only confirmed configuration changes
// on explicit Apply, and that a failed Apply never replaces a good snapshot.
//
//nolint:funlen // This is one complete real-MySQL Demo configuration lifecycle.
func TestActiveConfigDemoMySQL(t *testing.T) {
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
	active, err := catalogmysql.NewActiveConfigStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	uc := usecase.New(catalog)
	nonce := fmt.Sprintf("demo_%d", time.Now().UnixNano())
	character, err := uc.CreateCharacter(ctx, usecase.CharacterInput{
		Code: nonce, Name: "Reusable", Persona: "Original",
	})
	if err != nil {
		t.Fatal(err)
	}
	device, err := uc.CreateDeviceType(ctx, usecase.DeviceTypeInput{
		Code: nonce, Name: "Dynamic model", CharacterID: character.ID, HasSpeaker: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	voice, err := resources.Create(ctx, domain.Resource{
		Kind: domain.ResourceVoice, Code: nonce, Name: "Voice",
		Provider: "aliyun", Reference: "voice.demo", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, query := range []string{
			"DELETE FROM device_type_active_configs WHERE device_type_id = ?",
			"DELETE FROM device_type_capability_drafts WHERE device_type_id = ?",
		} {
			if err := db.Exec(query, device.ID).Error; err != nil {
				t.Errorf("cleanup config: %v", err)
			}
		}
		if err := db.Exec("DELETE FROM device_types WHERE id = ?", device.ID).Error; err != nil {
			t.Errorf("cleanup device type: %v", err)
		}
		if err := db.Exec("DELETE FROM characters WHERE id = ?", character.ID).Error; err != nil {
			t.Errorf("cleanup character: %v", err)
		}
		if err := db.Exec("DELETE FROM capability_resources WHERE id = ?", voice.ID).Error; err != nil {
			t.Errorf("cleanup voice: %v", err)
		}
	})
	_, err = active.FindActive(ctx, device.ID)
	assertAppError(t, err, apperr.ErrNotFound)
	if _, err := drafts.Save(ctx, domain.DeviceTypeCapabilities{
		DeviceTypeID: device.ID, VoiceID: voice.ID,
	}); err != nil {
		t.Fatal(err)
	}
	first, err := active.Apply(ctx, device.ID, 99)
	if err != nil || first.Version != 1 || first.Voice == nil || first.Voice.ID != voice.ID {
		t.Fatalf("first Apply() = %+v, %v", first, err)
	}
	if _, err := uc.UpdateCharacter(ctx, character.ID, usecase.CharacterInput{
		Name: "Reusable", Persona: "New persona",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := drafts.Save(ctx, domain.DeviceTypeCapabilities{DeviceTypeID: device.ID}); err != nil {
		t.Fatal(err)
	}
	unchanged, err := active.FindActive(ctx, device.ID)
	if err != nil || unchanged.Version != 1 || unchanged.Character.Persona != "Original" ||
		unchanged.Voice == nil || unchanged.Voice.ID != voice.ID {
		t.Fatalf("draft mutated active config: %+v, %v", unchanged, err)
	}
	if _, err := drafts.Save(ctx, domain.DeviceTypeCapabilities{
		DeviceTypeID: device.ID, VoiceID: voice.ID,
	}); err != nil {
		t.Fatal(err)
	}
	voice.Enabled = false
	if _, err := resources.Update(ctx, voice); err != nil {
		t.Fatal(err)
	}
	_, err = active.Apply(ctx, device.ID, 99)
	assertAppError(t, err, apperr.ErrBadRequest)
	unchanged, err = active.FindActive(ctx, device.ID)
	if err != nil || unchanged.Version != 1 || unchanged.Voice == nil {
		t.Fatalf("failed Apply() changed active snapshot: %+v, %v", unchanged, err)
	}
	if _, err := drafts.Save(ctx, domain.DeviceTypeCapabilities{DeviceTypeID: device.ID}); err != nil {
		t.Fatal(err)
	}
	second, err := active.Apply(ctx, device.ID, 99)
	if err != nil || second.Version != 2 || second.Voice != nil || second.Character.Persona != "New persona" {
		t.Fatalf("second Apply() = %+v, %v", second, err)
	}
	reopened, err := catalogmysql.NewActiveConfigStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := reopened.FindActive(ctx, device.ID)
	if err != nil || stored.Version != 2 || stored.Character.Persona != "New persona" {
		t.Fatalf("reopened FindActive() = %+v, %v", stored, err)
	}
}
