package cataloghttp

import (
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpreq"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

// CreateVoiceClone reserves a new voice resource without any vendor charge.
func (h *ResourceHandler) CreateVoiceClone(c *echo.Context) error {
	var in struct {
		Name string `json:"name"`
	}
	if err := httpreq.BindAndValidate(c, &in); err != nil {
		return err
	}
	resource, err := h.uc.CreateClone(c.Request().Context(), in.Name)
	err = h.audit.Record(c, "create", "capability_resource", strconv.FormatInt(resource.ID, 10), "reserved clone voice", err)
	if err != nil {
		return err
	}
	return httpresp.Created(c, resource)
}

// TrainVoiceClone submits bounded multipart audio directly, without retaining it.
func (h *ResourceHandler) TrainVoiceClone(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "voice clone")
	if err != nil {
		return err
	}
	const maxUpload = usecase.MaxCloneSampleBytes + 64*1024
	req := c.Request()
	req.Body = http.MaxBytesReader(c.Response(), req.Body, maxUpload)
	if err := req.ParseMultipartForm(maxUpload); err != nil {
		return apperr.NewBadRequest("invalid or oversized reference audio upload")
	}
	defer func() {
		if cleanupErr := req.MultipartForm.RemoveAll(); cleanupErr != nil {
			log.Warn().Err(cleanupErr).Msg("remove clone upload")
		}
	}()
	file, header, err := req.FormFile("audio")
	if err != nil {
		return apperr.NewBadRequest("reference audio required")
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("close clone upload")
		}
	}()
	data, err := io.ReadAll(io.LimitReader(file, usecase.MaxCloneSampleBytes+1))
	if err != nil {
		return apperr.WrapBadRequest(err, "read reference audio")
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Filename)), ".")
	status, err := h.uc.TrainClone(req.Context(), id, format, req.FormValue("transcript"), data, req.FormValue("authorized") == "true")
	err = h.audit.Record(c, "update", "capability_resource", strconv.FormatInt(id, 10), "train clone voice", err)
	if err != nil {
		return err
	}
	return httpresp.OK(c, status)
}

// VoiceCloneStatus reads training progress without calling synthesis.
func (h *ResourceHandler) VoiceCloneStatus(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "voice clone")
	if err != nil {
		return err
	}
	status, err := h.uc.CloneStatus(c.Request().Context(), id)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return httpresp.OK(c, status)
}

// EnableVoiceClone makes a trained resource available for device binding.
func (h *ResourceHandler) EnableVoiceClone(c *echo.Context) error {
	id, err := httpreq.PathID(c, "id", "voice clone")
	if err != nil {
		return err
	}
	var in struct {
		Confirmed bool `json:"confirmed"`
	}
	if err := httpreq.BindAndValidate(c, &in); err != nil {
		return err
	}
	resource, err := h.uc.EnableClone(c.Request().Context(), id, in.Confirmed)
	err = h.audit.Record(c, "update", "capability_resource", strconv.FormatInt(id, 10), "enable trained clone voice", err)
	if err != nil {
		return err
	}
	return httpresp.OK(c, resource)
}
