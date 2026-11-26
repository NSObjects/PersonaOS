package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"
	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

type speechReply struct {
	reply     string
	scope     memory.Scope
	err       error
	firstText time.Duration
	firstSent time.Duration
}

// StreamReply forwards Eino text increments to one duplex TTS session and
// emits its audio in order. Chat and synthesis share the applied snapshot.
func (u *VoiceUsecase) StreamReply(ctx context.Context, chat *ChatUsecase, in ChatInput, emit func(audio.Chunk) error) (ChatOutput, error) {
	in.spoken = true
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
	duplex, err := u.replySynthesizer(active.Voice.Provider)
	if err != nil {
		return ChatOutput{}, err
	}
	result, err := runDuplexReply(runCtx, cancel, chat, in, active, duplex, emit)
	if err != nil {
		return ChatOutput{}, err
	}
	return chat.completeChat(in, active, result.scope, result.reply), nil
}

func (u *VoiceUsecase) replySynthesizer(provider string) (tts.DuplexSynthesizer, error) {
	client, err := u.newSynth(provider)
	if err != nil {
		return nil, err
	}
	duplex, ok := client.(tts.DuplexSynthesizer)
	if !ok {
		return nil, tts.ErrDuplexUnsupported
	}
	return duplex, nil
}

func runDuplexReply(ctx context.Context, cancel context.CancelFunc, chat *ChatUsecase, in ChatInput, active domain.PublishedDeviceConfig, duplex tts.DuplexSynthesizer, emit func(audio.Chunk) error) (speechReply, error) {
	start := time.Now()
	format := ttsOutputFormat(active.Voice.Provider, true)
	ready := make(chan struct{})
	var stream tts.DuplexStream
	// Publish the opened stream exactly once. The LLM can start immediately
	// and only its first text send waits for the provider handshake.
	awaitStream := func() (tts.DuplexStream, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ready:
			return stream, nil
		}
	}
	done := make(chan speechReply, 1)
	go func() {
		result := sendReplyText(ctx, chat, in, active, awaitStream, start)
		done <- result
		if result.err != nil {
			cancel()
		}
	}()
	finishOpen := observeStage(ctx, "audio", "Open TTS")
	opened, err := duplex.OpenDuplex(ctx, tts.DuplexRequest{Voice: active.Voice.Reference, Format: format})
	finishOpen(err)
	if err != nil {
		cancel()
		result := <-done
		if errors.Is(err, context.Canceled) && result.err != nil && !errors.Is(result.err, context.Canceled) {
			return speechReply{}, result.err
		}
		return speechReply{}, fmt.Errorf("open duplex TTS: %w", err)
	}
	stream = opened
	openTime := time.Since(start)
	close(ready)
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("close duplex TTS stream")
		}
	}()
	var firstAudio time.Duration
	audioStage := startExecutionStage(ctx, "audio", "TTS audio stream", "")
	receiveErr := receiveReplyAudio(ctx, stream, format, func(chunk audio.Chunk) error {
		if firstAudio == 0 {
			firstAudio = time.Since(start)
			audioStage.firstOutput()
		}
		return emit(chunk)
	})
	audioStage.finish("", receiveErr)
	if receiveErr != nil {
		cancel()
	}
	result := <-done
	log.Info().Int64("tts_open_ms", openTime.Milliseconds()).
		Int64("llm_first_text_ms", result.firstText.Milliseconds()).
		Int64("tts_first_text_sent_ms", result.firstSent.Milliseconds()).
		Int64("first_audio_bytes_ms", firstAudio.Milliseconds()).
		Int64("total_ms", time.Since(start).Milliseconds()).Msg("demo voice reply timing")
	return result, replyStreamError(result.err, receiveErr)
}

func replyStreamError(sendErr, receiveErr error) error {
	if sendErr != nil && (receiveErr == nil || errors.Is(receiveErr, context.Canceled) || errors.Is(receiveErr, tts.ErrStreamClosed)) {
		return sendErr
	}
	return receiveErr
}

func sendReplyText(ctx context.Context, chat *ChatUsecase, in ChatInput, active domain.PublishedDeviceConfig, awaitStream func() (tts.DuplexStream, error), start time.Time) speechReply {
	var pending string
	var size int
	var firstText, firstSent time.Duration
	var stream tts.DuplexStream
	filter := newSpeechTextFilter()
	var submitted strings.Builder
	stage := startExecutionStage(ctx, "audio", "TTS submitted text", "")
	send := func(text string, waiting bool) error {
		// The SDK rejects whitespace-only fragments. Keep them until the
		// next spoken fragment without leaking Markdown syntax to TTS.
		pending += text
		if strings.TrimSpace(pending) == "" {
			return nil
		}
		if stream == nil {
			var openErr error
			stream, openErr = awaitStream()
			if openErr != nil {
				return openErr
			}
		}
		if err := stream.SendText(ctx, pending); err != nil {
			return fmt.Errorf("send duplex TTS text: %w", err)
		}
		submitted.WriteString(pending)
		if firstSent == 0 {
			firstSent = time.Since(start)
			stage.firstOutput()
		}
		pending = ""
		if waiting {
			return flushWaitingText(ctx, stream)
		}
		return nil
	}
	reply, scope, err := runChatWithToolFeedback(ctx, chat, in, active, func(text string) error { return send(text, true) }, func(delta string) error {
		size += len(delta)
		if size > maxVoiceText {
			return apperr.NewBadRequest("voice reply is too long")
		}
		if firstText == 0 && strings.TrimSpace(delta) != "" {
			firstText = time.Since(start)
		}
		return send(filter.Write(delta), false)
	})
	if err == nil {
		err = send(filter.Finish(), false)
	}
	if err == nil {
		if stream == nil {
			err = apperr.NewBadRequest("voice reply contains no spoken text")
		} else if closeErr := stream.CloseSend(ctx); closeErr != nil {
			err = fmt.Errorf("finish duplex TTS input: %w", closeErr)
		}
	}
	stage.finish(submitted.String(), err)
	return speechReply{reply: reply, scope: scope, err: err, firstText: firstText, firstSent: firstSent}
}

// flushWaitingText submits a short waiting phrase on providers that buffer
// incremental text (e.g. Seed-TTS 3.0), without closing the duplex session.
func flushWaitingText(ctx context.Context, stream tts.DuplexStream) error {
	if flusher, ok := stream.(interface{ Flush(context.Context) error }); ok {
		if err := flusher.Flush(ctx); err != nil {
			return fmt.Errorf("flush duplex TTS waiting text: %w", err)
		}
	}
	return nil
}

// runChatWithToolFeedback closes the turn-local feedback gate before the
// caller flushes and closes the one TTS stream.
func runChatWithToolFeedback(ctx context.Context, chat *ChatUsecase, in ChatInput, active domain.PublishedDeviceConfig, send, emit func(string) error) (string, memory.Scope, error) {
	feedback := &voiceToolFeedback{send: send}
	chatCtx := context.WithValue(ctx, toolFeedbackKey{}, feedback)
	reply, scope, err := chat.runChat(chatCtx, in, active, func(delta string) error {
		return feedback.reply(ctx, strings.TrimSpace(delta), func() error { return emit(delta) })
	})
	feedbackErr := feedback.finish()
	if err == nil {
		err = feedbackErr
	}
	return reply, scope, err
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
