package cataloghttp

import (
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/audit/oprec"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpreq"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

// ActiveConfigHandler confirms a Demo configuration on explicit admin action.
type ActiveConfigHandler struct {
	uc    *usecase.ActiveConfigUsecase
	audit *oprec.Recorder
}

// NewActiveConfigHandler constructs the managed Demo configuration endpoints.
func NewActiveConfigHandler(uc *usecase.ActiveConfigUsecase, audit *oprec.Recorder) *ActiveConfigHandler {
	return &ActiveConfigHandler{uc: uc, audit: audit}
}

// RegisterActiveConfigs mounts authenticated, authorized admin routes.
func RegisterActiveConfigs(group *echo.Group, h *ActiveConfigHandler) {
	group.GET("/device-types/:id/active-config", h.Get)
	group.PUT("/device-types/:id/active-config", h.Apply)
}

// Get loads only the confirmed snapshot; a missing one is HTTP 404.
func (h *ActiveConfigHandler) Get(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "device type")
	if err != nil {
		return err
	}
	result, err := h.uc.Get(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpresp.OK(c, result)
}

// Apply freezes the current draft for one device type, with audit.
func (h *ActiveConfigHandler) Apply(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "device type")
	if err != nil {
		return err
	}
	actorID := requestctx.GetUserID(c.Request().Context())
	value, err := h.uc.Apply(c.Request().Context(), id, actorID)
	err = h.audit.Record(c, "apply", "device_type_active_config", strconv.FormatInt(id, 10), "applied Demo device configuration", err)
	if err != nil {
		return err
	}
	return httpresp.OK(c, value)
}
