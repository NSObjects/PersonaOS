package providerconfig

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestProviderProjectionAndExplicitChanges(t *testing.T) {
	service, ok := Lookup("llm")
	if !ok {
		t.Fatal("missing LLM definition")
	}
	current := Values{FieldAPIKey: "secret-not-for-browser", "model": "old-model"}
	next, err := service.Merge(current, Values{"model": "new-model"})
	if err != nil || next[FieldAPIKey] != current[FieldAPIKey] || current["model"] != "old-model" {
		t.Fatal("omitted credential was not retained")
	}
	view := service.Project(next, "database")
	raw, err := json.Marshal(view)
	if err != nil || bytes.Contains(raw, []byte(current[FieldAPIKey])) || !view.Secrets[FieldAPIKey] || view.Values["model"] != "new-model" {
		t.Fatal("invalid redacted view")
	}
	if _, err := service.Merge(next, Values{FieldAPIKey: ""}); err == nil {
		t.Fatal("incomplete LLM configuration accepted")
	}
	cleared, err := service.Merge(next, Values{FieldAPIKey: "", "model": ""})
	if err != nil || service.Project(cleared, "database").Secrets[FieldAPIKey] {
		t.Fatal("explicit clear failed")
	}
	if _, err := service.Merge(next, Values{"base_url": "https://user:password@example.com"}); err == nil {
		t.Fatal("credentials in URL accepted")
	}
	aliyun, _ := Lookup("aliyun")
	if _, err := aliyun.Merge(nil, Values{"endpoint": "wss://example.com"}); err == nil {
		t.Fatal("speech endpoint override accepted")
	}
	asr, _ := Lookup("asr")
	if _, err := asr.Merge(nil, Values{"provider": "unknown"}); err == nil {
		t.Fatal("unsupported ASR provider accepted")
	}
}

const testSpeechSecret = "speech-secret"

func TestVolcengineGroupsDistinctCredentials(t *testing.T) {
	service, ok := Lookup("volcengine")
	if !ok {
		t.Fatal("missing Volcengine service")
	}
	values := service.Environment(func(key string) string {
		switch key {
		case "VOLCENGINE_API_KEY":
			return testSpeechSecret
		case "PERSONAOS_EMBEDDING_API_KEY":
			return "ark-secret"
		default:
			return ""
		}
	})
	if values["speech_api_key"] != testSpeechSecret || values["ark_api_key"] != "ark-secret" {
		t.Fatal("Volcengine credentials must remain independent")
	}
	next, err := service.Merge(values, Values{"ark_api_key": "new-ark-secret"})
	if err != nil || next["speech_api_key"] != testSpeechSecret {
		t.Fatal("Ark update replaced speech credentials")
	}
	view := service.Project(next, "database")
	if !view.Secrets["speech_api_key"] || !view.Secrets["ark_api_key"] || len(view.Values) != 2 {
		t.Fatal("incorrect Volcengine projection")
	}
	if _, ok := Lookup("embedding"); ok {
		t.Fatal("embedding must be grouped under Volcengine")
	}
	if _, ok := Lookup("deepseek"); !ok {
		t.Fatal("missing DeepSeek provider")
	}
}
