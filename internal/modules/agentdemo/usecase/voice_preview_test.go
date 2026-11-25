package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

type previewResourceReader struct {
	resource domain.Resource
	id       int64
}

func (r *previewResourceReader) Find(_ context.Context, id int64) (domain.Resource, error) {
	r.id = id
	return r.resource, nil
}

func TestVoicePreviewUsesSavedResourceWithoutDeviceConfig(t *testing.T) {
	const (
		provider = "preview-provider"
		voiceID  = "saved-preview-voice"
		text     = "试听语音"
	)
	format := audio.Format{Encoding: audio.EncodingMP3, SampleRate: 24000, Channels: 1}
	client := &fakeSynthesizer{chunks: []audio.Chunk{{Format: format, Data: []byte{1, 2}}}}
	reader := &previewResourceReader{resource: domain.Resource{
		Kind: domain.ResourceVoice, Provider: provider, Reference: voiceID, Enabled: true,
	}}
	u := NewVoiceUsecase(nil, nil, func(name string) (tts.Client, error) {
		if name != provider {
			t.Fatalf("unexpected provider: %s", name)
		}
		return client, nil
	}, audio.Format{}).WithVoiceResources(reader)
	out, err := u.PreviewVoice(context.Background(), 17, text, false)
	if err != nil || len(out.Data) != 2 || reader.id != 17 || client.request.Voice != voiceID || client.request.Text != text {
		t.Fatalf("preview=%+v request=%+v err=%v", out, client.request, err)
	}
}

func TestVoicePreviewRejectsWrongKindDisabledAndOversizedText(t *testing.T) {
	reader := &previewResourceReader{}
	u := NewVoiceUsecase(nil, nil, func(string) (tts.Client, error) {
		t.Fatal("invalid preview reached provider")
		return nil, nil
	}, audio.Format{}).WithVoiceResources(reader)
	ctx := context.Background()
	for _, resource := range []domain.Resource{
		{Kind: domain.ResourceTool, Enabled: true},
		{Kind: domain.ResourceVoice, Enabled: false},
	} {
		reader.resource = resource
		if _, err := u.PreviewVoice(ctx, 1, "sample", false); err == nil {
			t.Fatal("invalid resource accepted")
		}
	}
	reader.resource = domain.Resource{Kind: domain.ResourceVoice, Enabled: true}
	if _, err := u.PreviewVoice(ctx, 1, strings.Repeat("x", maxVoicePreviewBytes+1), false); err == nil {
		t.Fatal("oversized preview accepted")
	}
}
