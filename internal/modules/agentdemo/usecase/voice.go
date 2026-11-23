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
	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

const (
	maxVoicePCMBytes   = 16000 * 2 * 15 // 15 seconds, mono, 16 kHz, signed PCM16LE.
	maxVoiceOpusBytes  = 1 << 20        // Maximum compressed Opus capture size.
	maxVoiceText       = 2000
	maxVoiceAudioBytes = 4 << 20
	voiceTimeout       = 45 * time.Second
	asrFrameBytes      = 16000 * 2 * 40 / 1000 // 40 ms of PCM.
)

// VoiceSynthFactory selects only a server-configured provider, never an
// endpoint or credential supplied by a browser.
type VoiceSynthFactory func(provider string) (tts.Client, error)

// VoiceUsecase is a thin adapter around the existing go-speech SDK.
// It does not contain VAD, audio transcoding, Agent logic, or device playback.
type VoiceUsecase struct {
	configs    ConfigReader
	recognizer asr.StreamRecognizer
	newSynth   VoiceSynthFactory
	asrFormat  audio.Format
}

// NewVoiceUsecase reuses the confirmed device configuration reader.
func NewVoiceUsecase(configs ConfigReader, asrClient asr.StreamRecognizer, factory VoiceSynthFactory, asrFormat audio.Format) *VoiceUsecase {
	return &VoiceUsecase{configs: configs, recognizer: asrClient, newSynth: factory, asrFormat: asrFormat}
}

// Recognize accepts an Ogg Opus or browser WebM/Opus capture. The SDK is
// always given the exact codec its selected ASR provider supports. Tencent
// needs PCM16LE; Aliyun and Volcengine accept Ogg Opus without re-encoding.
func (u *VoiceUsecase) Recognize(ctx context.Context, encoded []byte, mediaType string) (string, error) {
	if ctx == nil || len(encoded) == 0 || len(encoded) > maxVoiceOpusBytes {
		return "", apperr.NewBadRequest("invalid Opus recording")
	}
	if u == nil || u.recognizer == nil {
		return "", apperr.New(apperr.ErrInternalServer, "demo ASR is not configured")
	}
	runCtx, cancel := context.WithTimeout(ctx, voiceTimeout)
	defer cancel()
	input, err := prepareASRAudio(runCtx, encoded, mediaType, u.asrFormat.Encoding)
	if err != nil {
		return "", err
	}
	stream, err := u.recognizer.OpenStream(runCtx, asr.Config{Format: u.asrFormat})
	if err != nil {
		return "", fmt.Errorf("open go-speech ASR: %w", err)
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("close go-speech stream")
		}
	}()
	type received struct {
		text string
		err  error
	}
	done := make(chan received, 1)
	go func() {
		text, receiveErr := receiveASR(runCtx, stream)
		done <- received{text: text, err: receiveErr}
	}()
	if u.asrFormat.Encoding == audio.EncodingOpus {
		err = sendOggASR(runCtx, stream, input)
	} else {
		err = sendASR(runCtx, stream, input)
	}
	if err != nil {
		cancel()
	}
	result := <-done
	if err != nil {
		return "", fmt.Errorf("send go-speech ASR audio: %w", err)
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
func (u *VoiceUsecase) Synthesize(ctx context.Context, deviceTypeID int64, text string, preferOpus bool) (audio.Buffer, error) {
	if ctx == nil || deviceTypeID <= 0 || strings.TrimSpace(text) == "" || len(text) > maxVoiceText {
		return audio.Buffer{}, apperr.NewBadRequest("invalid voice synthesis request")
	}
	if u == nil || u.configs == nil || u.newSynth == nil {
		return audio.Buffer{}, apperr.New(apperr.ErrInternalServer, "Demo TTS is not configured")
	}
	active, err := u.configs.Get(ctx, deviceTypeID)
	if err != nil {
		return audio.Buffer{}, err
	}
	if active.Voice == nil || !active.Voice.Enabled {
		return audio.Buffer{}, apperr.NewBadRequest("apply a voice to this device type first")
	}
	client, err := u.newSynth(active.Voice.Provider)
	if err != nil {
		return audio.Buffer{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, voiceTimeout)
	defer cancel()
	format := ttsOutputFormat(active.Voice.Provider, preferOpus)
	stream, err := client.SynthesizeStream(runCtx, tts.Request{
		Text: strings.TrimSpace(text), Voice: active.Voice.Reference, Format: format,
	})
	if err != nil {
		return audio.Buffer{}, fmt.Errorf("open go-speech TTS: %w", err)
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("close go-speech TTS stream")
		}
	}()
	return collectSynthAudio(runCtx, stream, format)
}

func collectSynthAudio(ctx context.Context, stream tts.Stream, format audio.Format) (audio.Buffer, error) {
	out := make([]byte, 0, 64*1024)
	for {
		chunk, receiveErr := stream.Recv(ctx)
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			return audio.Buffer{}, fmt.Errorf("receive go-speech TTS: %w", receiveErr)
		}
		if chunk.Format != format || len(chunk.Data)+len(out) > maxVoiceAudioBytes {
			return audio.Buffer{}, apperr.NewBadRequest("invalid or oversized audio from TTS")
		}
		out = append(out, chunk.Data...)
	}
	if len(out) == 0 {
		return audio.Buffer{}, apperr.New(apperr.ErrInternalServer, "TTS returned empty audio")
	}
	return audio.Buffer{Data: out, Format: format}, nil
}

func ttsOutputFormat(provider string, preferOpus bool) audio.Format {
	if preferOpus {
		switch strings.ToLower(provider) {
		case "aliyun":
			return audio.Format{Encoding: audio.EncodingOpus, SampleRate: 24000, Channels: 1}
		case "volcengine":
			return audio.Format{Encoding: audio.EncodingOpus, SampleRate: 48000, Channels: 1}
		}
	}
	return audio.Format{Encoding: audio.EncodingMP3, SampleRate: 24000, Channels: 1}
}
