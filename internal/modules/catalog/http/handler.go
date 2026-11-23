// Package cataloghttp adapts catalog administration requests to usecases.
package cataloghttp

import (
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/audit/oprec"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpreq"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

// Handler provides authenticated catalog management routes.
type Handler struct {
	uc    *usecase.Usecase
	audit *oprec.Recorder
}

// New creates catalog HTTP handlers.
func New(uc *usecase.Usecase, audit *oprec.Recorder) *Handler {
	return &Handler{uc: uc, audit: audit}
}

// Register mounts all catalog endpoints on the authorized /api group.
func Register(group *echo.Group, h *Handler) {
	group.GET("/characters", h.ListCharacters)
	group.POST("/characters", h.CreateCharacter)
	group.GET("/characters/:id", h.FindCharacter)
	group.PATCH("/characters/:id", h.UpdateCharacter)
	group.DELETE("/characters/:id", h.DeleteCharacter)
	group.GET("/device-types", h.ListDeviceTypes)
	group.POST("/device-types", h.CreateDeviceType)
	group.GET("/device-types/:id", h.FindDeviceType)
	group.PATCH("/device-types/:id", h.UpdateDeviceType)
	group.DELETE("/device-types/:id", h.DeleteDeviceType)
}

// ListCharacters lists reusable persona records.
func (h *Handler) ListCharacters(c *echo.Context) error {
	list, err := h.uc.ListCharacters(c.Request().Context())
	if err != nil {
		return err
	}
	return httpresp.OK(c, list)
}

// FindCharacter returns one persona.
func (h *Handler) FindCharacter(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "character")
	if err != nil {
		return err
	}
	value, err := h.uc.FindCharacter(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpresp.OK(c, value)
}

// CreateCharacter saves one persona.
func (h *Handler) CreateCharacter(c *echo.Context) error {
	var in usecase.CharacterInput
	if err := httpreq.BindAndValidate(c, &in); err != nil {
		return err
	}
	value, err := h.uc.CreateCharacter(c.Request().Context(), in)
	err = h.audit.Record(c, "create", "character", strconv.FormatInt(value.ID, 10), "created character", err)
	if err != nil {
		return err
	}
	return httpresp.Created(c, value)
}

// UpdateCharacter changes one persona without altering its code.
func (h *Handler) UpdateCharacter(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "character")
	if err != nil {
		return err
	}
	var in usecase.CharacterInput
	if err := httpreq.BindAndValidate(c, &in); err != nil {
		return err
	}
	value, err := h.uc.UpdateCharacter(c.Request().Context(), id, in)
	err = h.audit.Record(c, "update", "character", strconv.FormatInt(id, 10), "updated character", err)
	if err != nil {
		return err
	}
	return httpresp.OK(c, value)
}

// DeleteCharacter removes an unreferenced persona.
func (h *Handler) DeleteCharacter(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "character")
	if err != nil {
		return err
	}
	err = h.uc.DeleteCharacter(c.Request().Context(), id)
	err = h.audit.Record(c, "delete", "character", strconv.FormatInt(id, 10), "deleted character", err)
	if err != nil {
		return err
	}
	return httpresp.OK(c, map[string]int64{"id": id})
}

// ListDeviceTypes lists product models.
func (h *Handler) ListDeviceTypes(c *echo.Context) error {
	list, err := h.uc.ListDeviceTypes(c.Request().Context())
	if err != nil {
		return err
	}
	return httpresp.OK(c, list)
}

// FindDeviceType returns one product model.
func (h *Handler) FindDeviceType(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "device type")
	if err != nil {
		return err
	}
	value, err := h.uc.FindDeviceType(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpresp.OK(c, value)
}

// CreateDeviceType saves a new model referencing an existing character.
func (h *Handler) CreateDeviceType(c *echo.Context) error {
	var in usecase.DeviceTypeInput
	if err := httpreq.BindAndValidate(c, &in); err != nil {
		return err
	}
	value, err := h.uc.CreateDeviceType(c.Request().Context(), in)
	err = h.audit.Record(c, "create", "device_type", strconv.FormatInt(value.ID, 10), "created device type", err)
	if err != nil {
		return err
	}
	return httpresp.Created(c, value)
}

// UpdateDeviceType updates a model's character and capabilities.
func (h *Handler) UpdateDeviceType(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "device type")
	if err != nil {
		return err
	}
	var in usecase.DeviceTypeInput
	if err := httpreq.BindAndValidate(c, &in); err != nil {
		return err
	}
	value, err := h.uc.UpdateDeviceType(c.Request().Context(), id, in)
	err = h.audit.Record(c, "update", "device_type", strconv.FormatInt(id, 10), "updated device type", err)
	if err != nil {
		return err
	}
	return httpresp.OK(c, value)
}

// DeleteDeviceType removes one product model.
func (h *Handler) DeleteDeviceType(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "device type")
	if err != nil {
		return err
	}
	err = h.uc.DeleteDeviceType(c.Request().Context(), id)
	err = h.audit.Record(c, "delete", "device_type", strconv.FormatInt(id, 10), "deleted device type", err)
	if err != nil {
		return err
	}
	return httpresp.OK(c, map[string]int64{"id": id})
}
