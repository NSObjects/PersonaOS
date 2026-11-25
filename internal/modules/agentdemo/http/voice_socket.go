package agentdemohttp

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

const liveASRTimeout = 45 * time.Second

type asrSocketEvent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// A browser WebSocket cannot send the usual CSRF header. Require an explicit
// same-origin host rather than accepting cross-site cookie-authenticated calls.
func liveASROrigin(r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && (origin.Scheme == "https" || origin.Scheme == "http") &&
		origin.Host != "" && strings.EqualFold(origin.Host, r.Host)
}

// LiveASR streams one microphone turn through the existing voice usecase.
func (h *VoiceHandler) LiveASR(c *echo.Context) error {
	format := c.QueryParam("format")
	if format != "ogg" && format != "webm" {
		return apperr.NewBadRequest("expected Ogg or WebM Opus")
	}
	if !liveASROrigin(c.Request()) {
		return apperr.NewBadRequest("invalid WebSocket origin")
	}
	upgrader := websocket.Upgrader{CheckOrigin: liveASROrigin}
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		// Upgrade writes its own HTTP error on failure.
		return nil
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			log.Debug().Err(closeErr).Msg("close live ASR socket")
		}
	}()
	conn.SetReadLimit(maxDemoOpusRequestBytes)
	if err := conn.SetReadDeadline(time.Now().Add(liveASRTimeout)); err != nil {
		return nil
	}
	if !h.voice.SupportsLiveOpus() {
		writeASREvent(conn, asrSocketEvent{Type: "batch"})
		return nil
	}
	if !writeASREvent(conn, asrSocketEvent{Type: "ready"}) {
		return nil
	}
	input := &socketAudioReader{conn: conn}
	text, err := h.voice.RecognizeLiveCapture(c.Request().Context(), input, format)
	if err != nil {
		log.Warn().Err(err).Str("format", format).Int("audio_bytes", input.total).Msg("demo live ASR failed")
		writeASREvent(conn, asrSocketEvent{Type: streamEventError})
		return nil
	}
	writeASREvent(conn, asrSocketEvent{Type: streamEventDone, Text: text})
	return nil
}

func writeASREvent(conn *websocket.Conn, event asrSocketEvent) bool {
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return false
	}
	if err := conn.WriteJSON(event); err != nil {
		log.Debug().Err(err).Msg("write live ASR event")
		return false
	}
	return true
}

type socketAudioReader struct {
	conn    *websocket.Conn
	pending []byte
	total   int
	ended   bool
}

func (r *socketAudioReader) Read(p []byte) (int, error) {
	if r.ended {
		return 0, io.EOF
	}
	if len(r.pending) == 0 {
		kind, data, err := r.conn.ReadMessage()
		if err != nil {
			return 0, err
		}
		if kind == websocket.TextMessage && string(data) == "end" {
			r.ended = true
			return 0, io.EOF
		}
		if kind != websocket.BinaryMessage || len(data) == 0 || len(data) > maxDemoOpusRequestBytes-r.total {
			return 0, errors.New("invalid live ASR audio message")
		}
		r.total += len(data)
		r.pending = data
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

// Interrupt a blocked input read when the streaming remuxer fails. Net.Conn
// deadlines can be changed concurrently with reads.
func (r *socketAudioReader) Close() error {
	return r.conn.UnderlyingConn().SetReadDeadline(time.Now())
}
