package mysql

import (
	"context"
	"errors"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	inframysql "github.com/NSObjects/PersonaOS/internal/platform/infrastructure/mysql"
)

type deviceInstanceModel struct {
	ID           string          `gorm:"type:char(36);primaryKey"`
	DeviceTypeID int64           `gorm:"not null;index"`
	DeviceType   deviceTypeModel `gorm:"foreignKey:DeviceTypeID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	Name         string          `gorm:"type:varchar(160);not null"`
	Digest       string          `gorm:"type:char(64);not null"`
	Enabled      bool            `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (deviceInstanceModel) TableName() string { return "device_instances" }

func (m deviceInstanceModel) domain() domain.DeviceInstance {
	return domain.DeviceInstance{
		ID: m.ID, DeviceTypeID: m.DeviceTypeID, Name: m.Name,
		Enabled: m.Enabled, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// CreateDeviceInstance persists the identity and only the credential digest.
func (s *Store) CreateDeviceInstance(ctx context.Context, credential usecase.DeviceCredential) (domain.DeviceInstance, error) {
	d := credential.Device
	row := deviceInstanceModel{
		ID: d.ID, DeviceTypeID: d.DeviceTypeID, Name: d.Name,
		Digest: credential.Digest, Enabled: d.Enabled,
	}
	if err := s.db.WithContext(ctx).Omit("DeviceType").Create(&row).Error; err != nil {
		var mysqlErr *driver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1452 {
			return domain.DeviceInstance{}, apperr.NewNotFound("device type")
		}
		return domain.DeviceInstance{}, inframysql.MapWriteError(err, "device identity already exists", "create device instance")
	}
	return row.domain(), nil
}

// FindDeviceCredential is an internal lookup, not a catalog response.
func (s *Store) FindDeviceCredential(ctx context.Context, id string) (usecase.DeviceCredential, error) {
	var row deviceInstanceModel
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return usecase.DeviceCredential{}, inframysql.MapReadError(err, "device instance", "find device credential")
	}
	return usecase.DeviceCredential{Device: row.domain(), Digest: row.Digest}, nil
}

// DisableDeviceInstance revokes new connections without changing the binding.
func (s *Store) DisableDeviceInstance(ctx context.Context, id string) error {
	result := s.db.WithContext(ctx).Model(&deviceInstanceModel{}).Where("id = ?", id).
		Updates(map[string]any{"enabled": false, updatedAtColumn: time.Now().UTC()})
	if result.Error != nil {
		return apperr.WrapDatabase(result.Error, "disable device instance")
	}
	if result.RowsAffected == 0 {
		_, err := s.FindDeviceCredential(ctx, id)
		return err
	}
	return nil
}

var _ usecase.DeviceRegistryStore = (*Store)(nil)
