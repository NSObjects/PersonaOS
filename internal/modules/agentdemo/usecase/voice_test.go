package usecase

import (
	"context"
	"errors"
	"io"
	"encoding/binary"
	"testing"

	"github.com/NSObjects/go-speech/asr"
	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

type fakeRecognizer struct {
	stream *fakeRecognitionStream
	want audio.Encoding
}

func (f *fakeRecognizer) OpenStream(_ context.Context, cfg asr.Config) (asr.Stream, error) {
	if cfg.Format.Encoding != f.want ||
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

func makeTestOggPage(body []byte, granule uint64) []byte {
	page := make([]byte, 28+len(body))
	copy(page[:4], []byte("OggS"))
	binary.LittleEndian.PutUint64(page[6:14], granule)
	page[26] = 1
	page[27] = byte(len(body))
	copy(page[28:], body)
	return page
}

func makeTestOgg() []byte {
	first := makeTestOggPage([]byte("OpusHead"), 0)
	second := makeTestOggPage([]byte("OpusTags"), 0)
	third := makeTestOggPage([]byte{0x01, 0x02}, 960)
	return append(append(first, second...), third...)
}

func TestVoiceASRUsesSDKStreamingOpus(t *testing.T) {
	stream := &fakeRecognitionStream{events: []asr.Event{
		{Type: asr.EventPartial, Text: "今"},
		{Type: asr.EventFinal, Text: "今天天气"},
		{Type: asr.EventFinal, Text: "很好"},
	}}
	recognizer := &fakeRecognizer{stream: stream, want: audio.EncodingOpus}
	uc := NewVoiceUsecase(nil, recognizer, nil, audio.Format{
		Encoding: audio.EncodingOpus, SampleRate: 16000, Channels: 1,
	})
	data := makeTestOgg()
	text, err := uc.Recognize(context.Background(), data, "audio/ogg")
	if err != nil || text != "今天天气很好" || stream.sent != len(data) || !stream.ended {
		t.Fatalf("ASR transcript = %q, sent=%d, ended=%t, err=%v", text, stream.sent, stream.ended, err)
	}
}

func TestVoiceASRRejectsInvalidInput(t *testing.T) {
	uc := NewVoiceUsecase(nil, &fakeRecognizer{stream: &fakeRecognitionStream{}, want: audio.EncodingOpus},
		nil, audio.Format{Encoding: audio.EncodingOpus, SampleRate: 16000, Channels: 1})
	for _, data := range [][]byte{nil, {1}, make([]byte, maxVoiceOpusBytes+1)} {
		if _, err := uc.Recognize(context.Background(), data, "audio/ogg"); err == nil {
			t.Fatalf("invalid recording length %d accepted", len(data))
		}
	}
}

func TestOpusOggPagesValidateCodecAndDuration(t *testing.T) {
	pages, err := oggPages(makeTestOgg())
	if err != nil || len(pages) != 3 {
		t.Fatalf("pages = %d, err=%v", len(pages), err)
	}
	bad := makeTestOgg()
	bad[28] = 'N'
	if _, err := oggPages(bad); err == nil {
		t.Fatal("non-Opus first page accepted")
	}
	bad = makeTestOgg()
	binary.LittleEndian.PutUint64(bad[len(bad)-24:len(bad)-16], maxOpusDurationSamples+1)
	if _, err := oggPages(bad); err == nil {
		t.Fatal("recording exceeding duration limit accepted")
	}
	if _, err := oggPages(makeTestOgg()[:12]); err == nil {
		t.Fatal("truncated Ogg accepted")
	}
}

func TestVoiceTTSUsesAppliedVoiceNotBrowserVoice(t *testing.T) {
	client := &fakeSynthesizer{chunks: []audio.Chunk{
		{Data: []byte{1, 2}, Format: audio.Format{Encoding: audio.EncodingOpus, SampleRate: 48000, Channels: 1}},
		{Data: []byte{3}, Format: audio.Format{Encoding: audio.EncodingOpus, SampleRate: 48000, Channels: 1}},
	}}
	config := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Voice: &domain.Resource{
			Kind:     domain.ResourceVoice,
			Provider: "volcengine", Reference: "voice-123", Enabled: true,
		},
	}}
	uc := NewVoiceUsecase(config, nil, func(provider string) (tts.Client, error) {
		if provider != "volcengine" {
			t.Fatalf("unexpected provider %q", provider)
		}
		return client, nil
	}, audio.Format{})
	result, err := uc.Synthesize(context.Background(), 17, "你好", true)
	if err != nil || len(result.Data) != 3 || client.request.Voice != "voice-123" ||
		client.request.Text != "你好" || client.request.Format.Encoding != audio.EncodingOpus {
		t.Fatalf("TTS = %v, req=%+v, err=%v", result, client.request, err)
	}
}

func TestVoiceTTSRequiresAppliedVoice(t *testing.T) {
	config := &fakeConfigReader{value: domain.PublishedDeviceConfig{}}
	uc := NewVoiceUsecase(config, nil, func(string) (tts.Client, error) {
		t.Fatal("should not create client without a configured voice")
		return nil, nil
	}, audio.Format{})
	if _, err := uc.Synthesize(context.Background(), 1, "hello", true); err == nil {
		t.Fatal("synthesis without applied voice must be rejected")
	}
}

func TestVoiceTTSProviderFormatCompatibility(t *testing.T) {
	if format := ttsOutputFormat("aliyun", true); format.Encoding != audio.EncodingOpus ||
		format.SampleRate != 24000 {
		t.Fatalf("Aliyun should use Opus 24k: %+v", format)
	}
	if format := ttsOutputFormat("tencent", true); format.Encoding != audio.EncodingMP3 {
		t.Fatalf("Tencent should use MP3 fallback: %+v", format)
	}
	if format := ttsOutputFormat("volcengine", false); format.Encoding != audio.EncodingMP3 {
		t.Fatalf("browser without Ogg playback should use MP3: %+v", format)
	}
	if AudioMIME(ttsOutputFormat("aliyun", true)) != "audio/ogg" {
		t.Fatal("Opus response must have audio/ogg MIME")
	}
}
