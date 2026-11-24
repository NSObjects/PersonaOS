package usecase

import (
	"context"
	"errors"
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
	run := h.OnStart(ctx, info, []*schema.Message{schema.UserMessage("hello")})
	output := schema.StreamReaderFromArray([]callbacks.CallbackOutput{
		&model.CallbackOutput{Message: schema.AssistantMessage("hello", nil)},
		&model.CallbackOutput{Message: schema.AssistantMessage(" world", nil), TokenUsage: &model.TokenUsage{TotalTokens: 5}},
	})
	h.OnEndWithStreamOutput(run, info, output)
	h.streams.Wait()
	if len(events) != 2 || events[1].Output != "hello world" || events[1].Usage == nil || events[1].Usage.TotalTokens != 5 {
		t.Fatalf("events=%+v, want complete streamed response and usage", events)
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
		Character: domain.Character{Name: "Demo", Persona: "Friendly"},
	}}
	_, err := NewChatUsecase(config, &fakeChatModel{}).Chat(ctx, ChatInput{DeviceTypeID: 1, Message: "hello"})
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
