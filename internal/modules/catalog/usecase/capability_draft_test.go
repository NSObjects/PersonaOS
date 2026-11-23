package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

type draftFake struct {
	current domain.DeviceTypeCapabilities
	saves   int
}

func (f *draftFake) Find(_ context.Context, _ int64) (domain.DeviceTypeCapabilities, error) {
	return f.current.Clone(), nil
}

func (f *draftFake) Save(_ context.Context, v domain.DeviceTypeCapabilities) (domain.DeviceTypeCapabilities, error) {
	f.saves++
	f.current = v.Clone()
	return f.current.Clone(), nil
}

func TestCapabilityDraftUsecaseValidatesAndCopies(t *testing.T) {
	ctx := context.Background()
	store := &draftFake{current: domain.DeviceTypeCapabilities{DeviceTypeID: 9}}
	uc := usecase.NewCapabilityDraftUsecase(store)
	_, err := uc.Save(ctx, 9, usecase.CapabilityDraftInput{ToolIDs: []int64{3, 3}})
	requireCode(t, err, apperr.ErrBadRequest)
	if store.saves != 0 {
		t.Fatalf("invalid input reached storage: saves=%d", store.saves)
	}
	ids := []int64{4, 2}
	saved, err := uc.Save(ctx, 9, usecase.CapabilityDraftInput{ToolIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	ids[0] = 99
	saved.ToolIDs[0] = 99
	got, err := uc.Get(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	if got.ToolIDs[0] != 2 || got.ToolIDs[1] != 4 {
		t.Fatalf("stored refs modified by client: %+v", got)
	}
	requireCode(t, func() error {
		_, err := uc.Get(ctx, 0)
		return err
	}(), apperr.ErrBadRequest)
}

func TestCapabilityDraftUsecaseStopsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	uc := usecase.NewCapabilityDraftUsecase(&draftFake{})
	_, err := uc.Save(ctx, 7, usecase.CapabilityDraftInput{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Save(canceled) = %v, want context.Canceled", err)
	}
}
