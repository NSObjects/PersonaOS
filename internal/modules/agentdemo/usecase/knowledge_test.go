package usecase

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const (
	knowledgeFixtureKey  = "lore-v1"
	knowledgeFixtureFact = "月光徽章的颜色是琥珀金。"
)

type knowledgeRetrieverStub struct {
	versions []string
	query    string
}

func (s *knowledgeRetrieverStub) Retrieve(_ context.Context, versions []string, query string) ([]*schema.Document, error) {
	s.versions = slices.Clone(versions)
	s.query = query
	return []*schema.Document{{ID: knowledgeFixtureKey, Content: knowledgeFixtureFact}}, nil
}

func TestKnowledgeRetrieverScopesPublishedCorpora(t *testing.T) {
	stub := &knowledgeRetrieverStub{}
	active := []domain.Resource{
		{Kind: domain.ResourceKnowledgeBase, Provider: knowledgeProvider, Reference: knowledgeFixtureKey, Enabled: true},
		{Kind: domain.ResourceKnowledgeBase, Provider: knowledgeProvider, Reference: "private", Enabled: false},
		{Kind: domain.ResourceKnowledgeBase, Provider: knowledgeProvider, Reference: knowledgeFixtureKey, Enabled: true},
	}
	entry, err := newKnowledgeTool(stub, active)
	if err != nil {
		t.Fatal(err)
	}
	search, ok := entry.(knowledgeSearchTool)
	if !ok {
		t.Fatal("knowledge search tool missing")
	}
	got, err := search.InvokableRun(context.Background(), `{"query":"月光徽章","versions":["private"]}`)
	if err != nil || !strings.Contains(got, knowledgeFixtureFact) {
		t.Fatalf("result=%q err=%v", got, err)
	}
	if !slices.Equal(stub.versions, []string{knowledgeFixtureKey}) || stub.query != "月光徽章" {
		t.Fatalf("unexpected retrieval scope=%v query=%q", stub.versions, stub.query)
	}
}

func TestKnowledgeRejectsPathAndMissingConfiguration(t *testing.T) {
	active := []domain.Resource{{Kind: domain.ResourceKnowledgeBase, Provider: knowledgeProvider, Reference: "../private", Enabled: true}}
	if _, err := newKnowledgeTool(&knowledgeRetrieverStub{}, active); err == nil {
		t.Fatal("path traversal accepted")
	}
	active[0].Reference = knowledgeFixtureKey
	if _, err := newKnowledgeTool(nil, active); err == nil {
		t.Fatal("missing Redis knowledge configuration accepted")
	}
	active[0].Enabled = false
	if entry, err := newKnowledgeTool(nil, active); err != nil || entry != nil {
		t.Fatalf("disabled knowledge tool=%v err=%v", entry, err)
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
				ID: "knowledge-call", Type: testToolCallType,
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
	for _, streaming := range []bool{false, true} {
		active := domain.PublishedDeviceConfig{
			Character:      domain.Character{Name: testCharacterName, Persona: "依据事实回答"},
			KnowledgeBases: []domain.Resource{{Kind: domain.ResourceKnowledgeBase, Provider: knowledgeProvider, Reference: knowledgeFixtureKey, Enabled: true}},
		}
		m := &knowledgeCallingModel{}
		u := NewChatUsecase(&fakeConfigReader{value: active}, m).WithKnowledgeRetriever(&knowledgeRetrieverStub{})
		var spoken strings.Builder
		var emit func(string) error
		if streaming {
			emit = func(delta string) error {
				spoken.WriteString(delta)
				return nil
			}
		}
		reply, _, err := u.runChat(context.Background(), ChatInput{DeviceTypeID: 7, Message: "月光徽章是什么颜色？"}, active, emit, &behaviorBuffer{})
		if err != nil || !m.grounded || reply != knowledgeFixtureFact {
			t.Fatalf("stream=%t grounded=%t reply=%q err=%v", streaming, m.grounded, reply, err)
		}
		if streaming && spoken.String() != reply {
			t.Fatalf("spoken=%q reply=%q", spoken.String(), reply)
		}
	}
}
