package usecase

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const (
	streamTestCharacter = "Demo"
	streamTestMessage   = "你好"
)

type incrementalModel struct {
	fakeChatModel
	release <-chan struct{}
	started chan struct{}
}

func (m *incrementalModel) Stream(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if m.started != nil {
		close(m.started)
	}
	reader, writer := schema.Pipe[*schema.Message](1)
	go func() {
		defer writer.Close()
		if writer.Send(schema.AssistantMessage("你好！", nil), nil) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-m.release:
			writer.Send(schema.AssistantMessage("欢迎你。", nil), nil)
		}
	}()
	return reader, nil
}

type duplexClientStub struct {
	fakeSynthesizer
	stream   *duplexStreamStub
	openWait <-chan struct{}
}

func (c *duplexClientStub) OpenDuplex(ctx context.Context, req tts.DuplexRequest) (tts.DuplexStream, error) {
	if c.openWait != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.openWait:
		}
	}
	c.stream.format = req.Format
	return c.stream, nil
}

type duplexStreamStub struct {
	format audio.Format
	chunks chan string
}

func (s *duplexStreamStub) SendText(ctx context.Context, text string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.chunks <- text:
		return nil
	}
}

func (s *duplexStreamStub) CloseSend(context.Context) error {
	close(s.chunks)
	return nil
}

func (s *duplexStreamStub) Recv(ctx context.Context) (audio.Chunk, error) {
	select {
	case <-ctx.Done():
		return audio.Chunk{}, ctx.Err()
	case text, ok := <-s.chunks:
		if !ok {
			return audio.Chunk{}, io.EOF
		}
		return audio.Chunk{Format: s.format, Data: []byte(text)}, nil
	}
}

func (s *duplexStreamStub) Close() error { return nil }

func TestVoiceReplyOverlapsHandshakeAndEmitsAudioBeforeModelCompletes(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	config := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Character: domain.Character{ID: 9, Name: streamTestCharacter, Persona: "Friendly"},
		Voice:     &domain.Resource{Provider: "aliyun", Reference: "voice", Enabled: true},
	}}
	chat := NewChatUsecase(config, &incrementalModel{release: release, started: started})
	stream := &duplexStreamStub{chunks: make(chan string)}
	voice := NewVoiceUsecase(config, nil, func(string) (tts.Client, error) {
		return &duplexClientStub{stream: stream, openWait: started}, nil
	}, audio.Format{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var chunks []string
	out, err := voice.StreamReply(ctx, chat, ChatInput{DeviceTypeID: 7, Message: streamTestMessage}, func(chunk audio.Chunk) error {
		chunks = append(chunks, string(chunk.Data))
		if len(chunks) == 1 {
			// The model cannot finish until the first audio chunk is delivered.
			close(release)
		}
		return nil
	})
	if err != nil || out.Reply != "你好！欢迎你。" || len(chunks) != 2 {
		t.Fatalf("streaming reply=%+v chunks=%v err=%v", out, chunks, err)
	}
}

func TestVoiceReplyCancelsWhenAudioConsumerFails(t *testing.T) {
	config := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Character: domain.Character{ID: 9, Name: streamTestCharacter, Persona: "Friendly"},
		Voice:     &domain.Resource{Provider: "aliyun", Reference: "voice", Enabled: true},
	}}
	chat := NewChatUsecase(config, &incrementalModel{release: make(chan struct{})})
	voice := NewVoiceUsecase(config, nil, func(string) (tts.Client, error) {
		return &duplexClientStub{stream: &duplexStreamStub{chunks: make(chan string)}}, nil
	}, audio.Format{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	expected := errors.New("client disconnected")
	_, err := voice.StreamReply(ctx, chat, ChatInput{DeviceTypeID: 7, Message: streamTestMessage}, func(audio.Chunk) error {
		return expected
	})
	if !errors.Is(err, expected) {
		t.Fatalf("error=%v, want %v", err, expected)
	}
}
