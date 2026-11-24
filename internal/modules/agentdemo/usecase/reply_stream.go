package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"
	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

type speechReply struct {
	reply string
	scope memory.Scope
	err   error
}

// StreamReply forwards Eino text increments to one duplex TTS session and
// emits its audio in order. Chat and synthesis share the applied snapshot.
func (u *VoiceUsecase) StreamReply(ctx context.Context, chat *ChatUsecase, in ChatInput, emit func(audio.Chunk) error) (ChatOutput, error) {
	active, err := chat.activeConfig(ctx, in)
	if err != nil {
		return ChatOutput{}, err
	}
	if u == nil || u.newSynth == nil || emit == nil {
		return ChatOutput{}, apperr.New(apperr.ErrInternalServer, "demo voice stream is not configured")
	}
	if active.Voice == nil || !active.Voice.Enabled {
		return ChatOutput{}, apperr.NewBadRequest("apply a voice to this device type first")
	}
	runCtx, cancel := context.WithTimeout(ctx, voiceTimeout)
	defer cancel()
	format := ttsOutputFormat(active.Voice.Provider, true)
	stream, err := u.openReplyStream(runCtx, active.Voice, format)
	if err != nil {
		return ChatOutput{}, err
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("close duplex TTS stream")
		}
	}()

	// Only this goroutine writes text. The caller drains audio. Cancellation
	// interrupts the other direction on failure, and we join before returning.
	done := make(chan speechReply, 1)
	go func() {
		result := sendReplyText(runCtx, chat, in, active, stream)
		done <- result
		if result.err != nil {
			cancel()
		}
	}()
	receiveErr := receiveReplyAudio(runCtx, stream, format, emit)
	if receiveErr != nil {
		cancel()
	}
	result := <-done
	if result.err != nil && (receiveErr == nil || errors.Is(receiveErr, context.Canceled) || errors.Is(receiveErr, tts.ErrStreamClosed)) {
		return ChatOutput{}, result.err
	}
	if receiveErr != nil {
		return ChatOutput{}, receiveErr
	}
	return chat.completeChat(in, active, result.scope, result.reply), nil
}

func (u *VoiceUsecase) openReplyStream(ctx context.Context, voice *domain.Resource, format audio.Format) (tts.DuplexStream, error) {
	client, err := u.newSynth(voice.Provider)
	if err != nil {
		return nil, err
	}
	duplex, ok := client.(tts.DuplexSynthesizer)
	if !ok {
		return nil, tts.ErrDuplexUnsupported
	}
	stream, err := duplex.OpenDuplex(ctx, tts.DuplexRequest{Voice: voice.Reference, Format: format})
	if err != nil {
		return nil, fmt.Errorf("open duplex TTS: %w", err)
	}
	return stream, nil
}

func sendReplyText(ctx context.Context, chat *ChatUsecase, in ChatInput, active domain.PublishedDeviceConfig, stream tts.DuplexStream) speechReply {
	var pending string
	var size int
	reply, scope, err := chat.runChat(ctx, in, active, func(delta string) error {
		size += len(delta)
		if size > maxVoiceText {
			return apperr.NewBadRequest("voice reply is too long")
		}
		// The SDK rejects whitespace-only fragments. Preserve them until the
		// next spoken fragment, without dropping spaces inside the response.
		pending += delta
		if strings.TrimSpace(pending) == "" {
			return nil
		}
		if err := stream.SendText(ctx, pending); err != nil {
			return fmt.Errorf("send duplex TTS text: %w", err)
		}
		pending = ""
		return nil
	})
	if err == nil {
		if closeErr := stream.CloseSend(ctx); closeErr != nil {
			err = fmt.Errorf("finish duplex TTS input: %w", closeErr)
		}
	}
	return speechReply{reply: reply, scope: scope, err: err}
}

func receiveReplyAudio(ctx context.Context, stream tts.DuplexStream, format audio.Format, emit func(audio.Chunk) error) error {
	total := 0
	for {
		chunk, err := stream.Recv(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("receive duplex TTS audio: %w", err)
		}
		total += len(chunk.Data)
		if chunk.Format != format || total > maxVoiceAudioBytes {
			return apperr.NewBadRequest("invalid or oversized audio from TTS")
		}
		if len(chunk.Data) > 0 {
			if err := emit(chunk); err != nil {
				return err
			}
		}
	}
	if total == 0 {
		return apperr.New(apperr.ErrInternalServer, "TTS returned empty audio")
	}
	return nil
}
