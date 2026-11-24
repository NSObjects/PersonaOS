package usecase

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const knowledgeFixtureKey = "lore-v1"
const knowledgeFixtureFact = "月光徽章的颜色是琥珀金。"

func TestKnowledgeRetrieverScopesPublishedCorpora(t *testing.T) {
	dir := t.TempDir()
	for key, content := range map[string]string{
		knowledgeFixtureKey: knowledgeFixtureFact + "\n\n其他资料：森林里有三条路。",
		"private": "月光徽章的颜色是秘密紫。",
	} {
		if err := os.WriteFile(filepath.Join(dir, key+".txt"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	active := []domain.Resource{
		{Kind: domain.ResourceKnowledgeBase, Provider: "local", Reference: knowledgeFixtureKey, Enabled: true},
		{Kind: domain.ResourceKnowledgeBase, Provider: "local", Reference: "private", Enabled: false},
	}
	tool, err := newKnowledgeTool(dir, active)
	if err != nil {
		t.Fatal(err)
	}
	search, ok := tool.(knowledgeSearchTool)
	if !ok {
		t.Fatal("knowledge search tool missing")
	}
	got, err := search.InvokableRun(context.Background(), `{"query":"月光徽章"}`)
	if err != nil || !strings.Contains(got, knowledgeFixtureFact) || strings.Contains(got, "秘密紫") {
		t.Fatalf("result=%q err=%v", got, err)
	}
	got, err = search.InvokableRun(context.Background(), `{"query":"quantum"}`)
	if err != nil || got != "[]" {
		t.Fatalf("unmatched query result=%q err=%v", got, err)
	}
}

func TestKnowledgeRejectsPathAndMissingConfiguration(t *testing.T) {
	active := []domain.Resource{{Kind: domain.ResourceKnowledgeBase, Provider: "local", Reference: "../private", Enabled: true}}
	if _, err := newKnowledgeTool(t.TempDir(), active); err == nil {
		t.Fatal("path traversal accepted")
	}
	active[0].Reference = knowledgeFixtureKey
	if _, err := newKnowledgeTool("", active); err == nil {
		t.Fatal("missing knowledge directory accepted")
	}
}

type knowledgeCallingModel struct {
	grounded bool
}

func (m *knowledgeCallingModel) Generate(_ context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	last := messages[len(messages)-1]
	if last.Role == schema.Tool {
		m.grounded = strings.Contains(last.Content, knowledgeFixtureFact)
		return schema.AssistantMessage(knowledgeFixtureFact, nil), nil
	}
	for _, info := range model.GetCommonOptions(nil, opts...).Tools {
		if info.Name == knowledgeToolName {
			return schema.AssistantMessage("", []schema.ToolCall{{
				ID: "knowledge-call", Type: "function",
				Function: schema.FunctionCall{Name: knowledgeToolName, Arguments: `{"query":"月光徽章"}`},
			}}), nil
		}
	}
	return schema.AssistantMessage("没有知识库。", nil), nil
}

func (m *knowledgeCallingModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func TestAgentRetrievesKnowledgeInTextAndVoice(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, knowledgeFixtureKey+".txt"), []byte(knowledgeFixtureFact), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, streaming := range []bool{false, true} {
		active := domain.PublishedDeviceConfig{
			Character:      domain.Character{Name: "角色", Persona: "依据事实回答"},
			KnowledgeBases: []domain.Resource{{Kind: domain.ResourceKnowledgeBase, Provider: "local", Reference: knowledgeFixtureKey, Enabled: true}},
		}
		m := &knowledgeCallingModel{}
		u := NewChatUsecase(&fakeConfigReader{value: active}, m).WithKnowledgeDir(dir)
		var spoken strings.Builder
		var emit func(string) error
		if streaming {
			emit = func(delta string) error {
				spoken.WriteString(delta)
				return nil
			}
		}
		reply, _, err := u.runChat(context.Background(), ChatInput{DeviceTypeID: 7, Message: "月光徽章是什么颜色？"}, active, emit)
		if err != nil || !m.grounded || reply != knowledgeFixtureFact {
			t.Fatalf("stream=%t grounded=%t reply=%q err=%v", streaming, m.grounded, reply, err)
		}
		if streaming && spoken.String() != reply {
			t.Fatalf("spoken=%q reply=%q", spoken.String(), reply)
		}
	}
}
