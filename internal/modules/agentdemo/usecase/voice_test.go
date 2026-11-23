package usecase

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/NSObjects/go-speech/asr"
	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

type fakeRecognizer struct{ stream *fakeRecognitionStream }

func (f *fakeRecognizer) OpenStream(_ context.Context, cfg asr.Config) (asr.Stream, error) {
	if cfg.Format.Encoding != audio.EncodingPCM16LE ||
		cfg.Format.SampleRate != 16000 || cfg.Format.Channels != 1 {
		return nil, errors.New("unexpected input format")
	}
	return f.stream, nil
}

type fakeRecognitionStream struct {
	sent   int
	ended  bool
	events []asr.Event
}

func (f *fakeRecognitionStream) Send(_ context.Context, data []byte) error {
	f.sent += len(data)
	return nil
}
func (f *fakeRecognitionStream) CloseSend(context.Context) error {
	f.ended = true
	return nil
}
func (f *fakeRecognitionStream) Close() error { return nil }
func (f *fakeRecognitionStream) Recv(context.Context) (asr.Event, error) {
	if len(f.events) == 0 {
		return asr.Event{}, io.EOF
	}
	event := f.events[0]
	f.events = f.events[1:]
	return event, nil
}

type fakeSynthesizer struct {
	request tts.Request
	chunks  []audio.Chunk
}

func (f *fakeSynthesizer) Synthesize(_ context.Context, _ tts.Request) (audio.Buffer, error) {
	return audio.Buffer{}, errors.New("batch TTS must not be used")
}
func (f *fakeSynthesizer) SynthesizeStream(_ context.Context, req tts.Request) (tts.Stream, error) {
	f.request = req
	return &fakeSynthesisStream{chunks: f.chunks}, nil
}

type fakeSynthesisStream struct{ chunks []audio.Chunk }

func (f *fakeSynthesisStream) Recv(context.Context) (audio.Chunk, error) {
	if len(f.chunks) == 0 {
		return audio.Chunk{}, io.EOF
	}
	chunk := f.chunks[0]
	f.chunks = f.chunks[1:]
	return chunk, nil
}
func (f *fakeSynthesisStream) Close() error { return nil }

func TestVoiceASRUsesSDKStreamingPCM(t *testing.T) {
	stream := &fakeRecognitionStream{events: []asr.Event{
		{Type: asr.EventPartial, Text: "今"},
		{Type: asr.EventFinal, Text: "今天天气"},
		{Type: asr.EventFinal, Text: "很好"},
	}}
	uc := NewVoiceUsecase(nil, &fakeRecognizer{stream: stream}, nil)
	text, err := uc.Recognize(context.Background(), make([]byte, 320))
	if err != nil || text != "今天天气很好" || stream.sent != 320 || !stream.ended {
		t.Fatalf("ASR transcript = %q, sent=%d, ended=%t, err=%v", text, stream.sent, stream.ended, err)
	}
}

func TestVoiceASRRejectsInvalidInput(t *testing.T) {
	uc := NewVoiceUsecase(nil, &fakeRecognizer{stream: &fakeRecognitionStream{}}, nil)
	for _, data := range [][]byte{nil, {1}, make([]byte, maxVoicePCMBytes+2)} {
		if _, err := uc.Recognize(context.Background(), data); err == nil {
			t.Fatalf("invalid recording length %d accepted", len(data))
		}
	}
}

func TestVoiceTTSUsesAppliedVoiceNotBrowserVoice(t *testing.T) {
	client := &fakeSynthesizer{chunks: []audio.Chunk{
		{Data: []byte{1, 2}, Format: audio.Format{Encoding: audio.EncodingMP3, SampleRate: 24000, Channels: 1}},
		{Data: []byte{3}, Format: audio.Format{Encoding: audio.EncodingMP3, SampleRate: 24000, Channels: 1}},
	}}
	config := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Voice: &domain.Resource{Kind: domain.ResourceVoice,
			Provider: "volcengine", Reference: "voice-123", Enabled: true},
	}}
	uc := NewVoiceUsecase(config, nil, func(provider string) (tts.Client, error) {
		if provider != "volcengine" {
			t.Fatalf("unexpected provider %q", provider)
		}
		return client, nil
	})
	result, err := uc.Synthesize(context.Background(), 17, "你好")
	if err != nil || len(result) != 3 || client.request.Voice != "voice-123" ||
		client.request.Text != "你好" || client.request.Format.Encoding != audio.EncodingMP3 {
		t.Fatalf("TTS = %v, req=%+v, err=%v", result, client.request, err)
	}
}

func TestVoiceTTSRequiresAppliedVoice(t *testing.T) {
	config := &fakeConfigReader{value: domain.PublishedDeviceConfig{}}
	uc := NewVoiceUsecase(config, nil, func(string) (tts.Client, error) {
		t.Fatal("should not create client without a configured voice")
		return nil, nil
	})
	if _, err := uc.Synthesize(context.Background(), 1, "hello"); err == nil {
		t.Fatal("synthesis without applied voice must be rejected")
	}
}
