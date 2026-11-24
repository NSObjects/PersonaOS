package voiceconfig

import "testing"

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
