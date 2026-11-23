package cataloghttp_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/audit/oprec"
	auditusecase "github.com/NSObjects/PersonaOS/internal/modules/audit/usecase"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/adapters/memory"
	cataloghttp "github.com/NSObjects/PersonaOS/internal/modules/catalog/http"
	catalogusecase "github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
	"github.com/NSObjects/PersonaOS/internal/platform/server/middlewares"
)

type fakeAudit struct{ records []auditusecase.OperationInput }

func (f *fakeAudit) RecordOperation(_ context.Context, v auditusecase.OperationInput) (auditusecase.OperationLog, error) {
	f.records = append(f.records, v)
	return auditusecase.OperationLog{}, nil
}
func testEcho() *echo.Echo {
	e := echo.New()
	e.Validator = &middlewares.Validator{Validator: validator.New()}
	e.HTTPErrorHandler = middlewares.ErrorHandlerWithRecorder(nil)
	cataloghttp.Register(e.Group("/api"), cataloghttp.New(catalogusecase.New(memory.New()), oprec.New(&fakeAudit{})))
	return e
}
func call(e *echo.Echo, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	req = req.WithContext(requestctx.WithUserID(req.Context(), 1))
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, req)
	return recorder
}
func TestCreateAndListCharacters(t *testing.T) {
	e := testEcho()
	r := call(e, http.MethodPost, "/api/characters", `{"code":"shinchan","name":"蜡笔小新","persona":"有趣活泼"}`)
	if r.Code != http.StatusCreated {
		t.Fatalf("create status=%d, want 201, body=%s", r.Code, r.Body.String())
	}
	r = call(e, http.MethodGet, "/api/characters", "")
	if r.Code != http.StatusOK || !bytes.Contains(r.Body.Bytes(), []byte("shinchan")) {
		t.Fatalf("list status=%d body=%s, want character in list", r.Code, r.Body.String())
	}
}
func TestInvalidCharacterAndUnknownDeviceRelation(t *testing.T) {
	e := testEcho()
	r := call(e, http.MethodPost, "/api/characters", `{"code":"bad","name":"缺少人设"}`)
	if r.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", r.Code, r.Body.String())
	}
	r = call(e, http.MethodPost, "/api/device-types", `{"code":"A3","name":"基础版","character_id":999}`)
	if r.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s, want 404", r.Code, r.Body.String())
	}
}
