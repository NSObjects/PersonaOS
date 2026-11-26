package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

func TestExecutionCallbacksPairInvocations(t *testing.T) {
	var events []ExecutionEvent
	ctx := WithExecutionObserver(context.Background(), func(event ExecutionEvent) { events = append(events, event) })
	h := &executionCallbacks{observer: observerFrom(ctx)}
	info := &callbacks.RunInfo{Name: "query", Component: components.ComponentOfTool}
	first := h.OnStart(ctx, info, "first input")
	second := h.OnStart(ctx, info, "second input")
	h.OnEnd(second, info, "second result")
	h.OnError(first, info, errors.New("secret provider detail"))
	if len(events) != 4 || events[0].ID == events[1].ID || events[1].ID != events[2].ID || events[0].ID != events[3].ID {
		t.Fatalf("events=%+v, want paired concurrent invocations", events)
	}
	if events[2].Output != "second result" || events[3].Error != "execution failed" {
		t.Fatalf("events=%+v, want result and sanitized error", events)
	}
}

func TestExecutionCallbacksStream(t *testing.T) {
	var events []ExecutionEvent
	ctx := WithExecutionObserver(context.Background(), func(event ExecutionEvent) { events = append(events, event) })
	h := &executionCallbacks{observer: observerFrom(ctx)}
	info := &callbacks.RunInfo{Type: "fake", Component: components.ComponentOfChatModel}
	ObserveConfiguredModelOptions(ctx, map[string]any{
		"enable_thinking": false, "reasoning_effort": "low", "private_key": "secret",
	})
	choice := schema.ToolChoiceAllowed
	run := h.OnStart(ctx, info, &model.CallbackInput{
		Messages: []*schema.Message{schema.UserMessage("stream input")},
		Tools: []*schema.ToolInfo{{Name: "get_weather", Desc: "Get city weather"}},
		ToolChoice: &choice,
	})
	output := schema.StreamReaderFromArray([]callbacks.CallbackOutput{
		&model.CallbackOutput{Message: schema.AssistantMessage("stream", nil)},
		&model.CallbackOutput{
			Message: &schema.Message{Content: " world", ResponseMeta: &schema.ResponseMeta{FinishReason: "stop"}},
			TokenUsage: &model.TokenUsage{TotalTokens: 5, CompletionTokensDetails: model.CompletionTokensDetails{ReasoningTokens: 2}},
		},
		&model.CallbackOutput{Message: &schema.Message{ResponseMeta: &schema.ResponseMeta{}}},
	})
	h.OnEndWithStreamOutput(run, info, output)
	h.streams.Wait()
	if len(events) != 3 || events[1].Phase != executionFirst || events[1].ID != events[0].ID || events[2].Output != "stream world" || events[2].Usage == nil || events[2].Usage.TotalTokens != 5 {
		t.Fatalf("events=%+v, want complete streamed response and usage", events)
	}
	for _, expected := range []string{"1 messages, 1 tools", "get_weather", "Get city weather", "Eino tool_choice: allowed", "Configured enable_thinking: false", "Configured reasoning_effort: low"} {
		if !strings.Contains(events[0].Input, expected) {
			t.Fatalf("missing %q in %q", expected, events[0].Input)
		}
	}
	if strings.Contains(events[0].Input, "secret") || events[2].FinishReason != "stop" || events[2].Usage.CompletionTokensDetails.ReasoningTokens != 2 {
		t.Fatalf("incorrect model diagnostics: %+v", events)
	}
}

func TestChatPublishesNativeModelCallbacks(t *testing.T) {
	var mu sync.Mutex
	var events []ExecutionEvent
	ctx := WithExecutionObserver(context.Background(), func(event ExecutionEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	config := &fakeConfigReader{value: domain.PublishedDeviceConfig{
		Character: domain.Character{Name: streamTestCharacter, Persona: streamTestPersona},
	}}
	_, err := NewChatUsecase(config, &fakeChatModel{}).Chat(ctx, ChatInput{DeviceTypeID: 1, Message: streamTestMessage})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	var started, ended bool
	for _, event := range events {
		if event.Kind == executionModel {
			started = started || event.Phase == executionStart
			ended = ended || event.Phase == executionEnd
		}
	}
	if !started || !ended {
		t.Fatalf("events=%+v, want native model start and end", events)
	}
}
