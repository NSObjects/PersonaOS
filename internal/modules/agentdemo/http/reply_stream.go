package agentdemohttp

import (
	"encoding/json"
	"net/http"

	"github.com/NSObjects/go-speech/audio"
	"github.com/labstack/echo/v5"
	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
)

const voiceStreamMIME = "application/x-ndjson"

type voiceEvent struct {
	Type  string              `json:"type"`
	MIME  string              `json:"mime,omitempty"`
	Audio []byte              `json:"audio,omitempty"`
	Reply *usecase.ChatOutput `json:"reply,omitempty"`
}

func (h *Handler) streamReply(c *echo.Context, in usecase.ChatInput) error {
	return writeVoiceStream(c.Response(), func(emit func(audio.Chunk) error) (usecase.ChatOutput, error) {
		return h.voice.StreamReply(c.Request().Context(), h.chat, in, emit)
	})
}

func writeVoiceStream(w http.ResponseWriter, run func(func(audio.Chunk) error) (usecase.ChatOutput, error)) error {
	encoder := json.NewEncoder(w)
	controller := http.NewResponseController(w)
	started := false
	write := func(event voiceEvent) error {
		if !started {
			w.Header().Set("Content-Type", voiceStreamMIME)
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Accel-Buffering", "no")
			started = true
		}
		if err := encoder.Encode(event); err != nil {
			return err
		}
		return controller.Flush()
	}
	out, err := run(func(chunk audio.Chunk) error {
		return write(voiceEvent{Type: "audio", MIME: usecase.AudioMIME(chunk.Format), Audio: chunk.Data})
	})
	if err != nil {
		if !started {
			return err
		}
		// Headers have already been sent; report a terminal event rather than
		// appending a normal HTTP error envelope to the audio stream.
		log.Error().Err(err).Msg("demo voice stream failed")
		return write(voiceEvent{Type: "error"})
	}
	return write(voiceEvent{Type: "done", Reply: &out})
}
