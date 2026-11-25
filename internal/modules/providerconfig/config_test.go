package providerconfig

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func testCipher(t *testing.T, fill byte) *Cipher {
	t.Helper()
	c, err := NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCipherAuthentication(t *testing.T) {
	c := testCipher(t, 1)
	values := Values{"api_key": "test-secret", "model": "test-model"}
	first, err := c.Seal("llm", values)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Seal("llm", values)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) || bytes.Contains(first, []byte(values["api_key"])) {
		t.Fatal("ciphertext must use a fresh nonce and conceal credentials")
	}
	got, err := c.Open("llm", first)
	if err != nil || got["api_key"] != values["api_key"] || got["model"] != values["model"] {
		t.Fatal("encrypted snapshot did not round trip")
	}
	if _, err := c.Open("aliyun", first); err == nil {
		t.Fatal("ciphertext accepted for another service")
	}
	if _, err := testCipher(t, 2).Open("llm", first); err == nil {
		t.Fatal("ciphertext accepted with wrong key")
	}
	first[len(first)-1] ^= 1
	if _, err := c.Open("llm", first); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	if _, err := NewCipher("invalid"); err == nil {
		t.Fatal("invalid deployment key accepted")
	}
}

func TestProviderProjectionAndExplicitChanges(t *testing.T) {
	service, ok := Lookup("llm")
	if !ok {
		t.Fatal("missing LLM definition")
	}
	current := Values{"api_key": "secret-not-for-browser", "model": "old-model"}
	next, err := service.Merge(current, Values{"model": "new-model"})
	if err != nil || next["api_key"] != current["api_key"] || current["model"] != "old-model" {
		t.Fatal("omitted credential was not retained")
	}
	view := service.Project(next, "database")
	raw, err := json.Marshal(view)
	if err != nil || bytes.Contains(raw, []byte(current["api_key"])) || !view.Secrets["api_key"] || view.Values["model"] != "new-model" {
		t.Fatal("invalid redacted view")
	}
	if _, err := service.Merge(next, Values{"api_key": ""}); err == nil {
		t.Fatal("incomplete LLM configuration accepted")
	}
	cleared, err := service.Merge(next, Values{"api_key": "", "model": ""})
	if err != nil || service.Project(cleared, "database").Secrets["api_key"] {
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
