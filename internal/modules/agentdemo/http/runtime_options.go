package agentdemohttp

import (
	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/voiceconfig"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

type resourceOptions struct {
	Tools  []usecase.ResourceOption `json:"tools"`
	Skills []usecase.ResourceOption `json:"skills"`
	Voices []string                 `json:"voice_providers"`
}

// ResourceOptions lists only registered capabilities and approved keys.
func (h *Handler) ResourceOptions(c *echo.Context) error {
	tools, skills, err := h.chat.ResourceOptions(c.Request().Context())
	if err != nil {
		return err
	}
	return httpresp.OK(c, resourceOptions{Tools: tools, Skills: skills, Voices: voiceconfig.TTSProviders()})
}

// RuntimeStatus exposes configuration presence, never provider credentials.
func (h *Handler) RuntimeStatus(c *echo.Context) error {
	return httpresp.OK(c, h.chat.ConfigurationStatus(h.voice, voiceconfig.TTSProviders()))
}
