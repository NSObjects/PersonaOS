package agentdemohttp

import (
	"context"
	"encoding/binary"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NSObjects/go-speech/asr"
	"github.com/NSObjects/go-speech/audio"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
)

const socketTestPath = "/api/demo/asr/live"

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
	e.GET(socketTestPath, h.LiveASR)
	server := httptest.NewServer(e)
	defer server.Close()
	header := map[string][]string{"Origin": {server.URL}}
	conn, response, err := websocket.DefaultDialer.Dial(strings.Replace(server.URL, "http", "ws", 1)+socketTestPath+"?format=ogg", header)
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

type socketRecognizer struct {
	stream *socketRecognitionStream
}

func (s socketRecognizer) OpenStream(context.Context, asr.Config) (asr.Stream, error) {
	return s.stream, nil
}

type socketRecognitionStream struct {
	sent  chan struct{}
	end   chan struct{}
	final bool
}

func (s *socketRecognitionStream) Send(ctx context.Context, _ []byte) error {
	select {
	case s.sent <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *socketRecognitionStream) CloseSend(context.Context) error {
	close(s.end)
	return nil
}

func (s *socketRecognitionStream) Recv(ctx context.Context) (asr.Event, error) {
	select {
	case <-s.end:
		if !s.final {
			s.final = true
			return asr.Event{Type: asr.EventFinal, Text: "你好呀"}, nil
		}
		return asr.Event{}, io.EOF
	case <-ctx.Done():
		return asr.Event{}, ctx.Err()
	}
}

func (s *socketRecognitionStream) Close() error { return nil }

func socketOggPage(data []byte, granule uint64) []byte {
	page := make([]byte, 28+len(data))
	copy(page, "OggS")
	binary.LittleEndian.PutUint64(page[6:14], granule)
	page[26], page[27] = 1, byte(len(data))
	copy(page[28:], data)
	return page
}

func TestLiveASRForwardsBeforeEnd(t *testing.T) {
	stream := &socketRecognitionStream{sent: make(chan struct{}), end: make(chan struct{})}
	voice := usecase.NewVoiceUsecase(nil, socketRecognizer{stream: stream}, nil,
		audio.Format{Encoding: audio.EncodingOpus, Channels: 1})
	e := echo.New()
	e.GET(socketTestPath, NewVoiceHandler(voice).LiveASR)
	server := httptest.NewServer(e)
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(server.URL, "http", "ws", 1)+socketTestPath+"?format=ogg",
		map[string][]string{"Origin": {server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var event asrSocketEvent
	if err := conn.ReadJSON(&event); err != nil || event.Type != "ready" {
		t.Fatalf("ready event=%+v err=%v", event, err)
	}
	header := make([]byte, 19)
	copy(header, "OpusHead")
	header[8], header[9] = 1, 1
	binary.LittleEndian.PutUint32(header[12:16], 24000)
	for _, data := range [][]byte{socketOggPage(header, 0), socketOggPage([]byte("OpusTags"), 0), socketOggPage([]byte{1, 2}, 960)} {
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			t.Fatal(err)
		}
		select {
		case <-stream.sent:
		case <-time.After(3 * time.Second):
			t.Fatal("ASR did not receive the page before end")
		}
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("end")); err != nil {
		t.Fatal(err)
	}
	if err := conn.ReadJSON(&event); err != nil || event.Type != "done" || event.Text != "你好呀" {
		t.Fatalf("final event=%+v err=%v", event, err)
	}
}
