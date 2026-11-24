// Package mysql persists characters and device types in the configured MySQL database.
package mysql

import (
	"context"
	"errors"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	inframysql "github.com/NSObjects/PersonaOS/internal/platform/infrastructure/mysql"
)

// Store implements the catalog usecase persistence contract.
type Store struct{ db *gorm.DB }

// NewStore migrates catalog tables using the application's existing GORM connection.
func NewStore(ctx context.Context, db *gorm.DB) (*Store, error) {
	if ctx == nil {
		return nil, errors.New("create catalog store: nil context")
	}
	if db == nil {
		return nil, errors.New("create catalog store: nil db")
	}
	if err := db.WithContext(ctx).AutoMigrate(&characterModel{}, &deviceTypeModel{}); err != nil {
		return nil, apperr.WrapDatabase(err, "migrate catalog tables")
	}
	return &Store{db: db}, nil
}

type characterModel struct {
	ID          int64  `gorm:"primaryKey"`
	Code        string `gorm:"type:varchar(64);uniqueIndex;not null"`
	Name        string `gorm:"type:varchar(160);not null"`
	Persona     string `gorm:"type:text;not null"`
	Description string `gorm:"type:text"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (characterModel) TableName() string { return "characters" }
func (m characterModel) domain() domain.Character {
	return domain.Character{ID: m.ID, Code: m.Code, Name: m.Name, Persona: m.Persona, Description: m.Description, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

func modelCharacter(c domain.Character) characterModel {
	return characterModel{ID: c.ID, Code: c.Code, Name: c.Name, Persona: c.Persona, Description: c.Description, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}

type deviceTypeModel struct {
	ID            int64          `gorm:"primaryKey"`
	Code          string         `gorm:"type:varchar(64);uniqueIndex;not null"`
	Name          string         `gorm:"type:varchar(160);not null"`
	Description   string         `gorm:"type:text"`
	CharacterID   int64          `gorm:"not null;index"`
	Character     characterModel `gorm:"foreignKey:CharacterID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	HasCamera     bool           `gorm:"not null"`
	HasMicrophone bool           `gorm:"not null"`
	HasSpeaker    bool           `gorm:"not null"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (deviceTypeModel) TableName() string { return "device_types" }
func (m deviceTypeModel) domain() domain.DeviceType {
	return domain.DeviceType{
		ID: m.ID, Code: m.Code, Name: m.Name, Description: m.Description, CharacterID: m.CharacterID,
		HasCamera: m.HasCamera, HasMicrophone: m.HasMicrophone, HasSpeaker: m.HasSpeaker, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func modelDeviceType(d domain.DeviceType) deviceTypeModel {
	return deviceTypeModel{
		ID: d.ID, Code: d.Code, Name: d.Name, Description: d.Description, CharacterID: d.CharacterID,
		HasCamera: d.HasCamera, HasMicrophone: d.HasMicrophone, HasSpeaker: d.HasSpeaker, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func mapFK(err error, fallback string) error {
	var e *driver.MySQLError
	if errors.As(err, &e) {
		if e.Number == 1451 {
			return apperr.NewConflict("character is in use by device types")
		}
		if e.Number == 1452 {
			return apperr.NewBadRequest("character does not exist")
		}
	}
	return inframysql.MapWriteError(err, fallback, "write catalog")
}

// ListCharacters returns characters ordered by ID.
func (s *Store) ListCharacters(ctx context.Context) ([]domain.Character, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var rows []characterModel
	if err := s.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, apperr.WrapDatabase(err, "list characters")
	}
	out := make([]domain.Character, 0, len(rows))
	for _, v := range rows {
		out = append(out, v.domain())
	}
	return out, nil
}

// FindCharacter returns one character.
func (s *Store) FindCharacter(ctx context.Context, id int64) (domain.Character, error) {
	if err := ctx.Err(); err != nil {
		return domain.Character{}, err
	}
	var row characterModel
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return domain.Character{}, inframysql.MapReadError(err, "character", "find character")
	}
	return row.domain(), nil
}

// CreateCharacter saves a unique character.
func (s *Store) CreateCharacter(ctx context.Context, c domain.Character) (domain.Character, error) {
	if err := ctx.Err(); err != nil {
		return domain.Character{}, err
	}
	row := modelCharacter(c)
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return domain.Character{}, mapFK(err, "character code already exists")
	}
	return row.domain(), nil
}

// UpdateCharacter updates mutable character fields without changing the code.
func (s *Store) UpdateCharacter(ctx context.Context, c domain.Character) (domain.Character, error) {
	if err := ctx.Err(); err != nil {
		return domain.Character{}, err
	}
	result := s.db.WithContext(ctx).Model(&characterModel{}).Where("id = ?", c.ID).
		Updates(map[string]any{nameColumn: c.Name, "persona": c.Persona, descriptionColumn: c.Description, updatedAtColumn: time.Now().UTC()})
	if result.Error != nil {
		return domain.Character{}, mapFK(result.Error, "character code already exists")
	}
	if result.RowsAffected == 0 {
		return domain.Character{}, apperr.NewNotFound("character")
	}
	return s.FindCharacter(ctx, c.ID)
}

// DeleteCharacter protects characters in use by device types.
func (s *Store) DeleteCharacter(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&deviceTypeModel{}).Where("character_id = ?", id).Count(&count).Error; err != nil {
		return apperr.WrapDatabase(err, "count character references")
	}
	if count > 0 {
		return apperr.NewConflict("character is in use by device types")
	}
	result := s.db.WithContext(ctx).Delete(&characterModel{}, "id = ?", id)
	if result.Error != nil {
		return mapFK(result.Error, "cannot delete character")
	}
	if result.RowsAffected == 0 {
		return apperr.NewNotFound("character")
	}
	return nil
}

// ListDeviceTypes returns product models.
func (s *Store) ListDeviceTypes(ctx context.Context) ([]domain.DeviceType, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var rows []deviceTypeModel
	if err := s.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, apperr.WrapDatabase(err, "list device types")
	}
	out := make([]domain.DeviceType, 0, len(rows))
	for _, v := range rows {
		out = append(out, v.domain())
	}
	return out, nil
}

// FindDeviceType returns a product model.
func (s *Store) FindDeviceType(ctx context.Context, id int64) (domain.DeviceType, error) {
	if err := ctx.Err(); err != nil {
		return domain.DeviceType{}, err
	}
	var row deviceTypeModel
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return domain.DeviceType{}, inframysql.MapReadError(err, "device type", "find device type")
	}
	return row.domain(), nil
}

// CreateDeviceType saves one product model.
func (s *Store) CreateDeviceType(ctx context.Context, d domain.DeviceType) (domain.DeviceType, error) {
	if err := ctx.Err(); err != nil {
		return domain.DeviceType{}, err
	}
	row := modelDeviceType(d)
	if err := s.db.WithContext(ctx).Omit("Character").Create(&row).Error; err != nil {
		return domain.DeviceType{}, mapFK(err, "device type code already exists")
	}
	return row.domain(), nil
}

// UpdateDeviceType changes model configuration, preserving its code.
func (s *Store) UpdateDeviceType(ctx context.Context, d domain.DeviceType) (domain.DeviceType, error) {
	if err := ctx.Err(); err != nil {
		return domain.DeviceType{}, err
	}
	result := s.db.WithContext(ctx).Model(&deviceTypeModel{}).Where("id = ?", d.ID).
		Updates(map[string]any{
			nameColumn: d.Name, descriptionColumn: d.Description, "character_id": d.CharacterID,
			"has_camera": d.HasCamera, "has_microphone": d.HasMicrophone, "has_speaker": d.HasSpeaker, updatedAtColumn: time.Now().UTC(),
		})
	if result.Error != nil {
		return domain.DeviceType{}, mapFK(result.Error, "device type code already exists")
	}
	if result.RowsAffected == 0 {
		return domain.DeviceType{}, apperr.NewNotFound("device type")
	}
	return s.FindDeviceType(ctx, d.ID)
}

// DeleteDeviceType removes a model without deleting its character.
func (s *Store) DeleteDeviceType(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result := s.db.WithContext(ctx).Delete(&deviceTypeModel{}, "id = ?", id)
	if result.Error != nil {
		return apperr.WrapDatabase(result.Error, "delete device type")
	}
	if result.RowsAffected == 0 {
		return apperr.NewNotFound("device type")
	}
	return nil
}
