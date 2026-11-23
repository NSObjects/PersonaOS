package usecase

import (
	"context"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// ActiveConfigStore reads confirmed configuration and atomically applies a
// device type's current draft. No version history is exposed for the Demo.
type ActiveConfigStore interface {
	Apply(context.Context, int64, int64) (domain.PublishedDeviceConfig, error)
	FindActive(context.Context, int64) (domain.PublishedDeviceConfig, error)
}

// ActiveConfigUsecase is the shared read boundary for the Demo Agent.
type ActiveConfigUsecase struct{ store ActiveConfigStore }

// NewActiveConfigUsecase constructs confirmed configuration operations.
func NewActiveConfigUsecase(store ActiveConfigStore) *ActiveConfigUsecase {
	return &ActiveConfigUsecase{store: store}
}

// Apply confirms one dynamically created device type's current configuration.
func (u *ActiveConfigUsecase) Apply(ctx context.Context, deviceTypeID, actorID int64) (domain.PublishedDeviceConfig, error) {
	if err := u.ready(ctx, deviceTypeID); err != nil {
		return domain.PublishedDeviceConfig{}, err
	}
	if actorID <= 0 {
		return domain.PublishedDeviceConfig{}, apperr.NewBadRequest("publisher is required")
	}
	return u.store.Apply(ctx, deviceTypeID, actorID)
}

// Get returns only confirmed configuration; never falls back to the draft.
func (u *ActiveConfigUsecase) Get(ctx context.Context, deviceTypeID int64) (domain.PublishedDeviceConfig, error) {
	if err := u.ready(ctx, deviceTypeID); err != nil {
		return domain.PublishedDeviceConfig{}, err
	}
	result, err := u.store.FindActive(ctx, deviceTypeID)
	if err != nil {
		return domain.PublishedDeviceConfig{}, err
	}
	return result.Clone(), nil
}

func (u *ActiveConfigUsecase) ready(ctx context.Context, deviceTypeID int64) error {
	if ctx == nil || deviceTypeID <= 0 {
		return apperr.NewBadRequest("valid context and device type are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if u == nil || u.store == nil {
		return apperr.New(apperr.ErrInternalServer, "active configuration store is unavailable")
	}
	return nil
}
