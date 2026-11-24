package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

const (
	uploadFilename = "lore.md"
	uploadContent  = "角色徽章编号为 LX-7429。"
)

type knowledgeImportFixture struct {
	sources map[string]string
	indexed map[string]string
	fail    bool
}

func (f *knowledgeImportFixture) SaveKnowledgeSource(_ context.Context, version, filename, text string) error {
	f.sources[version] = filename + ":" + text
	return nil
}

func (f *knowledgeImportFixture) EnsureIndex(context.Context) error { return nil }

func (f *knowledgeImportFixture) IngestText(_ context.Context, version, text string) error {
	if f.fail {
		return errors.New("embedding unavailable")
	}
	f.indexed[version] = text
	return nil
}

func TestKnowledgeImportCreatesImmutableVersion(t *testing.T) {
	ctx := context.Background()
	fixture := &knowledgeImportFixture{sources: make(map[string]string), indexed: make(map[string]string)}
	uc := usecase.NewResourceUsecase(&resourceMemoryStore{rows: make(map[int64]domain.Resource)}).
		WithKnowledgeImport(fixture, fixture)
	input := usecase.ResourceInput{
		Kind: domain.ResourceKnowledgeBase, Code: "lore", Name: "角色资料", Enabled: true,
		KnowledgeFilename: uploadFilename, KnowledgeText: uploadContent,
	}
	created, err := uc.Create(ctx, input)
	if err != nil || created.Provider != "redis" || !strings.HasPrefix(created.Reference, "kb-") {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	if fixture.indexed[created.Reference] != uploadContent || fixture.sources[created.Reference] != uploadFilename+":"+uploadContent {
		t.Fatal("original document or vector index missing")
	}
	input.KnowledgeText = "新版角色资料"
	updated, err := uc.Update(ctx, created.ID, input)
	if err != nil || updated.Reference == created.Reference || fixture.indexed[updated.Reference] != input.KnowledgeText {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	if fixture.indexed[created.Reference] != uploadContent {
		t.Fatal("old version was overwritten")
	}
	fixture.fail = true
	if _, err := uc.Update(ctx, created.ID, input); err == nil {
		t.Fatal("failed indexing accepted")
	}
	current, err := uc.Find(ctx, created.ID)
	if err != nil || current.Reference != updated.Reference {
		t.Fatalf("failed upload changed current version: %+v %v", current, err)
	}
}

func TestKnowledgeImportRejectsInvalidDocuments(t *testing.T) {
	fixture := &knowledgeImportFixture{sources: make(map[string]string), indexed: make(map[string]string)}
	uc := usecase.NewResourceUsecase(&resourceMemoryStore{rows: make(map[int64]domain.Resource)}).
		WithKnowledgeImport(fixture, fixture)
	for _, name := range []string{"../lore.md", "lore.pdf", ""} {
		_, err := uc.Create(context.Background(), usecase.ResourceInput{
			Kind: domain.ResourceKnowledgeBase, Code: "lore", Name: "角色资料",
			KnowledgeFilename: name, KnowledgeText: uploadContent,
		})
		requireCode(t, err, apperr.ErrBadRequest)
	}
	if len(fixture.sources) != 0 || len(fixture.indexed) != 0 {
		t.Fatal("invalid document caused persistence")
	}
}
