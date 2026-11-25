package voiceconfig

import (
	"testing"

	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

func TestAliyunUsesSDKDefaultEndpoint(t *testing.T) {
	t.Setenv("PERSONAOS_ASR_PROVIDER", "aliyun")
	t.Setenv("DASHSCOPE_API_KEY", "test-key")
	t.Setenv("DASHSCOPE_WS_ENDPOINT", "")

	asrClient, err := NewASRFromEnv()
	if err != nil || asrClient == nil {
		t.Fatalf("create Aliyun ASR with default endpoint: %v", err)
	}
	ttsClient, err := NewTTSFromEnv("aliyun")
	if err != nil || ttsClient == nil {
		t.Fatalf("create Aliyun TTS with default endpoint: %v", err)
	}
}

func TestASRInputFormat(t *testing.T) {
	for _, tc := range []struct {
		provider string
		want     audio.Format
	}{
		{"aliyun", audio.Format{Encoding: audio.EncodingOpus, Channels: 1}},
		{"volcengine", audio.Format{Encoding: audio.EncodingOpus, SampleRate: 16000, Channels: 1}},
		{"tencent", audio.Format{Encoding: audio.EncodingPCM16LE, SampleRate: 16000, Channels: 1}},
	} {
		t.Setenv("PERSONAOS_ASR_PROVIDER", tc.provider)
		if got := ASRInputFormat(); got != tc.want {
			t.Fatalf("%s: input format = %+v, want %+v", tc.provider, got, tc.want)
		}
	}
}

func TestVolcengineOfficialAndCloneShareDuplexProvider(t *testing.T) {
	t.Setenv("VOLCENGINE_API_KEY", "test-key")
	t.Setenv("VOLCENGINE_TTS_MODEL_ID", "")
	for _, provider := range []string{providerVolcengine, domain.Seed3CloneProvider} {
		client, err := NewTTSFromEnv(provider)
		if err != nil || client == nil {
			t.Fatalf("%s client=%v err=%v", provider, client, err)
		}
		if _, ok := client.(tts.DuplexSynthesizer); !ok {
			t.Fatalf("%s has no duplex synthesis", provider)
		}
	}
}
