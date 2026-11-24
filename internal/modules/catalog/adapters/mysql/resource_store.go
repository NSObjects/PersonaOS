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

// ResourceStore manages reusable capability definitions independently of device types.
type ResourceStore struct{ db *gorm.DB }

// NewResourceStore migrates the standalone resource catalog table.
// D2-3 will compose this store into the administration runtime.
func NewResourceStore(ctx context.Context, db *gorm.DB) (*ResourceStore, error) {
	if ctx == nil {
		return nil, errors.New("create resource store: nil context")
	}
	if db == nil {
		return nil, errors.New("create resource store: nil db")
	}
	if err := db.WithContext(ctx).AutoMigrate(&resourceModel{}); err != nil {
		return nil, apperr.WrapDatabase(err, "migrate capability resources")
	}
	return &ResourceStore{db: db}, nil
}

type resourceModel struct {
	ID          int64  `gorm:"primaryKey"`
	Kind        string `gorm:"type:varchar(32);not null;uniqueIndex:idx_resource_kind_code"`
	Code        string `gorm:"type:varchar(64);not null;uniqueIndex:idx_resource_kind_code"`
	Name        string `gorm:"type:varchar(160);not null"`
	Description string `gorm:"type:text"`
	Provider    string `gorm:"type:varchar(120)"`
	Reference   string `gorm:"type:varchar(512);not null"`
	ToolConfig  string `gorm:"type:text"`
	Enabled     bool   `gorm:"not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (resourceModel) TableName() string { return "capability_resources" }

func (m resourceModel) domain() domain.Resource {
	return domain.Resource{
		ID: m.ID, Kind: domain.ResourceKind(m.Kind), Code: m.Code, Name: m.Name,
		Description: m.Description, Provider: m.Provider, Reference: m.Reference, ToolConfig: m.ToolConfig,
		Enabled: m.Enabled, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func resourceRow(r domain.Resource) resourceModel {
	return resourceModel{
		ID: r.ID, Kind: string(r.Kind), Code: r.Code, Name: r.Name,
		Description: r.Description, Provider: r.Provider, Reference: r.Reference, ToolConfig: r.ToolConfig,
		Enabled: r.Enabled, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// List returns resources of one kind, sorted by ID.
func (s *ResourceStore) List(ctx context.Context, kind domain.ResourceKind) ([]domain.Resource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var rows []resourceModel
	if err := s.db.WithContext(ctx).Where("kind = ?", string(kind)).Order("id").Find(&rows).Error; err != nil {
		return nil, apperr.WrapDatabase(err, "list capability resources")
	}
	result := make([]domain.Resource, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.domain())
	}
	return result, nil
}

// Find reads a resource by ID.
func (s *ResourceStore) Find(ctx context.Context, id int64) (domain.Resource, error) {
	if err := ctx.Err(); err != nil {
		return domain.Resource{}, err
	}
	var row resourceModel
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return domain.Resource{}, inframysql.MapReadError(err, "capability resource", "find capability resource")
	}
	return row.domain(), nil
}

// Create saves a validated resource and enforces a unique code within its kind.
func (s *ResourceStore) Create(ctx context.Context, resource domain.Resource) (domain.Resource, error) {
	if err := ctx.Err(); err != nil {
		return domain.Resource{}, err
	}
	valid, err := domain.NewResource(0, resource.Kind, resource.Code, resource.Name, resource.Description, resource.Provider, resource.Reference, resource.Enabled, time.Time{}, time.Time{})
	if err != nil {
		return domain.Resource{}, apperr.WrapBadRequest(err, "invalid resource")
	}
	valid, err = valid.WithToolConfig(resource.ToolConfig)
	if err != nil {
		return domain.Resource{}, apperr.WrapBadRequest(err, "invalid tool configuration")
	}
	row := resourceRow(valid)
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return domain.Resource{}, inframysql.MapWriteError(err, "resource code already exists", "create capability resource")
	}
	return row.domain(), nil
}

// Update modifies mutable metadata, preserving kind and code.
func (s *ResourceStore) Update(ctx context.Context, resource domain.Resource) (domain.Resource, error) {
	if err := ctx.Err(); err != nil {
		return domain.Resource{}, err
	}
	old, err := s.Find(ctx, resource.ID)
	if err != nil {
		return domain.Resource{}, err
	}
	valid, err := domain.NewResource(old.ID, old.Kind, old.Code, resource.Name, resource.Description, resource.Provider, resource.Reference, resource.Enabled, old.CreatedAt, time.Now().UTC())
	if err != nil {
		return domain.Resource{}, apperr.WrapBadRequest(err, "invalid resource")
	}
	valid, err = valid.WithToolConfig(resource.ToolConfig)
	if err != nil {
		return domain.Resource{}, apperr.WrapBadRequest(err, "invalid tool configuration")
	}
	result := s.db.WithContext(ctx).Model(&resourceModel{}).Where("id = ?", valid.ID).
		Updates(map[string]any{
			"name": valid.Name, "description": valid.Description, "provider": valid.Provider,
			"reference": valid.Reference, "tool_config": valid.ToolConfig, "enabled": valid.Enabled, "updated_at": valid.UpdatedAt,
		})
	if result.Error != nil {
		return domain.Resource{}, inframysql.MapWriteError(result.Error, "resource update conflict", "update capability resource")
	}
	if result.RowsAffected == 0 {
		return domain.Resource{}, apperr.NewNotFound("capability resource")
	}
	return s.Find(ctx, valid.ID)
}

// Delete removes an unbound resource. D2-3 will enforce reference protection
// when device-type association tables are introduced.
func (s *ResourceStore) Delete(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result := s.db.WithContext(ctx).Delete(&resourceModel{}, "id = ?", id)
	if result.Error != nil {
		var foreignKey *driver.MySQLError
		if errors.As(result.Error, &foreignKey) && foreignKey.Number == 1451 {
			return apperr.NewConflict("resource is in use by device types")
		}
		return apperr.WrapDatabase(result.Error, "delete capability resource")
	}
	if result.RowsAffected == 0 {
		return apperr.NewNotFound("capability resource")
	}
	return nil
}
