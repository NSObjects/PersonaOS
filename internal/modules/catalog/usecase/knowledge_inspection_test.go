package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

type knowledgeInspectorFixture struct {
	version string
	count   int64
	err     error
}

func (f *knowledgeInspectorFixture) VersionChunkCount(_ context.Context, version string) (int64, error) {
	f.version = version
	return f.count, f.err
}

func (f *knowledgeInspectorFixture) PreviewVersion(_ context.Context, version, _ string) ([]domain.KnowledgeHit, error) {
	f.version = version
	return []domain.KnowledgeHit{{Source: "chunk", Content: uploadContent}}, f.err
}

func TestKnowledgeDiagnosticsUseResourceVersion(t *testing.T) {
	ctx := context.Background()
	uc := usecase.NewResourceUsecase(&resourceMemoryStore{rows: make(map[int64]domain.Resource)})
	resource, err := uc.Create(ctx, usecase.ResourceInput{
		Kind: domain.ResourceKnowledgeBase, Code: "inspect_lore", Name: "Lore",
		Provider: "redis", Reference: "immutable-v1", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := uc.InspectKnowledge(ctx, resource.ID)
	if err != nil || status.State != "unconfigured" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	fixture := &knowledgeInspectorFixture{}
	uc.WithKnowledgeInspector(fixture)
	for _, tc := range []struct {
		count int64
		err   error
		state string
	}{
		{0, nil, "not_indexed"},
		{2, nil, "indexed"},
		{0, errors.New("backend unavailable"), "unavailable"},
	} {
		fixture.count, fixture.err = tc.count, tc.err
		status, err = uc.InspectKnowledge(ctx, resource.ID)
		if err != nil || status.State != tc.state || status.Chunks != tc.count || fixture.version != resource.Reference {
			t.Fatalf("status=%+v version=%s err=%v", status, fixture.version, err)
		}
	}
	fixture.err = nil
	result, err := uc.SearchKnowledge(ctx, resource.ID, "徽章")
	if err != nil || result.Version != resource.Reference || len(result.Hits) != 1 || fixture.version != resource.Reference {
		t.Fatalf("preview=%+v err=%v", result, err)
	}
	_, err = uc.SearchKnowledge(ctx, resource.ID, " ")
	requireCode(t, err, apperr.ErrBadRequest)
}
