package usecase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const httpFixtureConfig = `{"method":"GET","input_schema":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}`

func TestRegistryBuildsConfiguredHTTPTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("city") != "上海" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		}
		if _, err := w.Write([]byte("晴天")); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	resource := domain.Resource{
		Kind: domain.ResourceTool, Provider: domain.HTTPToolProvider, Code: "weather_query",
		Reference: "weather", Description: "查询天气", ToolConfig: httpFixtureConfig, Enabled: true,
	}
	registry := NewToolRegistry(map[string]string{"weather": server.URL})
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
		if _, err := invokable.InvokableRun(context.Background(), args); err == nil {
			t.Fatalf("invalid arguments accepted: %s", args)
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
