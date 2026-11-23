package cataloghttp_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/NSObjects/PersonaOS/internal/modules/audit/oprec"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	cataloghttp "github.com/NSObjects/PersonaOS/internal/modules/catalog/http"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
)

type draftStoreStub struct{ value domain.DeviceTypeCapabilities }

func (s *draftStoreStub) Find(_ context.Context, id int64) (domain.DeviceTypeCapabilities, error) {
	result := s.value.Clone()
	result.DeviceTypeID = id
	return result, nil
}

func (s *draftStoreStub) Save(_ context.Context, value domain.DeviceTypeCapabilities) (domain.DeviceTypeCapabilities, error) {
	s.value = value.Clone()
	return s.value.Clone(), nil
}

func TestCapabilityDraftHTTPReadWriteAndAudit(t *testing.T) {
	e := testEcho()
	audit := &fakeAudit{}
	cataloghttp.RegisterDrafts(e.Group("/api"), cataloghttp.NewDraftHandler(usecase.NewCapabilityDraftUsecase(&draftStoreStub{}), oprec.New(audit)))
	res := call(e, http.MethodPut, "/api/device-types/9/capability-draft", `{"tool_ids":[4,2]}`)
	if res.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", res.Code, res.Body.String())
	}
	if len(audit.records) != 1 {
		t.Fatalf("audit records=%d, want 1", len(audit.records))
	}
	res = call(e, http.MethodGet, "/api/device-types/9/capability-draft", "")
	if res.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", res.Code, res.Body.String())
	}
	res = call(e, http.MethodPut, "/api/device-types/9/capability-draft", `{"tool_ids":[4,4]}`)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("invalid draft status=%d body=%s", res.Code, res.Body.String())
	}
}
