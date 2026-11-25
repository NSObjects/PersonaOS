package usecase

import (
	"strings"
	"testing"
)

func TestSpokenTextAcrossTokenBoundaries(t *testing.T) {
	const raw = "## 今日天气\n- **上海**：晴天。\n1. _建议_：带伞。\n查看[天气](https://example.com/a(b))，温度 23.5°C。"
	const want = "今日天气\n上海：晴天。\n建议：带伞。\n查看天气，温度 23.5°C。"
	if got := spokenText(raw); got != want {
		t.Fatalf("batch spoken text = %q, want %q", got, want)
	}
	filter := newSpeechTextFilter()
	var out strings.Builder
	for _, r := range raw {
		out.WriteString(filter.Write(string(r)))
	}
	out.WriteString(filter.Finish())
	if got := out.String(); got != want {
		t.Fatalf("stream spoken text = %q, want %q", got, want)
	}
}

func TestSpokenTextDoesNotBufferOrdinaryText(t *testing.T) {
	filter := newSpeechTextFilter()
	if got := filter.Write("**你好"); got != streamTestMessage {
		t.Fatalf("first spoken fragment = %q", got)
	}
	if got := filter.Write("**，欢迎你。"); got != "，欢迎你。" {
		t.Fatalf("second spoken fragment = %q", got)
	}
	if got := filter.Finish(); got != "" {
		t.Fatalf("unexpected tail = %q", got)
	}
}

func TestSpokenTextPreservesNumbersAndPunctuation(t *testing.T) {
	const text = "3.14，零下 -5°C，今天很好！"
	if got := spokenText(text); got != text {
		t.Fatalf("spoken text = %q, want %q", got, text)
	}
	if got := spokenText("***"); got != "" {
		t.Fatalf("formatting-only text = %q", got)
	}
}
