package cataloghttp_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/NSObjects/PersonaOS/internal/modules/audit/oprec"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	cataloghttp "github.com/NSObjects/PersonaOS/internal/modules/catalog/http"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

type resourceStoreStub struct {
	values map[int64]domain.Resource
}

func (s *resourceStoreStub) List(_ context.Context, kind domain.ResourceKind) ([]domain.Resource, error) {
	results := make([]domain.Resource, 0)
	for _, value := range s.values {
		if value.Kind == kind {
			results = append(results, value)
		}
	}
	return results, nil
}

func (s *resourceStoreStub) Find(_ context.Context, id int64) (domain.Resource, error) {
	value, ok := s.values[id]
	if !ok {
		return domain.Resource{}, apperr.NewNotFound("resource")
	}
	return value, nil
}

func (s *resourceStoreStub) Create(_ context.Context, value domain.Resource) (domain.Resource, error) {
	value.ID = 1
	s.values[value.ID] = value
	return value, nil
}

func (s *resourceStoreStub) Update(_ context.Context, value domain.Resource) (domain.Resource, error) {
	s.values[value.ID] = value
	return value, nil
}

func (s *resourceStoreStub) Delete(_ context.Context, id int64) error {
	delete(s.values, id)
	return nil
}

// TestCapabilityResourceHTTPCRUD exercises registered routes and audit records.
func TestCapabilityResourceHTTPCRUD(t *testing.T) {
	e := testEcho()
	audit := &fakeAudit{}
	cataloghttp.RegisterResources(e.Group("/api"), cataloghttp.NewResourceHandler(
		usecase.NewResourceUsecase(&resourceStoreStub{values: make(map[int64]domain.Resource)}),
		oprec.New(audit),
	))

	created := call(e, http.MethodPost, "/api/capability-resources", `{"kind":"voice","code":"voice_a","name":"Voice A","provider":"aliyun","reference":"voice-id","enabled":true}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST status=%d, body=%s", created.Code, created.Body.String())
	}
	list := call(e, http.MethodGet, "/api/capability-resources?kind=voice", "")
	if list.Code != http.StatusOK {
		t.Fatalf("GET list status=%d, body=%s", list.Code, list.Body.String())
	}
	if got := call(e, http.MethodGet, "/api/capability-resources/1", ""); got.Code != http.StatusOK {
		t.Fatalf("GET resource status=%d, body=%s", got.Code, got.Body.String())
	}
	updated := call(e, http.MethodPatch, "/api/capability-resources/1", `{"name":"Voice B","provider":"aliyun","reference":"voice-id-2","enabled":false}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("PATCH status=%d, body=%s", updated.Code, updated.Body.String())
	}
	if got := call(e, http.MethodDelete, "/api/capability-resources/1", ""); got.Code != http.StatusOK {
		t.Fatalf("DELETE status=%d, body=%s", got.Code, got.Body.String())
	}
	if len(audit.records) != 3 {
		t.Fatalf("audit records=%d, want 3", len(audit.records))
	}
	if got := call(e, http.MethodGet, "/api/capability-resources/1", ""); got.Code != http.StatusNotFound {
		t.Fatalf("deleted resource GET status=%d, body=%s", got.Code, got.Body.String())
	}
}

// TestCapabilityResourceHTTPInvalidInputs verifies API errors without mutation.
func TestCapabilityResourceHTTPInvalidInputs(t *testing.T) {
	e := testEcho()
	audit := &fakeAudit{}
	cataloghttp.RegisterResources(e.Group("/api"), cataloghttp.NewResourceHandler(
		usecase.NewResourceUsecase(&resourceStoreStub{values: make(map[int64]domain.Resource)}),
		oprec.New(audit),
	))
	for _, tt := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/capability-resources", ""},
		{http.MethodGet, "/api/capability-resources?kind=camera", ""},
		{http.MethodGet, "/api/capability-resources/nope", ""},
		{http.MethodPost, "/api/capability-resources", `{"kind":"voice","code":"voice_a","name":"Voice A","reference":"voice-id"}`},
	} {
		got := call(e, tt.method, tt.path, tt.body)
		if got.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status=%d, body=%s", tt.method, tt.path, got.Code, got.Body.String())
		}
	}
}

func TestKnowledgeUploadHTTPRejectsInvalidDocument(t *testing.T) {
	e := testEcho()
	cataloghttp.RegisterResources(e.Group("/api"), cataloghttp.NewResourceHandler(
		usecase.NewResourceUsecase(&resourceStoreStub{values: make(map[int64]domain.Resource)}),
		oprec.New(&fakeAudit{}),
	))
	got := call(e, http.MethodPost, "/api/capability-resources", `{"kind":"knowledge_base","code":"lore","name":"Lore","knowledge_filename":"lore.pdf","knowledge_text":"invalid"}`)
	if got.Code != http.StatusBadRequest {
		t.Fatalf("POST status=%d, body=%s", got.Code, got.Body.String())
	}
}
