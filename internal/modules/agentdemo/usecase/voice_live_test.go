package usecase

import (
	"context"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/NSObjects/go-speech/asr"
	"github.com/NSObjects/go-speech/audio"
)

type liveRecognizer struct {
	stream *liveRecognitionStream
}

func (r liveRecognizer) OpenStream(_ context.Context, _ asr.Config) (asr.Stream, error) {
	return r.stream, nil
}

type liveRecognitionStream struct {
	fakeRecognitionStream
	pageSent chan struct{}
}

func (s *liveRecognitionStream) Send(ctx context.Context, data []byte) error {
	if err := s.fakeRecognitionStream.Send(ctx, data); err != nil {
		return err
	}
	select {
	case s.pageSent <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestLiveOpusSendsPagesBeforeInputEOF(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	header := make([]byte, 19)
	copy(header, "OpusHead")
	header[8], header[9] = 1, 1
	binary.LittleEndian.PutUint32(header[12:16], 24000)
	pages := [][]byte{
		makeTestOggPage(header, 0),
		makeTestOggPage([]byte("OpusTags"), 0),
		makeTestOggPage([]byte{1, 2}, 960),
	}
	stream := &liveRecognitionStream{pageSent: make(chan struct{}), fakeRecognitionStream: fakeRecognitionStream{
		events: []asr.Event{{Type: asr.EventFinal, Text: "直播识别"}},
	}}
	voice := NewVoiceUsecase(nil, liveRecognizer{stream: stream}, nil, audio.Format{Encoding: audio.EncodingOpus, Channels: 1})
	done := make(chan error, 1)
	go func() {
		defer writer.Close()
		for _, page := range pages {
			if _, err := writer.Write(page); err != nil {
				done <- err
				return
			}
			select {
			case <-stream.pageSent:
			case <-ctx.Done():
				done <- ctx.Err()
				return
			}
		}
		done <- writer.Close()
	}()
	got, err := voice.RecognizeLiveOpus(ctx, reader)
	if err != nil || got != "直播识别" || !stream.ended {
		t.Fatalf("live transcript=%q ended=%t err=%v", got, stream.ended, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
