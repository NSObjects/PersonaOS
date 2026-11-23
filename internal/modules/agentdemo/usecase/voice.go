package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/NSObjects/go-speech/asr"
	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"

	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

const (
	maxVoicePCMBytes = 16000 * 2 * 15 // 15 seconds, mono, 16 kHz, signed PCM16LE.
	maxVoiceText     = 2000
	maxVoiceMP3Bytes = 4 << 20
	voiceTimeout     = 45 * time.Second
	asrFrameBytes    = 16000 * 2 * 40 / 1000 // 40 ms of PCM.
)

// VoiceSynthFactory selects only a server-configured provider, never an
// endpoint or credential supplied by a browser.
type VoiceSynthFactory func(provider string) (tts.Client, error)

// VoiceUsecase is a thin adapter around the existing go-speech SDK.
// It does not contain VAD, audio transcoding, Agent logic, or device playback.
type VoiceUsecase struct {
	configs     ConfigReader
	recognizer  asr.StreamRecognizer
	newSynth    VoiceSynthFactory
}

// NewVoiceUsecase reuses the confirmed device configuration reader.
func NewVoiceUsecase(configs ConfigReader, asrClient asr.StreamRecognizer, factory VoiceSynthFactory) *VoiceUsecase {
	return &VoiceUsecase{configs: configs, recognizer: asrClient, newSynth: factory}
}

// Recognize accepts raw 16 kHz mono PCM16LE captured by the browser.
// For this first Demo the recording is complete before upload. The SDK still
// receives 40 ms frames and ASR events; a future live capture can use the same
// SDK stream without changing its audio contract.
func (u *VoiceUsecase) Recognize(ctx context.Context, pcm []byte) (string, error) {
	if ctx == nil || len(pcm) == 0 || len(pcm) > maxVoicePCMBytes || len(pcm)%2 != 0 {
		return "", apperr.NewBadRequest("invalid 16 kHz mono PCM16LE recording")
	}
	if u == nil || u.recognizer == nil {
		return "", apperr.New(apperr.ErrInternalServer, "Demo ASR is not configured")
	}
	runCtx, cancel := context.WithTimeout(ctx, voiceTimeout)
	defer cancel()
	stream, err := u.recognizer.OpenStream(runCtx, asr.Config{
		Format: audio.Format{Encoding: audio.EncodingPCM16LE, SampleRate: 16000, Channels: 1},
	})
	if err != nil {
		return "", fmt.Errorf("open go-speech ASR: %w", err)
	}
	defer stream.Close()

	type received struct {
		text string
		err  error
	}
	done := make(chan received, 1)
	go func() {
		text, receiveErr := receiveASR(runCtx, stream)
		done <- received{text: text, err: receiveErr}
	}()
	sendErr := sendASR(runCtx, stream, pcm)
	if sendErr != nil {
		cancel()
	}
	result := <-done
	if sendErr != nil {
		return "", fmt.Errorf("send go-speech ASR audio: %w", sendErr)
	}
	if result.err != nil {
		return "", fmt.Errorf("receive go-speech ASR: %w", result.err)
	}
	if result.text == "" {
		return "", apperr.NewBadRequest("no recognizable speech")
	}
	return result.text, nil
}

func sendASR(ctx context.Context, stream asr.Stream, pcm []byte) error {
	// Real-time pacing is required by some streaming ASR providers. It is
	// deliberately not a provider-specific protocol implementation.
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for offset := 0; offset < len(pcm); offset += asrFrameBytes {
		if offset > 0 {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		end := offset + asrFrameBytes
		if end > len(pcm) {
			end = len(pcm)
		}
		if err := stream.Send(ctx, pcm[offset:end]); err != nil {
			return err
		}
	}
	return stream.CloseSend(ctx)
}

func receiveASR(ctx context.Context, stream asr.Stream) (string, error) {
	var segments []string
	var latestPartial string
	for {
		event, err := stream.Recv(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		text := strings.TrimSpace(event.Text)
		if text == "" {
			continue
		}
		switch event.Type {
		case asr.EventFinal:
			segments = append(segments, text)
			latestPartial = ""
		case asr.EventPartial:
			latestPartial = text
		}
	}
	if len(segments) == 0 {
		return latestPartial, nil
	}
	return strings.Join(segments, ""), nil
}

// Synthesize uses the Voice bound to the confirmed Device Type, never the
// voice ID or provider supplied by the browser.
func (u *VoiceUsecase) Synthesize(ctx context.Context, deviceTypeID int64, text string) ([]byte, error) {
	if ctx == nil || deviceTypeID <= 0 || strings.TrimSpace(text) == "" || len(text) > maxVoiceText {
		return nil, apperr.NewBadRequest("invalid voice synthesis request")
	}
	if u == nil || u.configs == nil || u.newSynth == nil {
		return nil, apperr.New(apperr.ErrInternalServer, "Demo TTS is not configured")
	}
	active, err := u.configs.Get(ctx, deviceTypeID)
	if err != nil {
		return nil, err
	}
	if active.Voice == nil || !active.Voice.Enabled {
		return nil, apperr.NewBadRequest("apply a voice to this device type first")
	}
	client, err := u.newSynth(active.Voice.Provider)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, voiceTimeout)
	defer cancel()
	format := audio.Format{Encoding: audio.EncodingMP3, SampleRate: 24000, Channels: 1}
	stream, err := client.SynthesizeStream(runCtx, tts.Request{
		Text: strings.TrimSpace(text), Voice: active.Voice.Reference, Format: format,
	})
	if err != nil {
		return nil, fmt.Errorf("open go-speech TTS: %w", err)
	}
	defer stream.Close()
	out := make([]byte, 0, 64*1024)
	for {
		chunk, receiveErr := stream.Recv(runCtx)
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			return nil, fmt.Errorf("receive go-speech TTS: %w", receiveErr)
		}
		if chunk.Format != format || len(chunk.Data)+len(out) > maxVoiceMP3Bytes {
			return nil, apperr.NewBadRequest("invalid or oversized MP3 from TTS")
		}
		out = append(out, chunk.Data...)
	}
	if len(out) == 0 {
		return nil, apperr.New(apperr.ErrInternalServer, "TTS returned empty audio")
	}
	return out, nil
}
