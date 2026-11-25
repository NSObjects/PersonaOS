package providerconfig

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/audit/oprec"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

// Handler exposes only redacted views and requires a root login session,
// including when an ordinary role has been granted the route.
type Handler struct {
	store *Store
	root  func(context.Context, int64) (bool, error)
	audit *oprec.Recorder
}

// NewHandler binds the credential store and the platform role policy.
func NewHandler(store *Store, root func(context.Context, int64) (bool, error), audit *oprec.Recorder) *Handler {
	return &Handler{store: store, root: root, audit: audit}
}

// Register mounts management-only provider configuration routes.
func Register(group *echo.Group, handler *Handler) {
	group.GET("/system/provider-configs", handler.List)
	group.PATCH("/system/provider-configs/:service", handler.Update)
}

func (h *Handler) authorize(c *echo.Context) error {
	ctx := c.Request().Context()
	if requestctx.GetUserID(ctx) <= 0 || requestctx.GetLoginSessionID(ctx) <= 0 || requestctx.GetRoleID(ctx) <= 0 {
		return apperr.NewUnauthorized()
	}
	root, err := h.root(ctx, requestctx.GetRoleID(ctx))
	if err != nil {
		return apperr.New(apperr.ErrInternalServer, "cannot verify provider administrator")
	}
	if !root {
		return apperr.NewPermissionDenied("provider configuration", "manage")
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return nil
}

// List returns no credential material, including for the root administrator.
func (h *Handler) List(c *echo.Context) error {
	if err := h.authorize(c); err != nil {
		return err
	}
	views, err := h.store.List(c.Request().Context())
	if err != nil {
		return apperr.New(apperr.ErrInternalServer, "cannot read provider configuration")
	}
	return httpresp.OK(c, views)
}

// Update accepts only explicit string fields. Error responses and operation
// audit records deliberately omit the request body and credential values.
func (h *Handler) Update(c *echo.Context) error {
	if err := h.authorize(c); err != nil {
		return err
	}
	id := c.Param("service")
	if _, ok := Lookup(id); !ok {
		return apperr.NewBadRequest("unsupported provider service")
	}
	var input map[string]*string
	body := http.MaxBytesReader(c.Response(), c.Request().Body, 32<<10)
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&input); err != nil || len(input) == 0 {
		return apperr.NewBadRequest("invalid provider configuration changes")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return apperr.NewBadRequest("invalid provider configuration changes")
	}
	changes := make(Values, len(input))
	for key, value := range input {
		if value == nil {
			return apperr.NewBadRequest("provider configuration fields must be strings")
		}
		changes[key] = *value
	}
	view, err := h.store.Update(c.Request().Context(), id, changes)
	if err != nil {
		err = apperr.NewBadRequest("provider configuration could not be saved; check required fields")
	}
	err = h.audit.Record(c, "update", "provider_config", id, "updated provider configuration", err)
	if err != nil {
		return err
	}
	return httpresp.OK(c, view)
}
