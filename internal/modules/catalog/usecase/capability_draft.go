package usecase

import (
	"context"
	"fmt"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// CapabilityDraftStore reads and atomically replaces unpublished device-type selections.
type CapabilityDraftStore interface {
	Find(context.Context, int64) (domain.DeviceTypeCapabilities, error)
	Save(context.Context, domain.DeviceTypeCapabilities) (domain.DeviceTypeCapabilities, error)
}

// CapabilityDraftUsecase coordinates draft reads and writes without publishing them.
type CapabilityDraftUsecase struct{ store CapabilityDraftStore }

// NewCapabilityDraftUsecase constructs the draft usecase.
func NewCapabilityDraftUsecase(store CapabilityDraftStore) *CapabilityDraftUsecase {
	return &CapabilityDraftUsecase{store: store}
}

// CapabilityDraftInput contains editable device-type resource references.
type CapabilityDraftInput struct {
	VoiceID          int64   `json:"voice_id"`
	ToolIDs          []int64 `json:"tool_ids"`
	SkillIDs         []int64 `json:"skill_ids"`
	KnowledgeBaseIDs []int64 `json:"knowledge_base_ids"`
}

func (u *CapabilityDraftUsecase) ready(ctx context.Context, deviceTypeID int64) error {
	if ctx == nil || deviceTypeID <= 0 {
		return apperr.NewBadRequest("context and positive device type id are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if u == nil || u.store == nil {
		return apperr.New(apperr.ErrInternalServer, "capability draft store is not configured")
	}
	return nil
}

// Get returns an unpublished device-type capability selection.
func (u *CapabilityDraftUsecase) Get(ctx context.Context, deviceTypeID int64) (domain.DeviceTypeCapabilities, error) {
	if err := u.ready(ctx, deviceTypeID); err != nil {
		return domain.DeviceTypeCapabilities{}, err
	}
	result, err := u.store.Find(ctx, deviceTypeID)
	if err != nil {
		return domain.DeviceTypeCapabilities{}, fmt.Errorf("get capability draft: %w", err)
	}
	return result.Clone(), nil
}

// Save validates selection syntax and delegates atomic reference checks to storage.
func (u *CapabilityDraftUsecase) Save(ctx context.Context, deviceTypeID int64, in CapabilityDraftInput) (domain.DeviceTypeCapabilities, error) {
	if err := u.ready(ctx, deviceTypeID); err != nil {
		return domain.DeviceTypeCapabilities{}, err
	}
	selection, err := domain.NewDeviceTypeCapabilities(deviceTypeID, in.VoiceID, in.ToolIDs, in.SkillIDs, in.KnowledgeBaseIDs)
	if err != nil {
		return domain.DeviceTypeCapabilities{}, apperr.WrapBadRequest(err, "invalid capability draft")
	}
	result, err := u.store.Save(ctx, selection)
	if err != nil {
		return domain.DeviceTypeCapabilities{}, fmt.Errorf("save capability draft: %w", err)
	}
	return result.Clone(), nil
}
