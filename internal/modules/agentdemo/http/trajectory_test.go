package agentdemohttp

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/NSObjects/go-speech/audio"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
)

func TestTrajectoryWritesAudioAndTerminalFrame(t *testing.T) {
	w := httptest.NewRecorder()
	err := writeTrajectory(context.Background(), w, func(_ context.Context, emit func(audio.Chunk) error) (usecase.ChatOutput, error) {
		err := emit(audio.Chunk{Format: audio.Format{Encoding: audio.EncodingOpus}, Data: []byte("OggS")})
		return usecase.ChatOutput{Reply: "hello"}, err
	})
	if err != nil || !w.Flushed || w.Header().Get("Content-Type") != voiceStreamMIME {
		t.Fatalf("headers=%v flushed=%t error=%v", w.Header(), w.Flushed, err)
	}
	decoder := json.NewDecoder(w.Body)
	var first, last trajectoryFrame
	if err := decoder.Decode(&first); err != nil || first.Type != "audio" || string(first.Audio) != "OggS" {
		t.Fatalf("first=%+v error=%v, want audio", first, err)
	}
	if err := decoder.Decode(&last); err != nil || last.Type != "done" || last.Reply == nil || last.Reply.Reply != "hello" {
		t.Fatalf("last=%+v error=%v, want terminal reply", last, err)
	}
}
