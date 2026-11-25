package agentdemohttp

import (
	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/platform/server/httpreq"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

// InspectMemory returns only the authenticated operator's preferences.
func (h *Handler) InspectMemory(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "device type")
	if err != nil {
		return err
	}
	snapshot, err := h.chat.InspectMemory(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpresp.OK(c, snapshot)
}

// DeleteMemoryFact removes one scoped preference, never conversation history.
func (h *Handler) DeleteMemoryFact(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "device type")
	if err != nil {
		return err
	}
	if err := h.chat.DeleteMemoryFact(c.Request().Context(), id, c.Param("key")); err != nil {
		return err
	}
	return httpresp.OK(c, map[string]string{"key": c.Param("key")})
}
