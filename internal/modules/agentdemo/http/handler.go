package agentdemohttp

import (
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpreq"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

// Handler exposes a small authenticated web Demo of the Eino agent.
type Handler struct {
	chat  *usecase.ChatUsecase
	voice *usecase.VoiceUsecase
}

// New constructs the Demo chat endpoint.
func New(chat *usecase.ChatUsecase, voice *usecase.VoiceUsecase) *Handler {
	return &Handler{chat: chat, voice: voice}
}

// Register adds a managed, RBAC-protected API route.
func Register(group *echo.Group, h *Handler) {
	group.POST("/demo/chat", h.Chat)
}

// Chat generates one real response using the device type's active config.
func (h *Handler) Chat(c *echo.Context) error {
	var in usecase.ChatInput
	if err := httpreq.BindAndValidate(c, &in); err != nil {
		return err
	}
	if strings.Contains(c.Request().Header.Get("Accept"), voiceStreamMIME) && c.QueryParam("trajectory") == "1" {
		return h.streamTrajectory(c, in)
	}
	if strings.Contains(c.Request().Header.Get("Accept"), voiceStreamMIME) {
		return h.streamReply(c, in)
	}
	reply, err := h.chat.Chat(c.Request().Context(), in)
	if err != nil {
		return err
	}
	return httpresp.OK(c, reply)
}
