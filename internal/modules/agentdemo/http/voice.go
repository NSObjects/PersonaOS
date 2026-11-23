package agentdemohttp

import (
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpreq"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

const maxDemoPCMRequestBytes = 16000 * 2 * 15

// VoiceHandler adapts authenticated browser audio to go-speech, without
// introducing a second Agent or storing raw microphone recordings.
type VoiceHandler struct{ voice *usecase.VoiceUsecase }

// NewVoiceHandler exposes the existing speech SDK through managed routes.
func NewVoiceHandler(voice *usecase.VoiceUsecase) *VoiceHandler {
	return &VoiceHandler{voice: voice}
}

// RegisterVoice adds two authenticated, RBAC-managed Demo endpoints.
func RegisterVoice(group *echo.Group, h *VoiceHandler) {
	group.POST("/demo/asr", h.ASR)
	group.POST("/demo/tts", h.TTS)
}

// ASR accepts exactly raw 16 kHz mono PCM16LE bytes, not WebM, WAV, or JSON.
func (h *VoiceHandler) ASR(c *echo.Context) error {
	mediaType, _, err := mime.ParseMediaType(c.Request().Header.Get(echo.HeaderContentType))
	if err != nil || mediaType != "application/octet-stream" {
		return apperr.NewBadRequest("expected raw 16 kHz PCM16LE audio")
	}
	if c.Request().ContentLength > maxDemoPCMRequestBytes {
		return apperr.NewBadRequest("recording is too long")
	}
	data, err := io.ReadAll(io.LimitReader(c.Request().Body, maxDemoPCMRequestBytes+1))
	if err != nil {
		return apperr.WrapBadRequest(err, "read microphone audio")
	}
	text, err := h.voice.Recognize(c.Request().Context(), data)
	if err != nil {
		return err
	}
	return httpresp.OK(c, map[string]string{"text": text})
}

// TTS always looks up the applied Voice of the requested device type.
func (h *VoiceHandler) TTS(c *echo.Context) error {
	var in struct {
		DeviceTypeID int64  `json:"device_type_id"`
		Text         string `json:"text"`
	}
	if err := httpreq.BindAndValidate(c, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Text) == "" {
		return apperr.NewBadRequest("speech text is required")
	}
	data, err := h.voice.Synthesize(c.Request().Context(), in.DeviceTypeID, in.Text)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.Blob(http.StatusOK, "audio/mpeg", data)
}
