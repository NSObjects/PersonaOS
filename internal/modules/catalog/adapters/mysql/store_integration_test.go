package mysql_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	catalogmysql "github.com/NSObjects/PersonaOS/internal/modules/catalog/adapters/mysql"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

func integrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("PERSONAOS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("PERSONAOS_TEST_MYSQL_DSN must be set for MySQL integration tests")
	}
	// CI passes the explicit disposable test database in its DSN.
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil || cfg.DBName == "" {
		t.Fatalf("invalid MySQL integration DSN: %v", err)
	}
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect to MySQL integration database: %v", err)
	}
	return db
}

func assertAppError(t *testing.T, err error, code int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected application error; got nil")
	}
	appError, ok := apperr.Parse(err)
	if !ok || appError.Code() != code {
		t.Fatalf("error = %v, want application code %d", err, code)
	}
}

// This test intentionally verifies the entire transactional lifetime of shared character records.
//
//nolint:gocyclo,funlen // An integration lifecycle must assert its independent failure branches.
func TestCatalogMySQLCompleteLifecycle(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	store, err := catalogmysql.NewStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	uc := usecase.New(store)

	// Every run uses unique codes so reruns remain safe on a dedicated DB.
	nonce := fmt.Sprintf("%d", time.Now().UnixNano())
	charCode := "d1c_" + nonce
	otherCode := "d1x_" + nonce
	a3Code := "A3_" + nonce
	a4Code := "A4_" + nonce
	characterIDs := make([]int64, 0, 2)
	deviceIDs := make([]int64, 0, 2)
	t.Cleanup(func() {
		for _, id := range deviceIDs {
			if err := db.Exec("DELETE FROM device_types WHERE id = ?", id).Error; err != nil {
				t.Errorf("cleanup device type %d: %v", id, err)
			}
		}
		for _, id := range characterIDs {
			if err := db.Exec("DELETE FROM characters WHERE id = ?", id).Error; err != nil {
				t.Errorf("cleanup character %d: %v", id, err)
			}
		}
	})

	character, err := uc.CreateCharacter(ctx, usecase.CharacterInput{
		Code: charCode, Name: "小新", Persona: "顽皮", Description: "测试角色",
	})
	if err != nil {
		t.Fatal(err)
	}
	characterIDs = append(characterIDs, character.ID)
	if character.ID <= 0 || character.Code != charCode {
		t.Fatalf("created character = %+v", character)
	}
	other, err := uc.CreateCharacter(ctx, usecase.CharacterInput{
		Code: otherCode, Name: "另一个角色", Persona: "认真",
	})
	if err != nil {
		t.Fatal(err)
	}
	characterIDs = append(characterIDs, other.ID)

	_, err = uc.CreateCharacter(ctx, usecase.CharacterInput{Code: charCode, Name: "重复", Persona: "重复"})
	assertAppError(t, err, apperr.ErrConflict)

	a3, err := uc.CreateDeviceType(ctx, usecase.DeviceTypeInput{
		Code: a3Code, Name: "A3 基础版", CharacterID: character.ID,
		HasMicrophone: true, HasSpeaker: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	deviceIDs = append(deviceIDs, a3.ID)
	a4, err := uc.CreateDeviceType(ctx, usecase.DeviceTypeInput{
		Code: a4Code, Name: "A4 视觉版", CharacterID: character.ID,
		HasCamera: true, HasMicrophone: true, HasSpeaker: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	deviceIDs = append(deviceIDs, a4.ID)
	if a3.CharacterID != a4.CharacterID || a3.HasCamera || !a4.HasCamera {
		t.Fatalf("A3/A4 character reuse and camera features = %+v %+v", a3, a4)
	}
	_, err = uc.CreateDeviceType(ctx, usecase.DeviceTypeInput{
		Code: a3Code, Name: "重复型号", CharacterID: character.ID,
	})
	assertAppError(t, err, apperr.ErrConflict)

	// Reopening the store verifies real persistence, not a fake or GORM cache.
	reopened, err := catalogmysql.NewStore(ctx, db.Session(&gorm.Session{NewDB: true}))
	if err != nil {
		t.Fatal(err)
	}
	reader := usecase.New(reopened)
	stored, err := reader.FindDeviceType(ctx, a4.ID)
	if err != nil || stored.CharacterID != character.ID || !stored.HasCamera {
		t.Fatalf("reloaded A4 = %+v, err = %v", stored, err)
	}
	types, err := reader.ListDeviceTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, d := range types {
		if d.ID == a3.ID || d.ID == a4.ID {
			matches++
			if d.CharacterID != character.ID {
				t.Fatalf("device %d references character %d, want %d", d.ID, d.CharacterID, character.ID)
			}
		}
	}
	if matches != 2 {
		t.Fatalf("found %d test device types, want 2", matches)
	}

	updated, err := uc.UpdateCharacter(ctx, character.ID, usecase.CharacterInput{
		Name: "小新（更新）", Persona: "新的角色人格",
	})
	if err != nil || updated.Name != "小新（更新）" {
		t.Fatalf("UpdateCharacter = %+v, %v", updated, err)
	}
	updatedDevice, err := uc.UpdateDeviceType(ctx, a4.ID, usecase.DeviceTypeInput{
		Name: "A4 关闭摄像头", CharacterID: other.ID,
		HasCamera: false, HasMicrophone: true, HasSpeaker: true,
	})
	if err != nil || updatedDevice.Code != a4Code || updatedDevice.HasCamera || updatedDevice.CharacterID != other.ID {
		t.Fatalf("UpdateDeviceType = %+v, %v", updatedDevice, err)
	}

	// Reject deletion via both the usecase and the actual database FK.
	assertAppError(t, uc.DeleteCharacter(ctx, character.ID), apperr.ErrConflict)
	if err := db.Exec("DELETE FROM characters WHERE id = ?", character.ID).Error; err == nil {
		t.Fatal("MySQL FK allowed deleting a referenced character")
	}
	_, err = uc.CreateDeviceType(ctx, usecase.DeviceTypeInput{
		Code: "Z_" + nonce, Name: "无效外键", CharacterID: 999999999,
	})
	assertAppError(t, err, apperr.ErrNotFound)

	if err := uc.DeleteDeviceType(ctx, a3.ID); err != nil {
		t.Fatal(err)
	}
	if err := uc.DeleteCharacter(ctx, character.ID); err != nil {
		t.Fatal(err)
	}
	if err := uc.DeleteDeviceType(ctx, a4.ID); err != nil {
		t.Fatal(err)
	}
	if err := uc.DeleteCharacter(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	_, err = reader.FindCharacter(ctx, character.ID)
	assertAppError(t, err, apperr.ErrNotFound)
	_, err = reader.FindDeviceType(ctx, a3.ID)
	assertAppError(t, err, apperr.ErrNotFound)

	// A canceled context must never perform a write.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = uc.CreateCharacter(canceled, usecase.CharacterInput{
		Code: "cancel_" + nonce, Name: "取消", Persona: "取消",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled CreateCharacter error = %v", err)
	}
}
