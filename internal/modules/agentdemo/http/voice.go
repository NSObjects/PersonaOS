package agentdemohttp

import (
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpreq"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

const maxDemoOpusRequestBytes = 1 << 20

// VoiceHandler adapts authenticated browser audio to go-speech, without
// introducing a second Agent or storing raw microphone recordings.
type VoiceHandler struct{ voice *usecase.VoiceUsecase }

// NewVoiceHandler exposes the existing speech SDK through managed routes.
func NewVoiceHandler(voice *usecase.VoiceUsecase) *VoiceHandler {
	return &VoiceHandler{voice: voice}
}

// RegisterVoice adds authenticated, RBAC-managed Demo endpoints.
func RegisterVoice(group *echo.Group, h *VoiceHandler) {
	group.POST("/demo/asr", h.ASR)
	group.GET("/demo/asr/live", h.LiveASR)
	group.POST("/demo/tts", h.TTS)
}

// ASR accepts browser Ogg Opus or WebM/Opus, never a disguised PCM upload.
func (h *VoiceHandler) ASR(c *echo.Context) error {
	mediaType, params, err := mime.ParseMediaType(c.Request().Header.Get(echo.HeaderContentType))
	if err != nil || (mediaType != "audio/ogg" && mediaType != "audio/webm") ||
		!strings.EqualFold(params["codecs"], "opus") {
		return apperr.NewBadRequest("expected audio/ogg or audio/webm with Opus codec")
	}
	if c.Request().ContentLength > maxDemoOpusRequestBytes {
		return apperr.NewBadRequest("Opus recording is too large")
	}
	if c.Request().ContentLength < 0 && mediaType == "audio/ogg" && h.voice.SupportsLiveOpus() {
		return h.liveOpus(c)
	}
	data, err := io.ReadAll(io.LimitReader(c.Request().Body, maxDemoOpusRequestBytes+1))
	if err != nil {
		return apperr.WrapBadRequest(err, "read microphone audio")
	}
	text, err := h.voice.Recognize(c.Request().Context(), data, mediaType)
	if err != nil {
		return err
	}
	return httpresp.OK(c, map[string]string{"text": text})
}

// A chunked Ogg request is read as it arrives. A bounded read deadline also
// prevents a stalled client from retaining a paid ASR session indefinitely.
func (h *VoiceHandler) liveOpus(c *echo.Context) error {
	controller := http.NewResponseController(c.Response())
	if err := controller.SetReadDeadline(time.Now().Add(45 * time.Second)); err != nil {
		return err
	}
	defer func() {
		if err := controller.SetReadDeadline(time.Time{}); err != nil {
			log.Warn().Err(err).Msg("reset live ASR read deadline")
		}
	}()
	text, err := h.voice.RecognizeLiveOpus(c.Request().Context(), c.Request().Body)
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
	preferOpus := strings.Contains(c.Request().Header.Get("Accept"), "audio/ogg")
	data, err := h.voice.Synthesize(c.Request().Context(), in.DeviceTypeID, in.Text, preferOpus)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.Blob(http.StatusOK, usecase.AudioMIME(data.Format), data.Data)
}
