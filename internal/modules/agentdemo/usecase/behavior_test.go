package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const (
	testWave      = "wave"
	testSmile     = "smile"
	behaviorReply = "你好呀！"
)

func TestBehaviorToolValidatesAndBoundsEvents(t *testing.T) {
	ctx := context.Background()
	buffer := &behaviorBuffer{}
	entry, ok := newBehaviorTool(actionToolName, buffer).(tool.InvokableTool)
	if !ok {
		t.Fatal("behavior tool must be invokable")
	}
	for _, args := range []string{`{"code":"unknown"}`, `{"code":"wave","extra":"unexpected"}`, `{}`} {
		got, err := entry.InvokableRun(ctx, args)
		if err != nil || got != behaviorFeedback || len(buffer.snapshot()) != 0 {
			t.Fatalf("invalid behavior accepted: %q, %v", got, err)
		}
	}
	for i := 0; i < maxBehaviorEvents; i++ {
		got, err := entry.InvokableRun(ctx, `{"code":"wave"}`)
		if err != nil || !strings.Contains(got, previewStatus) {
			t.Fatalf("preview result=%q err=%v", got, err)
		}
	}
	got, err := entry.InvokableRun(ctx, `{"code":"wave"}`)
	if err != nil || !strings.Contains(got, "preview_limit") || len(buffer.snapshot()) != maxBehaviorEvents {
		t.Fatalf("preview limit=%q err=%v", got, err)
	}
}

type behaviorCallingModel struct{}

func (behaviorCallingModel) Generate(_ context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if last := messages[len(messages)-1]; last.Role == schema.Tool {
		return schema.AssistantMessage(behaviorReply, nil), nil
	}
	calls := make([]schema.ToolCall, 0, 2)
	for _, info := range model.GetCommonOptions(nil, opts...).Tools {
		var code string
		switch info.Name {
		case actionToolName:
			code = testWave
		case expressionToolName:
			code = testSmile
		default:
			continue
		}
		args, err := json.Marshal(map[string]string{behaviorCodeParam: code})
		if err != nil {
			return nil, err
		}
		calls = append(calls, schema.ToolCall{
			ID: info.Name, Type: testToolCallType,
			Function: schema.FunctionCall{Name: info.Name, Arguments: string(args)},
		})
	}
	if len(calls) == 0 {
		return schema.AssistantMessage(behaviorReply, nil), nil
	}
	return schema.AssistantMessage("", calls), nil
}

func (m behaviorCallingModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func behaviorConfig() domain.PublishedDeviceConfig {
	return domain.PublishedDeviceConfig{
		Character: domain.Character{Name: streamTestCharacter, Persona: streamTestPersona},
		Voice:     &domain.Resource{Provider: "aliyun", Reference: "voice", Enabled: true},
		Tools: []domain.Resource{
			{Kind: domain.ResourceTool, Reference: actionToolName, Enabled: true},
			{Kind: domain.ResourceTool, Reference: expressionToolName, Enabled: true},
		},
	}
}

func assertBehaviorReply(t *testing.T, out ChatOutput, err error) {
	t.Helper()
	if err != nil || out.Reply != behaviorReply || len(out.Behaviors) != 2 {
		t.Fatalf("reply=%+v err=%v", out, err)
	}
	seen := make(map[string]string)
	for _, event := range out.Behaviors {
		seen[event.Kind] = event.Code
	}
	if seen[actionKind] != testWave || seen[expressionKind] != testSmile {
		t.Fatalf("unexpected behavior events: %+v", out.Behaviors)
	}
}

func TestBehaviorEventsUsePublishedToolsOnly(t *testing.T) {
	config := &fakeConfigReader{value: behaviorConfig()}
	chat := NewChatUsecase(config, behaviorCallingModel{})
	in := ChatInput{DeviceTypeID: 7, Message: streamTestMessage}
	out, err := chat.Chat(context.Background(), in)
	assertBehaviorReply(t, out, err)
	config.value.Tools[0].Enabled = false
	config.value.Tools[1].Enabled = false
	out, err = chat.Chat(context.Background(), in)
	if err != nil || len(out.Behaviors) != 0 {
		t.Fatalf("disabled behaviors or leaked previous events: %+v %v", out, err)
	}
}

func TestVoiceBehaviorEventsAreNotSpoken(t *testing.T) {
	config := &fakeConfigReader{value: behaviorConfig()}
	chat := NewChatUsecase(config, behaviorCallingModel{})
	voice := NewVoiceUsecase(config, nil, func(string) (tts.Client, error) {
		return &duplexClientStub{stream: &duplexStreamStub{chunks: make(chan string)}}, nil
	}, audio.Format{})
	var spoken strings.Builder
	out, err := voice.StreamReply(context.Background(), chat, ChatInput{DeviceTypeID: 7, Message: streamTestMessage}, func(chunk audio.Chunk) error {
		spoken.Write(chunk.Data)
		return nil
	})
	assertBehaviorReply(t, out, err)
	if spoken.String() != behaviorReply {
		t.Fatalf("spoken=%q, want %q", spoken.String(), behaviorReply)
	}
}
