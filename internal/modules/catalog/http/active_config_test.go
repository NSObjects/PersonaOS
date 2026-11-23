package cataloghttp_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/NSObjects/PersonaOS/internal/modules/audit/oprec"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	cataloghttp "github.com/NSObjects/PersonaOS/internal/modules/catalog/http"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

type activeConfigStub struct {
	applied domain.PublishedDeviceConfig
	actorID int64
}

func (s *activeConfigStub) Apply(_ context.Context, deviceTypeID, actorID int64) (domain.PublishedDeviceConfig, error) {
	s.actorID = actorID
	s.applied = domain.PublishedDeviceConfig{
		Version: 1, DeviceType: domain.DeviceType{ID: deviceTypeID},
	}
	return s.applied, nil
}

func (s *activeConfigStub) FindActive(_ context.Context, _ int64) (domain.PublishedDeviceConfig, error) {
	if s.applied.Version == 0 {
		return domain.PublishedDeviceConfig{}, apperr.NewNotFound("active device configuration")
	}
	return s.applied, nil
}

func TestActiveConfigHTTPApplyReadAndAudit(t *testing.T) {
	e := testEcho()
	store := &activeConfigStub{}
	audit := &fakeAudit{}
	cataloghttp.RegisterActiveConfigs(e.Group("/api"), cataloghttp.NewActiveConfigHandler(
		usecase.NewActiveConfigUsecase(store), oprec.New(audit),
	))
	path := "/api/device-types/7/active-config"
	if got := call(e, http.MethodGet, path, ""); got.Code != http.StatusNotFound {
		t.Fatalf("GET before apply: %d body=%s", got.Code, got.Body.String())
	}
	if got := call(e, http.MethodPut, path, ""); got.Code != http.StatusOK {
		t.Fatalf("PUT apply: %d body=%s", got.Code, got.Body.String())
	}
	if store.actorID != 1 || len(audit.records) != 1 {
		t.Fatalf("apply actor=%d audit=%d, want actor=1 audit=1", store.actorID, len(audit.records))
	}
	got := call(e, http.MethodGet, path, "")
	if got.Code != http.StatusOK || !bytes.Contains(got.Body.Bytes(), []byte("\"version\":1")) {
		t.Fatalf("GET active: %d body=%s", got.Code, got.Body.String())
	}
	if got := call(e, http.MethodPut, "/api/device-types/invalid/active-config", ""); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid device type status=%d, want 400", got.Code)
	}
}
