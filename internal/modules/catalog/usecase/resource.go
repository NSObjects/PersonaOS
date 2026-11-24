package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// ResourceStore persists independent capability definitions.
type ResourceStore interface {
	List(context.Context, domain.ResourceKind) ([]domain.Resource, error)
	Find(context.Context, int64) (domain.Resource, error)
	Create(context.Context, domain.Resource) (domain.Resource, error)
	Update(context.Context, domain.Resource) (domain.Resource, error)
	Delete(context.Context, int64) error
}

// ResourceUsecase coordinates catalog metadata without executing resources.
type ResourceUsecase struct{ store ResourceStore }

// NewResourceUsecase constructs capability resource management.
func NewResourceUsecase(store ResourceStore) *ResourceUsecase {
	return &ResourceUsecase{store: store}
}

// ResourceInput contains editable resource metadata.
type ResourceInput struct {
	Kind        domain.ResourceKind `json:"kind"`
	Code        string              `json:"code"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Provider    string              `json:"provider"`
	Reference   string              `json:"reference"`
	ToolConfig  string              `json:"tool_config"`
	Enabled     bool                `json:"enabled"`
}

func (u *ResourceUsecase) ready(ctx context.Context) error {
	if ctx == nil {
		return apperr.NewBadRequest("context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if u == nil || u.store == nil {
		return apperr.New(apperr.ErrInternalServer, "resource store is not configured")
	}
	return nil
}

// List filters resources by their kind.
func (u *ResourceUsecase) List(ctx context.Context, kind domain.ResourceKind) ([]domain.Resource, error) {
	if err := u.ready(ctx); err != nil {
		return nil, err
	}
	switch kind {
	case domain.ResourceVoice, domain.ResourceTool, domain.ResourceSkill, domain.ResourceKnowledgeBase:
	default:
		return nil, apperr.NewBadRequest("invalid resource kind")
	}
	return u.store.List(ctx, kind)
}

// Find returns one reusable resource.
func (u *ResourceUsecase) Find(ctx context.Context, id int64) (domain.Resource, error) {
	if err := u.ready(ctx); err != nil {
		return domain.Resource{}, err
	}
	if id <= 0 {
		return domain.Resource{}, apperr.NewBadRequest("invalid resource id")
	}
	return u.store.Find(ctx, id)
}

// Create validates metadata and persists an independent resource.
func (u *ResourceUsecase) Create(ctx context.Context, in ResourceInput) (domain.Resource, error) {
	if err := u.ready(ctx); err != nil {
		return domain.Resource{}, err
	}
	value, err := domain.NewResource(0, in.Kind, in.Code, in.Name, in.Description, in.Provider, in.Reference, in.Enabled, time.Time{}, time.Time{})
	if err != nil {
		return domain.Resource{}, apperr.WrapBadRequest(err, "invalid resource")
	}
	value, err = value.WithToolConfig(in.ToolConfig)
	if err != nil {
		return domain.Resource{}, apperr.WrapBadRequest(err, "invalid tool configuration")
	}
	result, err := u.store.Create(ctx, value)
	if err != nil {
		return domain.Resource{}, fmt.Errorf("create resource: %w", err)
	}
	return result, nil
}

// Update preserves the existing resource kind and immutable code.
func (u *ResourceUsecase) Update(ctx context.Context, id int64, in ResourceInput) (domain.Resource, error) {
	existing, err := u.Find(ctx, id)
	if err != nil {
		return domain.Resource{}, err
	}
	value, err := domain.NewResource(id, existing.Kind, existing.Code, in.Name, in.Description, in.Provider, in.Reference, in.Enabled, existing.CreatedAt, existing.UpdatedAt)
	if err != nil {
		return domain.Resource{}, apperr.WrapBadRequest(err, "invalid resource")
	}
	value, err = value.WithToolConfig(in.ToolConfig)
	if err != nil {
		return domain.Resource{}, apperr.WrapBadRequest(err, "invalid tool configuration")
	}
	updated, err := u.store.Update(ctx, value)
	if err != nil {
		return domain.Resource{}, fmt.Errorf("update resource: %w", err)
	}
	return updated, nil
}

// Delete delegates reference protection to database constraints.
func (u *ResourceUsecase) Delete(ctx context.Context, id int64) error {
	if _, err := u.Find(ctx, id); err != nil {
		return err
	}
	if err := u.store.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete resource: %w", err)
	}
	return nil
}
