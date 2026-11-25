package cataloghttp

import (
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/audit/oprec"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpreq"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

// ResourceHandler manages reusable capability resource metadata.
type ResourceHandler struct {
	uc    *usecase.ResourceUsecase
	audit *oprec.Recorder
}

// NewResourceHandler constructs the resource HTTP adapter.
func NewResourceHandler(uc *usecase.ResourceUsecase, audit *oprec.Recorder) *ResourceHandler {
	return &ResourceHandler{uc: uc, audit: audit}
}

// RegisterResources mounts managed capability resource endpoints.
func RegisterResources(group *echo.Group, h *ResourceHandler) {
	group.GET("/capability-resources", h.List)
	group.POST("/capability-resources", h.withProviders((*ResourceHandler).Create))
	group.POST("/capability-resources/voice-clones", h.withProviders((*ResourceHandler).CreateVoiceClone))
	group.POST("/capability-resources/:id/voice-clone/train", h.withProviders((*ResourceHandler).TrainVoiceClone))
	group.GET("/capability-resources/:id/voice-clone/status", h.withProviders((*ResourceHandler).VoiceCloneStatus))
	group.POST("/capability-resources/:id/voice-clone/enable", h.withProviders((*ResourceHandler).EnableVoiceClone))
	group.GET("/capability-resources/:id", h.Find)
	group.GET("/capability-resources/:id/knowledge-status", h.withProviders((*ResourceHandler).KnowledgeStatus))
	group.POST("/capability-resources/:id/knowledge-search", h.withProviders((*ResourceHandler).KnowledgeSearch))
	group.PATCH("/capability-resources/:id", h.withProviders((*ResourceHandler).Update))
	group.DELETE("/capability-resources/:id", h.Delete)
}

// List returns resources of exactly one requested kind.
func (h *ResourceHandler) List(c *echo.Context) error {
	list, err := h.uc.List(c.Request().Context(), domain.ResourceKind(c.QueryParam("kind")))
	if err != nil {
		return err
	}
	return httpresp.OK(c, list)
}

// Find reads one reusable resource.
func (h *ResourceHandler) Find(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "capability resource")
	if err != nil {
		return err
	}
	value, err := h.uc.Find(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return httpresp.OK(c, value)
}

// Create registers resource metadata without executing it.
func (h *ResourceHandler) Create(c *echo.Context) error {
	var in usecase.ResourceInput
	if err := httpreq.BindAndValidate(c, &in); err != nil {
		return err
	}
	value, err := h.uc.Create(c.Request().Context(), in)
	err = h.audit.Record(c, "create", "capability_resource", strconv.FormatInt(value.ID, 10), "created capability resource", err)
	if err != nil {
		return err
	}
	return httpresp.Created(c, value)
}

// Update edits resource metadata without changing its kind or code.
func (h *ResourceHandler) Update(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "capability resource")
	if err != nil {
		return err
	}
	var in usecase.ResourceInput
	if err := httpreq.BindAndValidate(c, &in); err != nil {
		return err
	}
	value, err := h.uc.Update(c.Request().Context(), id, in)
	err = h.audit.Record(c, "update", "capability_resource", strconv.FormatInt(id, 10), "updated capability resource", err)
	if err != nil {
		return err
	}
	return httpresp.OK(c, value)
}

// Delete removes an unreferenced resource; database FKs protect bound entries.
func (h *ResourceHandler) Delete(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "capability resource")
	if err != nil {
		return err
	}
	err = h.uc.Delete(c.Request().Context(), id)
	err = h.audit.Record(c, "delete", "capability_resource", strconv.FormatInt(id, 10), "deleted capability resource", err)
	if err != nil {
		return err
	}
	return httpresp.OK(c, map[string]int64{"id": id})
}

// withProviders binds the current configuration once for the operation.
func (h *ResourceHandler) withProviders(run func(*ResourceHandler, *echo.Context) error) echo.HandlerFunc {
	return func(c *echo.Context) error {
		uc, err := h.uc.Snapshot(c.Request().Context())
		if err != nil {
			return apperr.New(apperr.ErrInternalServer, "cannot load provider configuration")
		}
		next := *h
		next.uc = uc
		return run(&next, c)
	}
}
