package usecase

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/NSObjects/go-speech/asr"
	"github.com/NSObjects/go-speech/audio"
)

func TestLiveWebMRemux(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "anullsrc=channel_layout=mono:sample_rate=48000",
		"-t", "1", "-c:a", "libopus", "-b:a", "24k", "-live", "1", "-f", "webm", "pipe:1")
	webm, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	stream := &fakeRecognitionStream{events: []asr.Event{{Type: asr.EventFinal, Text: "test transcript"}}}
	voice := NewVoiceUsecase(nil, &fakeRecognizer{stream: stream, want: audio.EncodingOpus},
		nil, audio.Format{Encoding: audio.EncodingOpus, Channels: 1})
	got, err := voice.RecognizeLiveCapture(ctx, io.NopCloser(bytes.NewReader(webm)), "webm")
	if err != nil || got != "test transcript" || !stream.ended || stream.sent == 0 {
		t.Fatalf("live WebM transcript=%q bytes=%d ended=%t err=%v", got, stream.sent, stream.ended, err)
	}
}
