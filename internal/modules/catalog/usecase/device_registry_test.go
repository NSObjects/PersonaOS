package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

const registryFixtureName = "registry device"

type registryFixture struct {
	rows map[string]usecase.DeviceCredential
}

func (f *registryFixture) FindDeviceType(_ context.Context, id int64) (domain.DeviceType, error) {
	if id != 7 {
		return domain.DeviceType{}, apperr.NewNotFound("device type")
	}
	return domain.DeviceType{ID: id}, nil
}

func (f *registryFixture) CreateDeviceInstance(_ context.Context, value usecase.DeviceCredential) (domain.DeviceInstance, error) {
	f.rows[value.Device.ID] = value
	return value.Device, nil
}

func (f *registryFixture) FindDeviceCredential(_ context.Context, id string) (usecase.DeviceCredential, error) {
	value, ok := f.rows[id]
	if !ok {
		return usecase.DeviceCredential{}, apperr.NewNotFound("device instance")
	}
	return value, nil
}

func (f *registryFixture) DisableDeviceInstance(_ context.Context, id string) error {
	value, err := f.FindDeviceCredential(context.Background(), id)
	if err != nil {
		return err
	}
	value.Device.Enabled = false
	f.rows[id] = value
	return nil
}

func TestDeviceRegistryIndependentIdentities(t *testing.T) {
	ctx := context.Background()
	store := &registryFixture{rows: make(map[string]usecase.DeviceCredential)}
	registry := usecase.NewDeviceRegistry(store)
	first, err := registry.Enroll(ctx, 7, registryFixtureName)
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.Enroll(ctx, 7, registryFixtureName)
	if err != nil {
		t.Fatal(err)
	}
	if first.Device.ID == second.Device.ID || first.Secret == second.Secret {
		t.Fatal("device identities or secrets reused")
	}
	if digest := store.rows[first.Device.ID].Digest; len(digest) != 64 || digest == first.Secret {
		t.Fatal("expected only the credential digest in storage")
	}
	device, err := registry.Authenticate(ctx, first.Device.ID, first.Secret)
	if err != nil || device != first.Device || device.DeviceTypeID != 7 {
		t.Fatalf("device authentication = %+v, err=%v", device, err)
	}
	for _, id := range []string{first.Device.ID, uuid.NewString(), "invalid"} {
		_, err = registry.Authenticate(ctx, id, second.Secret)
		if appErr, ok := apperr.Parse(err); err == nil || !ok || appErr.Code() != apperr.ErrUnauthorized {
			t.Fatalf("expected unauthorized, got %v", err)
		}
	}
	if err := registry.Disable(ctx, first.Device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Authenticate(ctx, first.Device.ID, first.Secret); err == nil {
		t.Fatal("disabled device authenticated")
	}
	if _, err := registry.Authenticate(ctx, second.Device.ID, second.Secret); err != nil {
		t.Fatal("disabling one device affected another")
	}
}

func TestDeviceRegistryRejectsInvalidEnrollment(t *testing.T) {
	store := &registryFixture{rows: make(map[string]usecase.DeviceCredential)}
	registry := usecase.NewDeviceRegistry(store)
	for _, input := range []struct {
		id   int64
		name string
	}{
		{0, registryFixtureName},
		{7, " "},
		{8, registryFixtureName},
	} {
		if _, err := registry.Enroll(context.Background(), input.id, input.name); err == nil {
			t.Fatal("invalid enrollment accepted")
		}
	}
	if len(store.rows) != 0 {
		t.Fatal("invalid enrollment persisted")
	}
}
