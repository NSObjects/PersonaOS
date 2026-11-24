package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const testToolCallType = "function"

func TestResolveDemoToolsOnlyExposesEnabledRegisteredResources(t *testing.T) {
	resources := make([]domain.Resource, 0, 5)
	resources = append(resources, []domain.Resource{
		{Kind: domain.ResourceTool, Reference: currentTimeToolName, Enabled: false},
		{Kind: domain.ResourceTool, Reference: "unregistered", Enabled: true},
		{Kind: domain.ResourceSkill, Reference: currentTimeToolName, Enabled: true},
	}...)
	registry := NewToolRegistry(nil)
	if got, err := registry.Resolve(context.Background(), resources); err != nil || len(got) != 0 {
		t.Fatalf("unapproved tools exposed: %v", got)
	}
	enabled := domain.Resource{Kind: domain.ResourceTool, Reference: currentTimeToolName, Enabled: true}
	resources = append(resources, enabled, enabled)
	if got, err := registry.Resolve(context.Background(), resources); err != nil || len(got) != 1 {
		t.Fatalf("tool count=%d, want 1", len(got))
	}
}

type toolCallingModel struct {
	sawResult bool
}

func (m *toolCallingModel) Generate(_ context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	last := messages[len(messages)-1]
	if last.Role == schema.Tool {
		var result struct {
			UTC string `json:"utc"`
		}
		if err := json.Unmarshal([]byte(last.Content), &result); err != nil {
			return nil, err
		}
		if _, err := time.Parse(time.RFC3339, result.UTC); err != nil {
			return nil, err
		}
		m.sawResult = true
		return schema.AssistantMessage("现在的 UTC 时间是 "+result.UTC, nil), nil
	}
	options := model.GetCommonOptions(nil, opts...)
	if len(options.Tools) == 1 && options.Tools[0].Name == currentTimeToolName {
		return schema.AssistantMessage("", []schema.ToolCall{{
			ID: "time-call", Type: testToolCallType,
			Function: schema.FunctionCall{Name: currentTimeToolName, Arguments: "{}"},
		}}), nil
	}
	return schema.AssistantMessage("当前没有可用时间工具。", nil), nil
}

func (m *toolCallingModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func TestAgentExecutesPublishedToolAndReturnsResult(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		active := domain.PublishedDeviceConfig{
			Character: domain.Character{Name: "角色", Persona: "自然回答"},
			Tools:     []domain.Resource{{Kind: domain.ResourceTool, Reference: currentTimeToolName, Enabled: true}},
		}
		chatModel := &toolCallingModel{}
		u := NewChatUsecase(&fakeConfigReader{value: active}, chatModel)
		var emitted strings.Builder
		var emit func(string) error
		if streaming {
			emit = func(delta string) error {
				emitted.WriteString(delta)
				return nil
			}
		}
		reply, _, err := u.runChat(context.Background(), ChatInput{DeviceTypeID: 7, Message: "现在几点？"}, active, emit)
		if err != nil || !chatModel.sawResult || !strings.HasPrefix(reply, "现在的 UTC 时间是 ") {
			t.Fatalf("stream=%t reply=%q tool=%t err=%v", streaming, reply, chatModel.sawResult, err)
		}
		if streaming && emitted.String() != reply {
			t.Fatalf("spoken=%q reply=%q", emitted.String(), reply)
		}
	}
}
