package agentdemohttp

import (
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpreq"
	"github.com/NSObjects/PersonaOS/internal/platform/server/httpresp"
)

// Handler exposes a small authenticated web Demo of the Eino agent.
type Handler struct {
	chat     *usecase.ChatUsecase
	voice    *usecase.VoiceUsecase
	sessions *usecase.ConversationSessions
}

// New constructs the Demo chat endpoint.
func New(chat *usecase.ChatUsecase, voice *usecase.VoiceUsecase, sessions *usecase.ConversationSessions) *Handler {
	return &Handler{chat: chat, voice: voice, sessions: sessions}
}

// Register adds a managed, RBAC-protected API route.
func Register(group *echo.Group, h *Handler) {
	group.POST("/demo/chat", h.withVoice(func(next *Handler, c *echo.Context) error { return next.Chat(c) }))
	group.GET("/demo/sessions/:id/events", h.SessionEvents)
	group.POST("/demo/sessions/:id/interrupt", h.InterruptSession)
	group.GET("/demo/resource-options", h.withVoice(func(next *Handler, c *echo.Context) error { return next.ResourceOptions(c) }))
	group.GET("/demo/status", h.withVoice(func(next *Handler, c *echo.Context) error { return next.RuntimeStatus(c) }))
	group.GET("/demo/memory/:id", h.InspectMemory)
	group.DELETE("/demo/memory/:id/:key", h.DeleteMemoryFact)
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

// withVoice captures one speech configuration for the whole reply or status
// request without touching active conversation sessions.
func (h *Handler) withVoice(run func(*Handler, *echo.Context) error) echo.HandlerFunc {
	return func(c *echo.Context) error {
		voice, err := h.voice.Snapshot(c.Request().Context())
		if err != nil {
			return apperr.New(apperr.ErrInternalServer, "cannot load speech configuration")
		}
		next := *h
		next.voice = voice
		next.chat, err = h.chat.Snapshot(c.Request().Context())
		if err != nil {
			return apperr.New(apperr.ErrInternalServer, "cannot load Agent configuration")
		}
		return run(&next, c)
	}
}
