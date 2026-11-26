package usecase

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const (
	httpFixtureQuestion    = "天气如何？"
	httpFixtureDescription = "查询天气"
	httpFixtureCity        = "上海"
	httpFixtureEndpoint    = "weather"
	httpFixtureReply       = "weather-result"
	httpFixtureToolName    = "query_weather"
	httpFixtureConfig      = `{"method":"GET","input_schema":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}`
)

func TestRegistryBuildsConfiguredHTTPTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("city") != httpFixtureCity {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		}
		if _, err := w.Write([]byte("晴天")); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	resource := domain.Resource{
		Kind: domain.ResourceTool, Provider: domain.HTTPToolProvider, Code: "weather_query",
		Reference: httpFixtureEndpoint, Description: httpFixtureDescription, ToolConfig: httpFixtureConfig, Enabled: true,
	}
	registry := NewToolRegistry(map[string]string{httpFixtureEndpoint: server.URL})
	entries, err := registry.Resolve(context.Background(), []domain.Resource{resource})
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
	invokable, ok := entries[0].(tool.InvokableTool)
	if !ok {
		t.Fatal("expected native Eino invokable tool")
	}
	info, err := invokable.Info(context.Background())
	if err != nil || info.Name != resource.Code || info.Desc != resource.Description {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	got, err := invokable.InvokableRun(context.Background(), `{"city":"上海"}`)
	if err != nil || got != "晴天" {
		t.Fatalf("result=%q err=%v", got, err)
	}
	for _, args := range []string{`{}`, `{"city":1}`, `{"city":"上海","url":"http://other"}`} {
		if got, err := invokable.InvokableRun(context.Background(), args); err != nil || got != httpToolArgumentFeedback {
			t.Fatalf("invalid arguments did not return corrective feedback: %q, %v", got, err)
		}
	}
	resource.Enabled = false
	if got, err := registry.Resolve(context.Background(), []domain.Resource{resource}); err != nil || len(got) != 0 {
		t.Fatalf("disabled tools=%v err=%v", got, err)
	}
	resource.Enabled = true
	resource.Reference = "unapproved"
	if _, err := registry.Resolve(context.Background(), []domain.Resource{resource}); err == nil {
		t.Fatal("unapproved endpoint exposed")
	}
}

type httpCallingModel struct {
	gotResult  bool
	corrected  bool
	gotFailure bool
}

func (m *httpCallingModel) Generate(_ context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	last := messages[len(messages)-1]
	if last.Role == schema.Tool {
		if last.Content == httpToolArgumentFeedback {
			m.corrected = true
			return schema.AssistantMessage("", []schema.ToolCall{{
				ID: "http-retry", Type: testToolCallType,
				Function: schema.FunctionCall{Name: httpFixtureToolName, Arguments: `{"city":"上海"}`},
			}}), nil
		}
		if last.Content == networkFailure {
			m.gotFailure = true
			return schema.AssistantMessage("暂时查不到天气。", nil), nil
		}
		m.gotResult = last.Content == httpFixtureReply
		return schema.AssistantMessage(last.Content, nil), nil
	}
	for _, info := range model.GetCommonOptions(nil, opts...).Tools {
		if info.Name == httpFixtureToolName {
			return schema.AssistantMessage("", []schema.ToolCall{{
				ID: "http-call", Type: testToolCallType,
				Function: schema.FunctionCall{Name: info.Name, Arguments: `{"city":1}`},
			}}), nil
		}
	}
	return schema.AssistantMessage("unavailable", nil), nil
}

