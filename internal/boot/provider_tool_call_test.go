package boot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	agentdemo "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/modules/providerconfig"
)

const (
	probeToolName   = "query_demo_badge"
	probeToolResult = "BT-7391"
	probeCallID     = "call_demo_badge"
)

type toolProbeConfig struct{}

func (toolProbeConfig) Get(context.Context, int64) (domain.PublishedDeviceConfig, error) {
	return domain.PublishedDeviceConfig{
		Character: domain.Character{ID: 1, Name: "测试角色", Persona: "自然回答"},
		Tools: []domain.Resource{{
			Kind: domain.ResourceTool, Code: probeToolName, Description: "查询徽章编号",
			Provider: domain.HTTPToolProvider, Reference: "badge", Enabled: true,
			ToolConfig: `{"method":"GET","input_schema":{"type":"object","properties":{}}}`,
		}},
	}, nil
}

type toolProbeRequest struct {
	Thinking   *bool           `json:"enable_thinking"`
	Stream     bool            `json:"stream"`
	ToolChoice json.RawMessage `json:"tool_choice"`
	Tools      []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tools"`
	Messages []struct {
		Role       string `json:"role"`
		Content    string `json:"content"`
		ToolCallID string `json:"tool_call_id"`
	} `json:"messages"`
}

func writeProbeChunks(t *testing.T, w http.ResponseWriter, chunks ...string) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	for _, chunk := range chunks {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", chunk); err != nil {
			t.Error(err)
			return
		}
	}
	if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
		t.Error(err)
	}
}

func probeModelHandler(t *testing.T, calls *atomic.Int32) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		var body toolProbeRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.Thinking == nil || *body.Thinking || !body.Stream {
			t.Error("expected streamed request with explicitly disabled thinking")
		}
		switch calls.Add(1) {
		case 1:
			if len(body.Tools) != 1 || body.Tools[0].Function.Name != probeToolName {
				t.Errorf("unexpected tools: %+v", body.Tools)
			}
			if choice := string(body.ToolChoice); choice != "" && choice != "null" && choice != `"auto"` {
				t.Errorf("unexpected initial tool choice: %s", choice)
			}
			writeProbeChunks(t, w,
				`{"id":"first","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"`+probeCallID+`","type":"function","function":{"name":"`+probeToolName+`","arguments":"{}"}}]}}]}`,
				`{"id":"first","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			)
		case 2:
			if len(body.Messages) == 0 {
				t.Error("model follow-up has no messages")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			last := body.Messages[len(body.Messages)-1]
			if last.Role != "tool" || last.Content != probeToolResult || last.ToolCallID != probeCallID {
				t.Errorf("tool result was not returned to model: %+v", last)
			}
			writeProbeChunks(t, w,
				`{"id":"second","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"`+probeToolResult+`"}}]}`,
				`{"id":"second","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			)
		default:
			t.Error("unexpected additional model invocation")
			w.WriteHeader(http.StatusBadRequest)
		}
	}
}

func TestNonThinkingStreamToolCalling(t *testing.T) {
	var modelCalls, toolCalls atomic.Int32
	modelServer := httptest.NewServer(probeModelHandler(t, &modelCalls))
	defer modelServer.Close()
	toolServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		toolCalls.Add(1)
		if _, err := fmt.Fprint(w, probeToolResult); err != nil {
			t.Error(err)
		}
	}))
	defer toolServer.Close()
	env := map[string]string{
		"PERSONAOS_LLM_API_KEY":          "fixture-key",
		"PERSONAOS_LLM_MODEL":            "fixture-model",
		"PERSONAOS_LLM_BASE_URL":         modelServer.URL,
		providerconfig.EnvLLMExtraFields: `{"enable_thinking":false}`,
	}
	var mu sync.Mutex
	var events []agentdemo.ExecutionEvent
	ctx := agentdemo.WithExecutionObserver(context.Background(), func(event agentdemo.ExecutionEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	ctx = agentdemo.WithReplyObserver(ctx, func(string) {})
	model, err := configuredChatModel(ctx, func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	chat := agentdemo.NewChatUsecase(toolProbeConfig{}, model).
		WithToolRegistry(agentdemo.NewToolRegistry(map[string]string{"badge": toolServer.URL}))
	reply, err := chat.Chat(ctx, agentdemo.ChatInput{DeviceTypeID: 1, Message: "徽章编号是什么？"})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Reply != probeToolResult || modelCalls.Load() != 2 || toolCalls.Load() != 1 {
		t.Fatalf("reply=%q model calls=%d tool calls=%d", reply.Reply, modelCalls.Load(), toolCalls.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	assertProbeTrace(t, events)
}

func assertProbeTrace(t *testing.T, events []agentdemo.ExecutionEvent) {
	t.Helper()
	var input, toolEnd, toolFinish, stop bool
	for _, event := range events {
		if event.Kind == "model" && event.Phase == "start" {
			input = input || (strings.Contains(event.Input, "Configured enable_thinking: false") && strings.Contains(event.Input, probeToolName))
		}
		if event.Kind == "tool" && event.Name == probeToolName && event.Phase == "end" {
			toolEnd = event.Error == "" && event.Output == probeToolResult
		}
		if event.Kind == "model" && event.Phase == "end" {
			toolFinish = toolFinish || event.FinishReason == "tool_calls"
			stop = stop || event.FinishReason == "stop"
		}
	}
	if !input || !toolEnd || !toolFinish || !stop {
		t.Fatalf("missing diagnostics: %+v", events)
	}
}
