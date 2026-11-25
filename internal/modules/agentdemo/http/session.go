package agentdemohttp

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

// SessionEvents provides independent, cursor-based long polling for observers.
// It carries no microphone or playback bytes and cannot cancel the sender.
func (h *Handler) SessionEvents(c *echo.Context) error {
	var after uint64
	if raw := c.QueryParam("after"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return apperr.NewBadRequest("invalid event cursor")
		}
		after = value
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 20*time.Second)
	defer cancel()
	batch, err := h.sessions.Read(ctx, c.Param("id"), after)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return httpresp.OK(c, batch)
}

// InterruptSession requests cancellation and acknowledges only after the old
// turn has released its session reservation.
func (h *Handler) InterruptSession(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()
	err := h.sessions.Interrupt(ctx, c.Param("id"))
	if errors.Is(err, context.DeadlineExceeded) {
		return apperr.NewConflict("conversation is still stopping")
	}
	if err != nil {
		return err
	}
	return httpresp.OK(c, map[string]bool{"stopped": true})
}