func (m *httpCallingModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func TestAgentExecutesHTTPToolThroughEino(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("city") != httpFixtureCity {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		if _, err := w.Write([]byte(httpFixtureReply)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	active := domain.PublishedDeviceConfig{
		Character: domain.Character{Name: testCharacterName, Persona: "自然回答"},
		Tools: []domain.Resource{{
			Kind: domain.ResourceTool, Code: httpFixtureToolName, Provider: domain.HTTPToolProvider,
			Reference: httpFixtureEndpoint, Description: httpFixtureDescription, ToolConfig: httpFixtureConfig, Enabled: true,
		}},
	}
	for _, streaming := range []bool{false, true} {
		m := &httpCallingModel{}
		u := NewChatUsecase(&fakeConfigReader{value: active}, m).
			WithToolRegistry(NewToolRegistry(map[string]string{httpFixtureEndpoint: server.URL}))
		var spoken strings.Builder
		var emit func(string) error
		if streaming {
			emit = func(delta string) error {
				spoken.WriteString(delta)
				return nil
			}
		}
		reply, _, err := u.runChat(context.Background(), ChatInput{DeviceTypeID: 7, Message: httpFixtureQuestion}, active, emit)
		if err != nil || !m.corrected || !m.gotResult || reply != httpFixtureReply {
			t.Fatalf("stream=%t tool=%t reply=%q err=%v", streaming, m.gotResult, reply, err)
		}
		if streaming && spoken.String() != reply {
			t.Fatalf("spoken=%q reply=%q", spoken.String(), reply)
		}
	}
}

func TestHTTPToolProviderFailureReturnsFeedback(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "provider status", status: http.StatusServiceUnavailable, body: "private provider failure"},
		{name: "oversized response", status: http.StatusOK, body: strings.Repeat("x", 16385)},
		{name: "invalid utf8", status: http.StatusOK, body: string([]byte{0xff})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			entry := testHTTPFailureTool(t, server.URL, server.Client())
			got, err := entry.InvokableRun(context.Background(), `{"city":"上海"}`)
			if err != nil || got != networkFailure || calls.Load() != 1 {
				t.Fatalf("feedback=%q err=%v calls=%d", got, err, calls.Load())
			}
		})
	}
}

func TestHTTPToolPreservesTurnCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
			t.Error("request context was not canceled")
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	entry := testHTTPFailureTool(t, server.URL, server.Client())
	got, err := entry.InvokableRun(ctx, `{"city":"上海"}`)
	if !errors.Is(err, context.Canceled) || got != "" {
		t.Fatalf("cancellation result=%q err=%v", got, err)
	}
}

func TestHTTPToolClientTimeoutReturnsFeedback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 50 * time.Millisecond
	entry := testHTTPFailureTool(t, server.URL, client)
	got, err := entry.InvokableRun(context.Background(), `{"city":"上海"}`)
	if err != nil || got != networkFailure {
		t.Fatalf("timeout feedback=%q err=%v", got, err)
	}
}

func testHTTPFailureTool(t *testing.T, endpoint string, client *http.Client) tool.InvokableTool {
	t.Helper()
	entry, err := newHTTPTool(domain.Resource{
		Code: httpFixtureToolName, Description: httpFixtureDescription, ToolConfig: httpFixtureConfig,
	}, endpoint, client)
	if err != nil {
		t.Fatal(err)
	}
	invokable, ok := entry.(tool.InvokableTool)
	if !ok {
		t.Fatal("expected native Eino invokable tool")
	}
	return invokable
}

func TestAgentRecoversHTTPToolFailureThroughEino(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	active := domain.PublishedDeviceConfig{
		Character: domain.Character{Name: testCharacterName, Persona: "自然回答"},
		Tools: []domain.Resource{{
			Kind: domain.ResourceTool, Code: httpFixtureToolName, Provider: domain.HTTPToolProvider,
			Reference: httpFixtureEndpoint, Description: httpFixtureDescription, ToolConfig: httpFixtureConfig, Enabled: true,
		}},
	}
	for _, streaming := range []bool{false, true} {
		m := &httpCallingModel{}
		u := NewChatUsecase(&fakeConfigReader{value: active}, m).
			WithToolRegistry(NewToolRegistry(map[string]string{httpFixtureEndpoint: server.URL}))
		var spoken strings.Builder
		var emit func(string) error
		if streaming {
			emit = func(delta string) error {
				spoken.WriteString(delta)
				return nil
			}
		}
		before := calls.Load()
		reply, _, err := u.runChat(context.Background(), ChatInput{DeviceTypeID: 7, Message: httpFixtureQuestion}, active, emit)
		if err != nil || !m.gotFailure || reply != "暂时查不到天气。" || calls.Load()-before != 1 {
			t.Fatalf("stream=%t feedback=%t reply=%q err=%v calls=%d", streaming, m.gotFailure, reply, err, calls.Load()-before)
		}
		if streaming && spoken.String() != reply {
			t.Fatalf("spoken=%q reply=%q", spoken.String(), reply)
		}
	}
}
