package cataloghttp

import (
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/audit/oprec"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpreq"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

// DraftHandler manages unpublished device-type capability selections.
type DraftHandler struct {
	uc    *usecase.CapabilityDraftUsecase
	audit *oprec.Recorder
}

// NewDraftHandler constructs the device-type draft HTTP adapter.
func NewDraftHandler(uc *usecase.CapabilityDraftUsecase, audit *oprec.Recorder) *DraftHandler {
	return &DraftHandler{uc: uc, audit: audit}
}

// RegisterDrafts mounts managed, authorization-protected draft endpoints.
func RegisterDrafts(group *echo.Group, h *DraftHandler) {
	group.GET("/device-types/:id/capability-draft", h.Get)
	group.PUT("/device-types/:id/capability-draft", h.Save)
}

// Get reads the current unpublished capability selection.
func (h *DraftHandler) Get(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "device type")
	if err != nil {
		return err
	}
	draft, err := h.uc.Get(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpresp.OK(c, draft)
}

// Save atomically updates the unpublished device-type capability selection.
func (h *DraftHandler) Save(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "device type")
	if err != nil {
		return err
	}
	var in usecase.CapabilityDraftInput
	if err := httpreq.BindAndValidate(c, &in); err != nil {
		return err
	}
	value, err := h.uc.Save(c.Request().Context(), id, in)
	err = h.audit.Record(c, "update", "device_type_capability_draft", strconv.FormatInt(id, 10), "updated device type capability draft", err)
	if err != nil {
		return err
	}
	return httpresp.OK(c, value)
}
