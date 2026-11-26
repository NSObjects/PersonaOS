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
			voice, chat := feedbackTestRuntime(server.URL)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var spoken strings.Builder
			out, err := voice.StreamReply(ctx, chat, ChatInput{DeviceTypeID: 7, Message: "天气如何？"}, func(chunk audio.Chunk) error {
				if tc.slow && spoken.Len() == 0 {
					if string(chunk.Data) != feedbackTestWaiting {
						t.Errorf("first audio=%q", chunk.Data)
					}
					close(release)
				}
				spoken.Write(chunk.Data)
				return nil
			})
			want := httpFixtureReply
			if tc.fail {
				want = feedbackTestFailure
			}
			wantSpoken := want
			if tc.slow {
				wantSpoken = feedbackTestWaiting + want
			}
			if err != nil || out.Reply != want || spoken.String() != wantSpoken {
				t.Fatalf("reply=%q spoken=%q err=%v; want %q / %q", out.Reply, spoken.String(), err, want, wantSpoken)
			}
		})
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
	voice, chat := feedbackTestRuntime(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spoken strings.Builder
	_, err := voice.StreamReply(ctx, chat, ChatInput{DeviceTypeID: 7, Message: "天气如何？"}, func(chunk audio.Chunk) error {
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

func feedbackTestRuntime(endpoint string) (*VoiceUsecase, *ChatUsecase) {
	active := domain.PublishedDeviceConfig{
		Character: domain.Character{ID: 9, Name: streamTestCharacter, Persona: streamTestPersona},
		Voice:     &domain.Resource{Provider: "aliyun", Reference: "voice", Enabled: true},
		Tools: []domain.Resource{{
			Kind: domain.ResourceTool, Code: httpFixtureToolName, Provider: domain.HTTPToolProvider,
			Reference: httpFixtureEndpoint, Description: "查询天气", ToolConfig: httpFixtureConfig, Enabled: true,
		}},
	}
	config := &fakeConfigReader{value: active}
	chat := NewChatUsecase(config, &httpCallingModel{}).
		WithToolRegistry(NewToolRegistry(map[string]string{httpFixtureEndpoint: endpoint}))
	voice := NewVoiceUsecase(config, nil, func(string) (tts.Client, error) {
		return &duplexClientStub{stream: &duplexStreamStub{chunks: make(chan string)}}, nil
	}, audio.Format{})
	return voice, chat
}
