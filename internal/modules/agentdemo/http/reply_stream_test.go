package agentdemohttp

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/NSObjects/go-speech/audio"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
)

func TestWriteVoiceStream(t *testing.T) {
	w := httptest.NewRecorder()
	err := writeVoiceStream(w, func(emit func(audio.Chunk) error) (usecase.ChatOutput, error) {
		err := emit(audio.Chunk{Format: audio.Format{Encoding: audio.EncodingOpus}, Data: []byte("OggS")})
		return usecase.ChatOutput{Reply: "你好"}, err
	})
	if err != nil || w.Header().Get("Content-Type") != voiceStreamMIME || !w.Flushed {
		t.Fatalf("stream headers=%v, flushed=%t, err=%v", w.Header(), w.Flushed, err)
	}
	decoder := json.NewDecoder(w.Body)
	var first, last voiceEvent
	if err := decoder.Decode(&first); err != nil || first.Type != "audio" || string(first.Audio) != "OggS" || first.MIME != "audio/ogg" {
		t.Fatalf("first=%+v, err=%v", first, err)
	}
	if err := decoder.Decode(&last); err != nil || last.Type != streamEventDone || last.Reply == nil || last.Reply.Reply != "你好" {
		t.Fatalf("last=%+v, err=%v", last, err)
	}
}

func TestWriteVoiceStreamErrorBeforeHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	expected := errors.New("provider unavailable")
	err := writeVoiceStream(w, func(func(audio.Chunk) error) (usecase.ChatOutput, error) {
		return usecase.ChatOutput{}, expected
	})
	if !errors.Is(err, expected) || w.Body.Len() != 0 || w.Flushed {
		t.Fatalf("error=%v body=%q, flushed=%t", err, w.Body.String(), w.Flushed)
	}
}
