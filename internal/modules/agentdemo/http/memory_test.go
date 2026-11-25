package agentdemohttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
)

type memoryConfigFixture struct{}

func (memoryConfigFixture) Get(context.Context, int64) (domain.PublishedDeviceConfig, error) {
	return domain.PublishedDeviceConfig{Character: domain.Character{ID: 9, Name: "Memory Demo"}}, nil
}

type memoryHTTPStore struct {
	facts map[memory.Scope][]memory.Fact
}

func (s *memoryHTTPStore) ListFacts(_ context.Context, scope memory.Scope, _ int) ([]memory.Fact, error) {
	return s.facts[scope], nil
}

func (*memoryHTTPStore) SaveExchange(context.Context, memory.Exchange) error { return nil }

func (*memoryHTTPStore) UpsertFacts(context.Context, memory.Scope, []memory.Fact) error {
	return nil
}

func (s *memoryHTTPStore) DeleteFact(_ context.Context, scope memory.Scope, key string) error {
	for i, fact := range s.facts[scope] {
		if fact.Key == key {
			s.facts[scope] = append(s.facts[scope][:i], s.facts[scope][i+1:]...)
			break
		}
	}
	return nil
}

func TestMemoryEndpointsUseAuthenticatedScope(t *testing.T) {
	owner := memory.Scope{OwnerID: 31, CharacterID: 9}
	other := memory.Scope{OwnerID: 32, CharacterID: 9}
	const key = "favorite_color"
	store := &memoryHTTPStore{facts: map[memory.Scope][]memory.Fact{
		owner: {{Key: key, Value: "purple"}},
		other: {{Key: key, Value: "green"}},
	}}
	chat := usecase.NewChatUsecase(memoryConfigFixture{}, nil).
		WithMemory(usecase.NewMemoryService(store, nil))
	handler := New(chat, nil, nil)
	server := echo.New()
	Register(server.Group("/api"), handler)
	ctx := requestctx.WithUserID(context.Background(), owner.OwnerID)

	req := httptest.NewRequest(http.MethodGet, "/api/demo/memory/7", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "purple") || strings.Contains(rec.Body.String(), "green") {
		t.Fatalf("inspect status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/demo/memory/7/"+key, nil).WithContext(ctx)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(store.facts[owner]) != 0 || len(store.facts[other]) != 1 {
		t.Fatalf("delete status=%d facts=%+v", rec.Code, store.facts)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/demo/memory/7", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "green") {
		t.Fatalf("unauthenticated status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMemoryEndpointsRejectInvalidKey(t *testing.T) {
	chat := usecase.NewChatUsecase(memoryConfigFixture{}, nil).
		WithMemory(usecase.NewMemoryService(&memoryHTTPStore{}, nil))
	handler := New(chat, nil, nil)
	server := echo.New()
	Register(server.Group("/api"), handler)
	ctx := requestctx.WithUserID(context.Background(), 31)
	req := httptest.NewRequest(http.MethodDelete, "/api/demo/memory/7/unsupported", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("unknown preference key accepted")
	}

	_, err := chat.InspectMemory(context.Background(), 7)
	if parsed, ok := apperr.Parse(err); !ok || parsed.Code() != apperr.ErrUnauthorized {
		t.Fatalf("unauthenticated inspection: %v", err)
	}
}
