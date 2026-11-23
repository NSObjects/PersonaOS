package mysql

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	inframysql "github.com/NSObjects/PersonaOS/internal/platform/infrastructure/mysql"
)

// CapabilityDraftStore persists unpublished resource selection per device type.
// The application must not interpret these rows as active runtime grants.
type CapabilityDraftStore struct{ db *gorm.DB }

// NewCapabilityDraftStore creates the association table using the existing DB.
func NewCapabilityDraftStore(ctx context.Context, db *gorm.DB) (*CapabilityDraftStore, error) {
	if ctx == nil {
		return nil, errors.New("create capability draft store: nil context")
	}
	if db == nil {
		return nil, errors.New("create capability draft store: nil db")
	}
	if err := db.WithContext(ctx).AutoMigrate(&capabilityDraftLinkModel{}); err != nil {
		return nil, apperr.WrapDatabase(err, "migrate device capability drafts")
	}
	return &CapabilityDraftStore{db: db}, nil
}

// Separate FK constraints protect resources and devices against orphan links.
// Device and resource deletion must first remove the corresponding draft links.
type capabilityDraftLinkModel struct {
	ID           int64           `gorm:"primaryKey"`
	DeviceTypeID int64           `gorm:"not null;uniqueIndex:idx_device_resource"`
	DeviceType   deviceTypeModel `gorm:"foreignKey:DeviceTypeID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	ResourceID   int64           `gorm:"not null;uniqueIndex:idx_device_resource"`
	Resource     resourceModel   `gorm:"foreignKey:ResourceID;references:ID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
}

func (capabilityDraftLinkModel) TableName() string { return "device_type_capability_drafts" }

// Find returns the current unpublished resource selection. Unconfigured
// device types have an empty selection, not a missing configuration.
func (s *CapabilityDraftStore) Find(ctx context.Context, deviceTypeID int64) (domain.DeviceTypeCapabilities, error) {
	if ctx == nil {
		return domain.DeviceTypeCapabilities{}, apperr.NewBadRequest("context is required")
	}
	if err := ctx.Err(); err != nil {
		return domain.DeviceTypeCapabilities{}, err
	}
	if deviceTypeID <= 0 {
		return domain.DeviceTypeCapabilities{}, apperr.NewBadRequest("invalid device type id")
	}
	var device deviceTypeModel
	if err := s.db.WithContext(ctx).First(&device, "id = ?", deviceTypeID).Error; err != nil {
		return domain.DeviceTypeCapabilities{}, inframysql.MapReadError(err, "device type", "find device capability draft")
	}
	var rows []capabilityDraftLinkModel
	if err := s.db.WithContext(ctx).Preload("Resource").Where("device_type_id = ?", deviceTypeID).Find(&rows).Error; err != nil {
		return domain.DeviceTypeCapabilities{}, apperr.WrapDatabase(err, "read device capability draft")
	}
	selection := domain.DeviceTypeCapabilities{DeviceTypeID: deviceTypeID}
	for _, row := range rows {
		switch domain.ResourceKind(row.Resource.Kind) {
		case domain.ResourceVoice:
			if selection.VoiceID != 0 {
				return domain.DeviceTypeCapabilities{}, apperr.New(apperr.ErrInternalServer, "multiple voice resources for device type")
			}
			selection.VoiceID = row.ResourceID
		case domain.ResourceTool:
			selection.ToolIDs = append(selection.ToolIDs, row.ResourceID)
		case domain.ResourceSkill:
			selection.SkillIDs = append(selection.SkillIDs, row.ResourceID)
		case domain.ResourceKnowledgeBase:
			selection.KnowledgeBaseIDs = append(selection.KnowledgeBaseIDs, row.ResourceID)
		default:
			return domain.DeviceTypeCapabilities{}, apperr.New(apperr.ErrInternalServer, "unknown capability resource kind")
		}
	}
	return domain.NewDeviceTypeCapabilities(deviceTypeID, selection.VoiceID, selection.ToolIDs, selection.SkillIDs, selection.KnowledgeBaseIDs)
}

// Save atomically validates resource existence, active state and kind, then
// replaces all links for a product model. It never publishes a configuration.
func (s *CapabilityDraftStore) Save(ctx context.Context, selection domain.DeviceTypeCapabilities) (domain.DeviceTypeCapabilities, error) {
	if ctx == nil {
		return domain.DeviceTypeCapabilities{}, apperr.NewBadRequest("context is required")
	}
	if err := ctx.Err(); err != nil {
		return domain.DeviceTypeCapabilities{}, err
	}
	valid, err := domain.NewDeviceTypeCapabilities(selection.DeviceTypeID, selection.VoiceID, selection.ToolIDs, selection.SkillIDs, selection.KnowledgeBaseIDs)
	if err != nil {
		return domain.DeviceTypeCapabilities{}, apperr.WrapBadRequest(err, "invalid capability selection")
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var device deviceTypeModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&device, "id = ?", valid.DeviceTypeID).Error; err != nil {
			return inframysql.MapReadError(err, "device type", "lock device capability draft")
		}
		links := make([]capabilityDraftLinkModel, 0, 1+len(valid.ToolIDs)+len(valid.SkillIDs)+len(valid.KnowledgeBaseIDs))
		add := func(ids []int64, kind domain.ResourceKind) error {
			for _, id := range ids {
				var resource resourceModel
				if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&resource, "id = ?", id).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return apperr.NewBadRequest(fmt.Sprintf("%s resource %d does not exist", kind, id))
					}
					return apperr.WrapDatabase(err, "validate device capability resource")
				}
				if domain.ResourceKind(resource.Kind) != kind || !resource.Enabled {
					return apperr.NewBadRequest(fmt.Sprintf("%s resource %d is unavailable", kind, id))
				}
				links = append(links, capabilityDraftLinkModel{DeviceTypeID: valid.DeviceTypeID, ResourceID: id})
			}
			return nil
		}
		if valid.VoiceID != 0 {
			if err := add([]int64{valid.VoiceID}, domain.ResourceVoice); err != nil {
				return err
			}
		}
		for _, group := range []struct {
			ids  []int64
			kind domain.ResourceKind
		}{
			{valid.ToolIDs, domain.ResourceTool},
			{valid.SkillIDs, domain.ResourceSkill},
			{valid.KnowledgeBaseIDs, domain.ResourceKnowledgeBase},
		} {
			if err := add(group.ids, group.kind); err != nil {
				return err
			}
		}
		if err := tx.Where("device_type_id = ?", valid.DeviceTypeID).Delete(&capabilityDraftLinkModel{}).Error; err != nil {
			return apperr.WrapDatabase(err, "clear capability draft")
		}
		if len(links) > 0 {
			if err := tx.Omit("DeviceType", "Resource").Create(&links).Error; err != nil {
				return inframysql.MapWriteError(err, "capability resource is in use", "save capability draft")
			}
		}
		return nil
	})
	if err != nil {
		return domain.DeviceTypeCapabilities{}, err
	}
	return valid.Clone(), nil
}
