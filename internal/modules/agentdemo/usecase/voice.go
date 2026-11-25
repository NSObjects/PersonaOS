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

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
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
	configs      ConfigReader
	recognizer   asr.StreamRecognizer
	newSynth     VoiceSynthFactory
	asrFormat    audio.Format
	resources    VoiceResourceReader
	previewSlots chan struct{}
	runtime      func(context.Context) (asr.StreamRecognizer, VoiceSynthFactory, audio.Format, error)
}

// NewVoiceUsecase reuses the confirmed device configuration reader.
func NewVoiceUsecase(configs ConfigReader, asrClient asr.StreamRecognizer, factory VoiceSynthFactory, asrFormat audio.Format) *VoiceUsecase {
	return &VoiceUsecase{configs: configs, recognizer: asrClient, newSynth: factory, asrFormat: asrFormat, previewSlots: make(chan struct{}, 2)}
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
	format := u.asrFormat
	if format.Encoding == audio.EncodingOpus && format.SampleRate == 0 {
		format, err = opusASRFormat(input)
		if err != nil {
			return "", err
		}
	}
	return u.recognizeStream(runCtx, cancel, format, func(stream asr.Stream) error {
		if format.Encoding == audio.EncodingOpus {
			return sendOggASR(runCtx, stream, input)
		}
		return sendASR(runCtx, stream, input)
	})
}

func (u *VoiceUsecase) recognizeStream(runCtx context.Context, cancel context.CancelFunc, format audio.Format, send func(asr.Stream) error) (string, error) {
	stream, err := u.recognizer.OpenStream(runCtx, asr.Config{Format: format})
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
	err = send(stream)
	if err != nil {
		cancel()
	}
	result := <-done
	// A provider failure can close the stream before the sender finishes.
	// Preserve the receiver's original cause instead of the closed-stream symptom.
	if errors.Is(err, asr.ErrStreamClosed) && result.err != nil &&
		!errors.Is(result.err, asr.ErrStreamClosed) && !errors.Is(result.err, context.Canceled) {
		return "", fmt.Errorf("receive go-speech ASR: %w", result.err)
	}
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
	return u.synthesizeVoice(ctx, *active.Voice, text, preferOpus)
}

func (u *VoiceUsecase) synthesizeVoice(ctx context.Context, voice domain.Resource, text string, preferOpus bool) (audio.Buffer, error) {
	text = spokenText(text)
	if text == "" {
		return audio.Buffer{}, apperr.NewBadRequest("voice reply contains no spoken text")
	}
	client, err := u.newSynth(voice.Provider)
	if err != nil {
		return audio.Buffer{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, voiceTimeout)
	defer cancel()
	format := ttsOutputFormat(voice.Provider, preferOpus)
	stream, err := client.SynthesizeStream(runCtx, tts.Request{
		Text: text, Voice: voice.Reference, Format: format,
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
		case "volcengine", domain.Seed3CloneProvider:
			return audio.Format{Encoding: audio.EncodingOpus, SampleRate: 48000, Channels: 1}
		}
	}
	return audio.Format{Encoding: audio.EncodingMP3, SampleRate: 24000, Channels: 1}
}

// WithRuntimeResolver installs the request-scoped speech configuration loader.
func (u *VoiceUsecase) WithRuntimeResolver(resolve func(context.Context) (asr.StreamRecognizer, VoiceSynthFactory, audio.Format, error)) *VoiceUsecase {
	u.runtime = resolve
	return u
}

// Snapshot pins the recognizer, codec and synthesis factory for one request.
// The preview concurrency limiter and catalog reader remain shared.
func (u *VoiceUsecase) Snapshot(ctx context.Context) (*VoiceUsecase, error) {
	if u == nil || u.runtime == nil {
		return u, nil
	}
	recognizer, factory, format, err := u.runtime(ctx)
	if err != nil {
		return nil, err
	}
	next := *u
	next.recognizer, next.newSynth, next.asrFormat = recognizer, factory, format
	next.runtime = nil
	return &next, nil
}
