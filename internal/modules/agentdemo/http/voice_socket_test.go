package agentdemohttp

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
)

func TestLiveASRRejectsCrossOrigin(t *testing.T) {
	request := httptest.NewRequest("GET", "http://demo.local/api/demo/asr/live?format=ogg", nil)
	request.Header.Set("Origin", "http://other.local")
	if liveASROrigin(request) {
		t.Fatal("cross-origin WebSocket accepted")
	}
}

func TestLiveASRReportsBatchBeforeCapture(t *testing.T) {
	e := echo.New()
	h := NewVoiceHandler(nil)
	e.GET("/api/demo/asr/live", h.LiveASR)
	server := httptest.NewServer(e)
	defer server.Close()
	header := map[string][]string{"Origin": {server.URL}}
	conn, response, err := websocket.DefaultDialer.Dial(strings.Replace(server.URL, "http", "ws", 1)+"/api/demo/asr/live?format=ogg", header)
	if err != nil {
		t.Fatalf("dial live ASR: %v, response=%v", err, response)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()
	var event asrSocketEvent
	if err := conn.ReadJSON(&event); err != nil || event.Type != "batch" {
		t.Fatalf("event=%+v err=%v", event, err)
	}
}
