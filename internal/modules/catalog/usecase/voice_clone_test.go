package usecase_test

import (
	"context"
	"strings"
	"testing"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

type clonerFixture struct {
	status domain.VoiceCloneStatus
	trains int
}

func (f *clonerFixture) Query(context.Context, string) (domain.VoiceCloneStatus, error) {
	return f.status, nil
}

func (f *clonerFixture) Train(_ context.Context, speaker, _, _ string, _ []byte) (domain.VoiceCloneStatus, error) {
	f.trains++
	f.status = domain.VoiceCloneStatus{SpeakerID: speaker, State: 2, TrainingTimes: 14, ModelReady: true}
	return f.status, nil
}

func TestCloneVoiceRequiresConsentAndRemainsDisabled(t *testing.T) {
	ctx := context.Background()
	fixture := &clonerFixture{}
	uc := usecase.NewResourceUsecase(&resourceMemoryStore{rows: make(map[int64]domain.Resource)}).WithVoiceCloner(fixture)
	resource, err := uc.CreateClone(ctx, "专属音色")
	if err != nil || resource.Enabled || !strings.HasPrefix(resource.Reference, "PersonaOS_") || fixture.trains != 0 {
		t.Fatalf("reserved clone=%+v err=%v", resource, err)
	}
	const format = "wav"
	sample := []byte("fixture")
	_, err = uc.TrainClone(ctx, resource.ID, format, "", sample, false)
	requireCode(t, err, apperr.ErrBadRequest)
	status, err := uc.TrainClone(ctx, resource.ID, format, "", sample, true)
	if err != nil || !status.ModelReady || fixture.trains != 1 {
		t.Fatalf("training=%+v err=%v", status, err)
	}
	_, err = uc.Update(ctx, resource.ID, usecase.ResourceInput{Enabled: true})
	requireCode(t, err, apperr.ErrBadRequest)
	saved, err := uc.Find(ctx, resource.ID)
	if err != nil || saved.Enabled {
		t.Fatalf("unconfirmed activation: %+v %v", saved, err)
	}
	fixture.status.State = 4
	_, err = uc.TrainClone(ctx, resource.ID, format, "", sample, true)
	requireCode(t, err, apperr.ErrConflict)
	if fixture.trains != 1 {
		t.Fatal("activated voice was retrained")
	}
}

func TestEnableCloneUsesProviderSpeakerAndRequiresConfirmation(t *testing.T) {
	ctx := context.Background()
	fixture := &clonerFixture{}
	uc := usecase.NewResourceUsecase(&resourceMemoryStore{rows: make(map[int64]domain.Resource)}).WithVoiceCloner(fixture)
	resource, err := uc.CreateClone(ctx, "克隆")
	if err != nil {
		t.Fatal(err)
	}
	_, err = uc.EnableClone(ctx, resource.ID, false)
	requireCode(t, err, apperr.ErrBadRequest)
	_, err = uc.EnableClone(ctx, resource.ID, true)
	requireCode(t, err, apperr.ErrConflict)
	fixture.status = domain.VoiceCloneStatus{SpeakerID: "S_provider_fixture", State: 2, ModelReady: true}
	saved, err := uc.EnableClone(ctx, resource.ID, true)
	if err != nil || !saved.Enabled || saved.Reference != fixture.status.SpeakerID {
		t.Fatalf("enabled resource=%+v err=%v", saved, err)
	}
	_, err = uc.TrainClone(ctx, resource.ID, "wav", "", []byte("fixture"), true)
	requireCode(t, err, apperr.ErrConflict)
}

func TestCloneCatalogMetadataDoesNotChangeIdentityOrEnable(t *testing.T) {
	ctx := context.Background()
	uc := usecase.NewResourceUsecase(&resourceMemoryStore{rows: make(map[int64]domain.Resource)}).WithVoiceCloner(&clonerFixture{})
	resource, err := uc.CreateClone(ctx, "初始名称")
	if err != nil {
		t.Fatal(err)
	}
	input := usecase.ResourceInput{
		Name: "新名称", Description: "角色音色", Provider: resource.Provider, Reference: resource.Reference,
	}
	saved, err := uc.Update(ctx, resource.ID, input)
	if err != nil || saved.Name != input.Name || saved.Reference != resource.Reference || saved.Enabled {
		t.Fatalf("updated clone=%+v err=%v", saved, err)
	}
	input.Enabled = true
	_, err = uc.Update(ctx, resource.ID, input)
	requireCode(t, err, apperr.ErrBadRequest)
	input.Enabled = false
	input.Reference = "other-voice"
	_, err = uc.Update(ctx, resource.ID, input)
	requireCode(t, err, apperr.ErrBadRequest)
}
