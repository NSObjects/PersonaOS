// Package voiceconfig selects go-speech providers using server-only secrets.
package voiceconfig

import (
	"errors"
	"os"
	"strings"

	speech "github.com/NSObjects/go-speech"
	"github.com/NSObjects/go-speech/asr"
	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const (
	defaultVolcASRResource = "volc.seedasr.sauc.duration"
	providerAliyun         = "aliyun"
	providerVolcengine     = "volcengine"
	providerTencent        = "tencent"
)

// TTSProviders returns the providers supported by the server's speech SDK.
func TTSProviders() []string {
	return []string{providerAliyun, providerVolcengine, providerTencent}
}

// NewASRFromEnv returns nil when the Demo has no ASR provider configured.
// Only these three built-in providers are selectable; no client-supplied URL
// or credentials are accepted.
func NewASRFromEnv() (asr.StreamRecognizer, error) { return NewASR(os.Getenv) }

// NewASR constructs the recognizer from one effective configuration snapshot.
func NewASR(get func(string) string) (asr.StreamRecognizer, error) {
	switch strings.ToLower(get("PERSONAOS_ASR_PROVIDER")) {
	case "":
		return nil, nil
	case providerVolcengine:
		resource := get("VOLCENGINE_ASR_RESOURCE_ID")
		if resource == "" {
			resource = defaultVolcASRResource
		}
		return speech.NewASR(speech.VolcengineASR{
			APIKey: get("VOLCENGINE_API_KEY"), ResourceID: resource,
		})
	case providerAliyun:
		return speech.NewASR(speech.AliyunASR{
			APIKey:   get("DASHSCOPE_API_KEY"),
			Endpoint: os.Getenv("DASHSCOPE_WS_ENDPOINT"),
		})
	case providerTencent:
		return speech.NewASR(speech.TencentASR{
			AppID: get("TENCENT_APP_ID"), SecretID: get("TENCENT_SECRET_ID"),
			SecretKey: get("TENCENT_SECRET_KEY"), EngineModel: "16k_zh_en_2.0",
			NeedVAD: true,
		})
	default:
		return nil, errors.New("unsupported PERSONAOS_ASR_PROVIDER")
	}
}

// ASRInputFormat keeps the transport aligned with the selected provider.
// Aliyun derives its Opus input rate per recording (zero until parsed).
// Volcengine requires a 16 kHz request; Tencent currently accepts PCM16LE.
func ASRInputFormat() audio.Format { return InputFormat(os.Getenv) }

// InputFormat derives the codec from the same snapshot as NewASR.
func InputFormat(get func(string) string) audio.Format {
	switch strings.ToLower(get("PERSONAOS_ASR_PROVIDER")) {
	case providerAliyun:
		return audio.Format{Encoding: audio.EncodingOpus, Channels: 1}
	case providerTencent:
		return audio.Format{Encoding: audio.EncodingPCM16LE, SampleRate: 16000, Channels: 1}
	}
	return audio.Format{Encoding: audio.EncodingOpus, SampleRate: 16000, Channels: 1}
}

// NewTTSFromEnv selects credentials for the provider recorded in the applied
// voice resource. The voice ID itself is supplied by that resource.
func NewTTSFromEnv(provider string) (tts.Client, error) { return NewTTS(os.Getenv, provider) }

// NewTTS selects the voice provider using server-side effective configuration.
func NewTTS(get func(string) string, provider string) (tts.Client, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case providerVolcengine, domain.Seed3CloneProvider:
		return speech.NewTTS(speech.VolcengineTTS{
			APIKey: get("VOLCENGINE_API_KEY"), ModelID: get("VOLCENGINE_TTS_MODEL_ID"),
		})
	case providerAliyun:
		return speech.NewTTS(speech.AliyunTTS{
			APIKey:   get("DASHSCOPE_API_KEY"),
			Endpoint: os.Getenv("DASHSCOPE_WS_ENDPOINT"),
		})
	case providerTencent:
		return speech.NewTTS(speech.TencentTTS{
			AppID: get("TENCENT_APP_ID"), SecretID: get("TENCENT_SECRET_ID"),
			SecretKey: get("TENCENT_SECRET_KEY"),
		})
	default:
		return nil, errors.New("unsupported configured voice provider")
	}
}
