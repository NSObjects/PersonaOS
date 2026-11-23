package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

type resourceMemoryStore struct {
	next int64
	rows map[int64]domain.Resource
}

func (s *resourceMemoryStore) List(_ context.Context, kind domain.ResourceKind) ([]domain.Resource, error) {
	out := make([]domain.Resource, 0)
	for _, v := range s.rows {
		if v.Kind == kind {
			out = append(out, v)
		}
	}
	return out, nil
}

func (s *resourceMemoryStore) Find(_ context.Context, id int64) (domain.Resource, error) {
	if v, ok := s.rows[id]; ok {
		return v, nil
	}
	return domain.Resource{}, apperr.NewNotFound("resource")
}

func (s *resourceMemoryStore) Create(_ context.Context, v domain.Resource) (domain.Resource, error) {
	s.next++
	v.ID = s.next
	v.CreatedAt = time.Now().UTC()
	v.UpdatedAt = v.CreatedAt
	s.rows[v.ID] = v
	return v, nil
}

func (s *resourceMemoryStore) Update(_ context.Context, v domain.Resource) (domain.Resource, error) {
	s.rows[v.ID] = v
	return v, nil
}

func (s *resourceMemoryStore) Delete(_ context.Context, id int64) error {
	delete(s.rows, id)
	return nil
}

func TestResourceUsecaseLifecycle(t *testing.T) {
	ctx := context.Background()
	uc := usecase.NewResourceUsecase(&resourceMemoryStore{rows: make(map[int64]domain.Resource)})
	_, err := uc.Create(ctx, usecase.ResourceInput{Kind: domain.ResourceVoice, Code: "voice_a", Name: "Invalid", Reference: "voice-a"})
	requireCode(t, err, apperr.ErrBadRequest)
	voice, err := uc.Create(ctx, usecase.ResourceInput{
		Kind: domain.ResourceVoice, Code: "VOICE_A", Name: "Voice", Provider: "aliyun", Reference: "voice-a", Enabled: true,
	})
	if err != nil || voice.Code != "voice_a" {
		t.Fatalf("Create() = %+v, %v", voice, err)
	}
	got, err := uc.Find(ctx, voice.ID)
	if err != nil || got.ID != voice.ID {
		t.Fatalf("Find() = %+v, %v", got, err)
	}
	updated, err := uc.Update(ctx, voice.ID, usecase.ResourceInput{
		Kind: domain.ResourceTool, Code: "changed", Name: "New voice", Provider: "aliyun", Reference: "voice-b",
	})
	if err != nil || updated.Kind != domain.ResourceVoice || updated.Code != "voice_a" || updated.Enabled {
		t.Fatalf("Update() = %+v, %v", updated, err)
	}
	voices, err := uc.List(ctx, domain.ResourceVoice)
	if err != nil || len(voices) != 1 {
		t.Fatalf("List() = %+v, %v", voices, err)
	}
	_, err = uc.List(ctx, "camera")
	requireCode(t, err, apperr.ErrBadRequest)
	if err := uc.Delete(ctx, voice.ID); err != nil {
		t.Fatal(err)
	}
	_, err = uc.Find(ctx, voice.ID)
	requireCode(t, err, apperr.ErrNotFound)
}

func TestResourceUsecaseRejectsInvalidContextAndIDs(t *testing.T) {
	uc := usecase.NewResourceUsecase(&resourceMemoryStore{rows: make(map[int64]domain.Resource)})
	_, err := uc.Find(context.Background(), 0)
	requireCode(t, err, apperr.ErrBadRequest)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = uc.List(ctx, domain.ResourceVoice)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want context.Canceled", err)
	}
}
