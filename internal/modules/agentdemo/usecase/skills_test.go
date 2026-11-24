package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

func TestSkillBackendRespectsPublishedResources(t *testing.T) {
	ctx := context.Background()
	enabled := domain.Resource{Kind: domain.ResourceSkill, Reference: storytellingSkillName, Enabled: true}
	backend := resolveDemoSkills([]domain.Resource{
		{Kind: domain.ResourceSkill, Reference: storytellingSkillName, Enabled: false},
		{Kind: domain.ResourceTool, Reference: storytellingSkillName, Enabled: true},
		{Kind: domain.ResourceSkill, Reference: "../private", Enabled: true},
	})
	metadata, err := backend.List(ctx)
	if err != nil || len(metadata) != 0 {
		t.Fatalf("unapproved skills=%v err=%v", metadata, err)
	}
	if _, err := backend.Get(ctx, storytellingSkillName); err == nil {
		t.Fatal("unapproved skill was loaded")
	}
	backend = resolveDemoSkills([]domain.Resource{enabled, enabled})
	metadata, err = backend.List(ctx)
	if err != nil || len(metadata) != 1 || metadata[0].Name != storytellingSkillName {
		t.Fatalf("metadata=%v err=%v", metadata, err)
	}
	entry, err := backend.Get(ctx, storytellingSkillName)
	if err != nil || entry.Content != storytellingInstructions || entry.Context != "" {
		t.Fatalf("unexpected installed skill=%+v err=%v", entry, err)
	}
}

type skillCallingModel struct {
	loaded bool
}

func (m *skillCallingModel) Generate(_ context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	last := messages[len(messages)-1]
	if last.Role == schema.Tool {
		m.loaded = strings.Contains(last.Content, storytellingInstructions[:30])
		return schema.AssistantMessage("从前有一只小兔子。", nil), nil
	}
	for _, info := range model.GetCommonOptions(nil, opts...).Tools {
		if info.Name == "skill" && strings.Contains(info.Desc, storytellingSkillName) {
			return schema.AssistantMessage("", []schema.ToolCall{{
				ID: "skill-call", Type: testToolCallType,
				Function: schema.FunctionCall{Name: "skill", Arguments: "{\"skill\":\"storytelling\"}"},
			}}), nil
		}
	}
	return schema.AssistantMessage("没有可用技能。", nil), nil
}

func (m *skillCallingModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func TestAgentLoadsPublishedSkillInTextAndVoice(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		active := domain.PublishedDeviceConfig{
			Character: domain.Character{Name: "故事角色", Persona: "自然讲故事"},
			Skills:    []domain.Resource{{Kind: domain.ResourceSkill, Reference: storytellingSkillName, Enabled: true}},
		}
		m := &skillCallingModel{}
		u := NewChatUsecase(&fakeConfigReader{value: active}, m)
		var spoken strings.Builder
		var emit func(string) error
		if streaming {
			emit = func(delta string) error {
				spoken.WriteString(delta)
				return nil
			}
		}
		reply, _, err := u.runChat(context.Background(), ChatInput{DeviceTypeID: 7, Message: "讲个故事"}, active, emit)
		if err != nil || !m.loaded || reply != "从前有一只小兔子。" {
			t.Fatalf("stream=%t loaded=%t reply=%q err=%v", streaming, m.loaded, reply, err)
		}
		if streaming && spoken.String() != reply {
			t.Fatalf("spoken=%q reply=%q", spoken.String(), reply)
		}
	}
}
