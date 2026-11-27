package mysql_test

import (
	"context"
	"testing"

	"gorm.io/gorm"

	catalogmysql "github.com/NSObjects/PersonaOS/internal/modules/catalog/adapters/mysql"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// Called from the existing real-MySQL catalog lifecycle in CI.
func verifyDeviceRegistryMySQL(t *testing.T, db *gorm.DB, store *catalogmysql.Store, deviceTypeID int64) {
	t.Helper()
	ctx := context.Background()
	registry := usecase.NewDeviceRegistry(store)
	enrollment, err := registry.Enroll(ctx, deviceTypeID, "MySQL device fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Exec("DELETE FROM device_instances WHERE id = ?", enrollment.Device.ID).Error; err != nil {
			t.Errorf("cleanup device instance: %v", err)
		}
	}()
	assertAppError(t, store.DeleteDeviceType(ctx, deviceTypeID), apperr.ErrConflict)
	reopened, err := catalogmysql.NewStore(ctx, db.Session(&gorm.Session{NewDB: true}))
	if err != nil {
		t.Fatal(err)
	}
	reader := usecase.NewDeviceRegistry(reopened)
	device, err := reader.Authenticate(ctx, enrollment.Device.ID, enrollment.Secret)
	if err != nil || device.ID != enrollment.Device.ID || device.DeviceTypeID != deviceTypeID || !device.Enabled {
		t.Fatalf("device reload = %+v, err=%v", device, err)
	}
	record, err := reopened.FindDeviceCredential(ctx, device.ID)
	if err != nil || len(record.Digest) != 64 || record.Digest == enrollment.Secret {
		t.Fatal("device credential was not stored as a digest")
	}
	if err := reader.Disable(ctx, device.ID); err != nil {
		t.Fatal(err)
	}
	_, err = registry.Authenticate(ctx, device.ID, enrollment.Secret)
	assertAppError(t, err, apperr.ErrUnauthorized)
}
