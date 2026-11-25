package cataloghttp

import (
	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/platform/server/httpreq"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

// KnowledgeStatus reports the resource's actual indexed version.
func (h *ResourceHandler) KnowledgeStatus(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "capability resource")
	if err != nil {
		return err
	}
	status, err := h.uc.InspectKnowledge(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpresp.OK(c, status)
}

// KnowledgeSearch tests retrieval directly, without a model or device.
func (h *ResourceHandler) KnowledgeSearch(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "capability resource")
	if err != nil {
		return err
	}
	var in struct {
		Query string `json:"query"`
	}
	if err := httpreq.BindAndValidate(c, &in); err != nil {
		return err
	}
	result, err := h.uc.SearchKnowledge(c.Request().Context(), id, in.Query)
	if err != nil {
		return err
	}
	return httpresp.OK(c, result)
}
