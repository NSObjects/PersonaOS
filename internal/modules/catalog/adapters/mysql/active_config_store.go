package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	inframysql "github.com/NSObjects/PersonaOS/internal/platform/infrastructure/mysql"
)

// ActiveConfigStore keeps only the current confirmed Demo configuration per
// device type. No historical version management or rollback is provided.
type ActiveConfigStore struct{ db *gorm.DB }

// NewActiveConfigStore migrates the single active configuration table.
func NewActiveConfigStore(ctx context.Context, db *gorm.DB) (*ActiveConfigStore, error) {
	if ctx == nil || db == nil {
		return nil, errors.New("create active config store: context and database required")
	}
	if err := db.WithContext(ctx).AutoMigrate(&activeConfigModel{}); err != nil {
		return nil, apperr.WrapDatabase(err, "migrate active device configuration")
	}
	return &ActiveConfigStore{db: db}, nil
}

type activeConfigModel struct {
	DeviceTypeID int64           `gorm:"primaryKey"`
	DeviceType   deviceTypeModel `gorm:"foreignKey:DeviceTypeID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:CASCADE"`
	Revision     int64           `gorm:"not null"`
	Snapshot     []byte          `gorm:"type:longblob;not null"`
	AppliedAt    time.Time       `gorm:"not null"`
}

func (activeConfigModel) TableName() string { return "device_type_active_configs" }

// Apply atomically freezes the character and capability draft selected by
// this device type. Concurrent draft edits and applications serialize on its
// device_type row. Applying one model never affects another model.
func (s *ActiveConfigStore) Apply(ctx context.Context, deviceTypeID, actorID int64) (domain.PublishedDeviceConfig, error) {
	if ctx == nil || deviceTypeID <= 0 || actorID <= 0 {
		return domain.PublishedDeviceConfig{}, apperr.NewBadRequest("valid context, device type and actor are required")
	}
	if err := ctx.Err(); err != nil {
		return domain.PublishedDeviceConfig{}, err
	}
	var applied domain.PublishedDeviceConfig
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		candidate, err := loadActiveCandidate(tx, deviceTypeID)
		if err != nil {
			return err
		}
		var current activeConfigModel
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&current, "device_type_id = ?", deviceTypeID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.WrapDatabase(err, "read current device configuration")
		}
		candidate.Version = current.Revision + 1
		candidate.PublishedBy = actorID
		candidate.PublishedAt = time.Now().UTC()
		applied, err = domain.NewPublishedDeviceConfig(candidate)
		if err != nil {
			return apperr.WrapBadRequest(err, "invalid device configuration")
		}
		payload, err := json.Marshal(applied)
		if err != nil {
			return fmt.Errorf("encode device configuration: %w", err)
		}
		return persistActiveConfig(tx, current.Revision, activeConfigModel{
			DeviceTypeID: deviceTypeID, Revision: applied.Version,
			Snapshot: payload, AppliedAt: applied.PublishedAt,
		})
	})
	if err != nil {
		return domain.PublishedDeviceConfig{}, err
	}
	return applied.Clone(), nil
}

// FindActive is the only configuration read allowed for the later Agent Demo.
// Unconfirmed drafts never appear in this response.
func (s *ActiveConfigStore) FindActive(ctx context.Context, deviceTypeID int64) (domain.PublishedDeviceConfig, error) {
	if ctx == nil || deviceTypeID <= 0 {
		return domain.PublishedDeviceConfig{}, apperr.NewBadRequest("valid context and device type are required")
	}
	if err := ctx.Err(); err != nil {
		return domain.PublishedDeviceConfig{}, err
	}
	var row activeConfigModel
	if err := s.db.WithContext(ctx).First(&row, "device_type_id = ?", deviceTypeID).Error; err != nil {
		return domain.PublishedDeviceConfig{}, inframysql.MapReadError(err, "active device configuration", "read active configuration")
	}
	var snapshot domain.PublishedDeviceConfig
	if err := json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		return domain.PublishedDeviceConfig{}, apperr.WrapDatabase(err, "decode active configuration")
	}
	return snapshot.Clone(), nil
}

func loadActiveCandidate(tx *gorm.DB, deviceTypeID int64) (domain.PublishSnapshotInput, error) {
	var device deviceTypeModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&device, "id = ?", deviceTypeID).Error; err != nil {
		return domain.PublishSnapshotInput{}, inframysql.MapReadError(err, "device type", "lock device type")
	}
	var character characterModel
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
		First(&character, "id = ?", device.CharacterID).Error; err != nil {
		return domain.PublishSnapshotInput{}, inframysql.MapReadError(err, "character", "read character")
	}
	candidate := domain.PublishSnapshotInput{
		DeviceType: device.domain(), Character: character.domain(),
		Draft: domain.DeviceTypeCapabilities{DeviceTypeID: deviceTypeID},
	}
	if err := loadActiveResources(tx, &candidate); err != nil {
		return domain.PublishSnapshotInput{}, err
	}
	return candidate, nil
}

func loadActiveResources(tx *gorm.DB, candidate *domain.PublishSnapshotInput) error {
	var links []capabilityDraftLinkModel
	if err := tx.Where("device_type_id = ?", candidate.DeviceType.ID).Order("resource_id").
		Find(&links).Error; err != nil {
		return apperr.WrapDatabase(err, "load device capability draft")
	}
	for _, link := range links {
		var row resourceModel
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
			First(&row, "id = ?", link.ResourceID).Error; err != nil {
			return inframysql.MapReadError(err, "capability resource", "load capability resource")
		}
		if err := appendActiveResource(candidate, row.domain()); err != nil {
			return err
		}
	}
	return nil
}

func appendActiveResource(candidate *domain.PublishSnapshotInput, resource domain.Resource) error {
	switch resource.Kind {
	case domain.ResourceVoice:
		if candidate.Voice != nil {
			return apperr.NewBadRequest("multiple voice resources for device type")
		}
		candidate.Draft.VoiceID = resource.ID
		candidate.Voice = &resource
	case domain.ResourceTool:
		candidate.Draft.ToolIDs = append(candidate.Draft.ToolIDs, resource.ID)
		candidate.Tools = append(candidate.Tools, resource)
	case domain.ResourceSkill:
		candidate.Draft.SkillIDs = append(candidate.Draft.SkillIDs, resource.ID)
		candidate.Skills = append(candidate.Skills, resource)
	case domain.ResourceKnowledgeBase:
		candidate.Draft.KnowledgeBaseIDs = append(candidate.Draft.KnowledgeBaseIDs, resource.ID)
		candidate.KnowledgeBases = append(candidate.KnowledgeBases, resource)
	default:
		return apperr.NewBadRequest("unknown capability resource kind")
	}
	return nil
}

func persistActiveConfig(tx *gorm.DB, previousRevision int64, next activeConfigModel) error {
	if previousRevision == 0 {
		if err := tx.Omit("DeviceType").Create(&next).Error; err != nil {
			return inframysql.MapWriteError(err, "active config conflict", "create active device configuration")
		}
		return nil
	}
	result := tx.Model(&activeConfigModel{}).
		Where("device_type_id = ? AND revision = ?", next.DeviceTypeID, previousRevision).
		Updates(map[string]any{
			"revision": next.Revision, "snapshot": next.Snapshot, "applied_at": next.AppliedAt,
		})
	if result.Error != nil {
		return apperr.WrapDatabase(result.Error, "replace active device configuration")
	}
	if result.RowsAffected != 1 {
		return apperr.NewConflict("active device configuration was changed")
	}
	return nil
}
