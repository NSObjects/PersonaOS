package agentdemohttp

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"

	"github.com/NSObjects/go-speech/audio"
	"github.com/labstack/echo/v5"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
)

type trajectoryFrame struct {
	Type  string                  `json:"type"`
	Event *usecase.ExecutionEvent `json:"event,omitempty"`
	MIME  string                  `json:"mime,omitempty"`
	Audio []byte                  `json:"audio,omitempty"`
	Reply *usecase.ChatOutput     `json:"reply,omitempty"`
}

type trajectoryResult struct {
	out usecase.ChatOutput
	err error
}

func (h *Handler) streamTrajectory(c *echo.Context, in usecase.ChatInput) error {
	voice := c.QueryParam("voice") == "1"
	return writeTrajectory(c.Request().Context(), c.Response(), func(ctx context.Context, emit func(audio.Chunk) error) (usecase.ChatOutput, error) {
		if voice {
			return h.voice.StreamReply(ctx, h.chat, in, emit)
		}
		return h.chat.Chat(ctx, in)
	})
}

func writeTrajectory(ctx context.Context, w http.ResponseWriter, run func(context.Context, func(audio.Chunk) error) (usecase.ChatOutput, error)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	frames := make(chan trajectoryFrame, 128)
	var dropped atomic.Bool
	ctx = usecase.WithExecutionObserver(ctx, func(event usecase.ExecutionEvent) {
		select {
		case frames <- trajectoryFrame{Type: "trace", Event: &event}:
		default:
			dropped.Store(true)
		}
	})
	done := make(chan trajectoryResult, 1)
	go func() {
		out, err := run(ctx, func(chunk audio.Chunk) error {
			select {
			case frames <- trajectoryFrame{Type: "audio", MIME: usecase.AudioMIME(chunk.Format), Audio: chunk.Data}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		done <- trajectoryResult{out: out, err: err}
	}()
	w.Header().Set("Content-Type", voiceStreamMIME)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	encoder := json.NewEncoder(w)
	controller := http.NewResponseController(w)
	write := func(frame trajectoryFrame) error {
		if err := encoder.Encode(frame); err != nil {
			return err
		}
		return controller.Flush()
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case frame := <-frames:
			if err := write(frame); err != nil {
				return err
			}
		case result := <-done:
			return finishTrajectory(frames, write, result, dropped.Load())
		}
	}
}

func finishTrajectory(frames <-chan trajectoryFrame, write func(trajectoryFrame) error, result trajectoryResult, dropped bool) error {
	for {
		select {
		case frame := <-frames:
			if err := write(frame); err != nil {
				return err
			}
		default:
			if dropped {
				if err := write(trajectoryFrame{Type: "trace_gap"}); err != nil {
					return err
				}
			}
			if result.err != nil {
				return write(trajectoryFrame{Type: streamEventError})
			}
			return write(trajectoryFrame{Type: streamEventDone, Reply: &result.out})
		}
	}
}
