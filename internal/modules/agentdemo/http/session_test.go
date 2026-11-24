package agentdemohttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
)

func TestSessionEventsEndpointChecksActorAndCursor(t *testing.T) {
	h := New(nil, nil)
	ctx := requestctx.WithRoleID(requestctx.WithUserID(context.Background(), 3), 8)
	in := usecase.ChatInput{DeviceTypeID: 4, SessionID: uuid.NewString(), Message: "session test"}
	turn, err := h.sessions.Begin(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = turn.Run(ctx, func(context.Context, usecase.ChatInput) (usecase.ChatOutput, error) {
		return usecase.ChatOutput{Reply: "observed reply"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.GET("/demo/sessions/:id/events", h.SessionEvents)
	path := "/demo/sessions/" + in.SessionID + "/events"
	request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "observed reply") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	// Invoke the adapter directly to assert failure before any response is written.
	request = httptest.NewRequest(http.MethodGet, path+"?after=invalid", nil).WithContext(ctx)
	c := e.NewContext(request, httptest.NewRecorder())
	if err := h.SessionEvents(c); err == nil {
		t.Fatal("invalid cursor accepted")
	}
	request = httptest.NewRequest(http.MethodGet, path, nil).WithContext(requestctx.WithUserID(ctx, 17))
	response = httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code == http.StatusOK || strings.Contains(response.Body.String(), "observed reply") {
		t.Fatalf("cross-owner response=%d body=%s", response.Code, response.Body.String())
	}
}
