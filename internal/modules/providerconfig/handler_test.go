package providerconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
)

func TestProviderManagementRequiresRootSession(t *testing.T) {
	h := NewHandler(nil, func(_ context.Context, role int64) (bool, error) {
		return role == 1, nil
	}, nil)
	for _, tc := range []struct {
		name string
		info requestctx.Info
		code int
	}{
		{name: "anonymous", code: apperr.ErrUnauthorized},
		{name: "root API token", info: requestctx.Info{UserID: 1, RoleID: 1}, code: apperr.ErrUnauthorized},
		{name: "ordinary role", info: requestctx.Info{UserID: 2, RoleID: 2, LoginSessionID: 3}, code: apperr.ErrPermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil).WithContext(requestctx.WithInfo(context.Background(), tc.info)), httptest.NewRecorder())
			err := h.List(c)
			appErr, ok := apperr.Parse(err)
			if !ok || appErr.Code() != tc.code {
				t.Fatalf("unexpected authorization result: %v", err)
			}
		})
	}
}

func TestProviderManagementRejectsMalformedChanges(t *testing.T) {
	h := NewHandler(nil, func(context.Context, int64) (bool, error) { return true, nil }, nil)
	e := echo.New()
	group := e.Group("/api")
	Register(group, h)
	// The nil store is intentional. Invalid input must stop before persistence.
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		appErr, ok := apperr.Parse(err)
		if !ok || appErr.Code() != apperr.ErrBadRequest {
			t.Errorf("unexpected validation result: %v", err)
		}
		if writeErr := c.NoContent(http.StatusBadRequest); writeErr != nil {
			t.Error(writeErr)
		}
	}
	for _, input := range []string{`{"api_key":null}`, `{"api_key":123}`, `{}`, `{"api_key":"a"}{}`} {
		req := httptest.NewRequest(http.MethodPatch, "/api/system/provider-configs/aliyun", strings.NewReader(input))
		req = req.WithContext(requestctx.WithInfo(req.Context(), requestctx.Info{UserID: 1, RoleID: 1, LoginSessionID: 1}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("invalid input returned status %d", rec.Code)
		}
	}
}
