package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidDeviceInstance indicates invalid physical device metadata.
var ErrInvalidDeviceInstance = errors.New("invalid device instance")

// DeviceInstance identifies one physical device. Its capabilities and persona
// are inherited from the published configuration of its DeviceTypeID.
type DeviceInstance struct {
	ID           string    `json:"id"`
	DeviceTypeID int64     `json:"device_type_id"`
	Name         string    `json:"name"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// NewDeviceInstance validates physical identity without embedding role or
// capability configuration into the instance.
func NewDeviceInstance(id string, deviceTypeID int64, name string) (DeviceInstance, error) {
	parsed, err := uuid.Parse(id)
	name = strings.TrimSpace(name)
	if err != nil || parsed == uuid.Nil || deviceTypeID <= 0 || name == "" || len(name) > 160 {
		return DeviceInstance{}, ErrInvalidDeviceInstance
	}
	return DeviceInstance{ID: parsed.String(), DeviceTypeID: deviceTypeID, Name: name, Enabled: true}, nil
}
