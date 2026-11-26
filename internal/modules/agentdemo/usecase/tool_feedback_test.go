package usecase

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const (
	feedbackTestWaiting = "我查查。"
	feedbackTestFailure = "暂时查不到天气。"
)

func TestVoiceToolFeedbackSharesDuplexStream(t *testing.T) {
	for _, tc := range []struct {
		name string
		slow bool
		fail bool
	}{
		{name: "fast"},
		{name: "fast failure", fail: true},
		{name: "slow", slow: true},
		{name: "slow failure", slow: true, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.slow {
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				}
				if tc.fail {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				if _, err := w.Write([]byte(httpFixtureReply)); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			voice, chat, stream := feedbackTestRuntime(server.URL)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var spoken strings.Builder
			out, err := voice.StreamReply(ctx, chat, ChatInput{DeviceTypeID: 7, Message: httpFixtureQuestion}, func(chunk audio.Chunk) error {
				if tc.slow && spoken.Len() == 0 {
					if string(chunk.Data) != feedbackTestWaiting {
						t.Errorf("first audio=%q", chunk.Data)
					}
					close(release)
				}
				spoken.Write(chunk.Data)
				return nil
			})
			assertFeedbackVoice(t, out, err, spoken.String(), stream.flushes, tc.slow, tc.fail)
		})
	}
}

func assertFeedbackVoice(t *testing.T, out ChatOutput, err error, spoken string, flushes int, slow, failed bool) {
	t.Helper()
	want := httpFixtureReply
	if failed {
		want = feedbackTestFailure
	}
	wantSpoken, wantFlushes := want, 0
	if slow {
		wantSpoken, wantFlushes = feedbackTestWaiting+want, 1
	}
	if flushes != wantFlushes {
		t.Fatalf("waiting flushes=%d want=%d", flushes, wantFlushes)
	}
	if err != nil || out.Reply != want || spoken != wantSpoken {
		t.Fatalf("reply=%q spoken=%q err=%v; want %q / %q", out.Reply, spoken, err, want, wantSpoken)
	}
}

func TestVoiceToolFeedbackCancellationStopsTurn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(4 * time.Second):
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	voice, chat, _ := feedbackTestRuntime(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spoken strings.Builder
	_, err := voice.StreamReply(ctx, chat, ChatInput{DeviceTypeID: 7, Message: httpFixtureQuestion}, func(chunk audio.Chunk) error {
		spoken.Write(chunk.Data)
		cancel()
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) || spoken.String() != feedbackTestWaiting {
		t.Fatalf("canceled reply spoken=%q err=%v", spoken.String(), err)
	}
}

func TestToolFeedbackSuppressesAfterReplyAndDeduplicates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	sent := make(chan string, 2)
	state := &voiceToolFeedback{send: func(text string) error {
		sent <- text
		return nil
	}}
	ctx = context.WithValue(ctx, toolFeedbackKey{}, state)
	cfg := ToolFeedback{WaitingMessage: feedbackTestWaiting, WaitingDelay: time.Millisecond}
	stopFirst := beginToolFeedback(ctx, cfg)
	defer stopFirst()
	stopSecond := beginToolFeedback(ctx, cfg)
	defer stopSecond()
	select {
	case got := <-sent:
		if got != feedbackTestWaiting {
			t.Fatalf("feedback=%q", got)
		}
	case <-ctx.Done():
		t.Fatal("no feedback")
	}
	if err := state.reply(ctx, "reply", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	stopThird := beginToolFeedback(ctx, cfg)
	defer stopThird()
	if err := state.finish(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-sent:
		t.Fatalf("duplicate feedback=%q", got)
	default:
	}
}

type feedbackFlushStream struct {
	*duplexStreamStub
	flushes int
}

func (s *feedbackFlushStream) Flush(context.Context) error {
	s.flushes++
	return nil
}

type feedbackFlushClient struct {
	fakeSynthesizer
	stream *feedbackFlushStream
}

func (c *feedbackFlushClient) OpenDuplex(_ context.Context, req tts.DuplexRequest) (tts.DuplexStream, error) {
	c.stream.format = req.Format
	return c.stream, nil
}

func feedbackTestRuntime(endpoint string) (*VoiceUsecase, *ChatUsecase, *feedbackFlushStream) {
	active := domain.PublishedDeviceConfig{
		Character: domain.Character{ID: 9, Name: streamTestCharacter, Persona: streamTestPersona},
		Voice:     &domain.Resource{Provider: streamTestProvider, Reference: streamTestVoice, Enabled: true},
		Tools: []domain.Resource{{
			Kind: domain.ResourceTool, Code: httpFixtureToolName, Provider: domain.HTTPToolProvider,
			Reference: httpFixtureEndpoint, Description: httpFixtureDescription, ToolConfig: httpFixtureConfig, Enabled: true,
		}},
	}
	config := &fakeConfigReader{value: active}
	chat := NewChatUsecase(config, &httpCallingModel{}).
		WithToolRegistry(NewToolRegistry(map[string]string{httpFixtureEndpoint: endpoint}))
	stream := &feedbackFlushStream{duplexStreamStub: &duplexStreamStub{chunks: make(chan string)}}
	voice := NewVoiceUsecase(config, nil, func(string) (tts.Client, error) {
		return &feedbackFlushClient{stream: stream}, nil
	}, audio.Format{})
	return voice, chat, stream
}
