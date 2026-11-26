package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"

	"github.com/google/uuid"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// DeviceCredential is an internal persistence record, never an API response.
type DeviceCredential struct {
	Device domain.DeviceInstance
	Digest string `json:"-"`
}

// DeviceRegistryStore persists device identity separately from admin tokens.
type DeviceRegistryStore interface {
	FindDeviceType(context.Context, int64) (domain.DeviceType, error)
	CreateDeviceInstance(context.Context, DeviceCredential) (domain.DeviceInstance, error)
	FindDeviceCredential(context.Context, string) (DeviceCredential, error)
	DisableDeviceInstance(context.Context, string) error
}

// DeviceRegistry owns registration, verification and revocation. Transport
// adapters must bind authenticated identity to a server-owned device type.
type DeviceRegistry struct{ store DeviceRegistryStore }

// NewDeviceRegistry creates the physical device registry.
func NewDeviceRegistry(store DeviceRegistryStore) *DeviceRegistry {
	return &DeviceRegistry{store: store}
}

// DeviceEnrollment returns the secret once. It must never be recorded in audit
// events, ordinary catalog responses or application logs.
type DeviceEnrollment struct {
	Device domain.DeviceInstance `json:"device"`
	Secret string                `json:"secret"`
}

// Enroll registers a new device with a cryptographically random credential.
func (r *DeviceRegistry) Enroll(ctx context.Context, deviceTypeID int64, name string) (DeviceEnrollment, error) {
	device, err := domain.NewDeviceInstance(uuid.NewString(), deviceTypeID, name)
	if err != nil {
		return DeviceEnrollment{}, apperr.WrapBadRequest(err, "invalid device registration")
	}
	if _, err := r.store.FindDeviceType(ctx, deviceTypeID); err != nil {
		return DeviceEnrollment{}, err
	}
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return DeviceEnrollment{}, err
	}
	secret := base64.RawURLEncoding.EncodeToString(key[:])
	device, err = r.store.CreateDeviceInstance(ctx, DeviceCredential{Device: device, Digest: deviceDigest(secret)})
	if err != nil {
		return DeviceEnrollment{}, err
	}
	return DeviceEnrollment{Device: device, Secret: secret}, nil
}

// Authenticate verifies one enabled device without granting administrator access.
func (r *DeviceRegistry) Authenticate(ctx context.Context, id, secret string) (domain.DeviceInstance, error) {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed == uuid.Nil || len(secret) != base64.RawURLEncoding.EncodedLen(32) {
		return domain.DeviceInstance{}, apperr.NewUnauthorized()
	}
	record, err := r.store.FindDeviceCredential(ctx, parsed.String())
	if err != nil {
		if appErr, ok := apperr.Parse(err); ok && appErr.Code() == apperr.ErrNotFound {
			return domain.DeviceInstance{}, apperr.NewUnauthorized()
		}
		return domain.DeviceInstance{}, err
	}
	if !record.Device.Enabled || subtle.ConstantTimeCompare([]byte(record.Digest), []byte(deviceDigest(secret))) != 1 {
		return domain.DeviceInstance{}, apperr.NewUnauthorized()
	}
	return record.Device, nil
}

// Disable revokes subsequent authentication of a device.
func (r *DeviceRegistry) Disable(ctx context.Context, id string) error {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed == uuid.Nil {
		return apperr.NewBadRequest("invalid device identity")
	}
	return r.store.DisableDeviceInstance(ctx, parsed.String())
}

func deviceDigest(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
