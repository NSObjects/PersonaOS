package usecase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const (
	networkTestCity  = "Beijing"
	networkTestQuery = "Go release notes"
	networkTestKey   = "test-only-token"
)

func TestNetworkWeatherQueriesLocationAndForecast(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/geocode":
			if r.URL.Query().Get("name") != networkTestCity {
				t.Errorf("unexpected location query %s", r.URL.RawQuery)
			}
			if _, err := w.Write([]byte(`{"results":[{"name":"Beijing","country":"China","latitude":39.9,"longitude":116.4}]}`)); err != nil {
				t.Error(err)
			}
		case "/forecast":
			if r.URL.Query().Get("latitude") != "39.9" || r.URL.Query().Get("forecast_days") != "3" {
				t.Errorf("unexpected forecast query %s", r.URL.RawQuery)
			}
			if _, err := w.Write([]byte(`{"timezone":"Asia/Shanghai","current":{"time":"2026-11-24T12:00","temperature_2m":12.5,"weather_code":0},"daily":{"time":["2026-11-24"],"temperature_2m_max":[14]}}`)); err != nil {
				t.Error(err)
			}
		default:
			t.Errorf("unexpected provider path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	entry := &weatherTool{client: server.Client(), geocodeURL: server.URL + "/geocode", forecastURL: server.URL + "/forecast"}
	got, err := entry.InvokableRun(context.Background(), `{"city":"Beijing"}`)
	if err != nil || !strings.Contains(got, "12.5") || !strings.Contains(got, "Asia/Shanghai") || !strings.Contains(got, "Open-Meteo") {
		t.Fatalf("weather=%q err=%v", got, err)
	}
}

func TestWebSearchReturnsBoundedSourceResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query string `json:"query"`
			Max   int    `json:"max_results"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+networkTestKey ||
			request.Query != networkTestQuery || request.Max != searchTopK {
			t.Errorf("unexpected search request")
		}
		if _, err := w.Write([]byte(`{"results":[{"title":"Go","url":"https://go.dev/doc/","content":"release notes"},{"title":"invalid","url":"file:///tmp/private","content":"ignored"}]}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	entry := &webSearchTool{client: server.Client(), endpoint: server.URL, key: networkTestKey}
	got, err := entry.InvokableRun(context.Background(), `{"query":"Go release notes"}`)
	var hits []webSearchHit
	if err != nil || json.Unmarshal([]byte(got), &hits) != nil || len(hits) != 1 || hits[0].URL != "https://go.dev/doc/" {
		t.Fatalf("search=%q err=%v", got, err)
	}
}

func TestNetworkToolsRejectInvalidArgumentsAndRecoverProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	entry := &webSearchTool{client: server.Client(), endpoint: server.URL, key: networkTestKey}
	got, err := entry.InvokableRun(context.Background(), `{"query":"Go","url":"https://other.invalid"}`)
	if err != nil || got != httpToolArgumentFeedback {
		t.Fatalf("invalid query=%q err=%v", got, err)
	}
	got, err = entry.InvokableRun(context.Background(), `{"query":"Go"}`)
	if err != nil || got != networkFailure {
		t.Fatalf("unavailable provider=%q err=%v", got, err)
	}
}

func TestNetworkRegistryRequiresConfigurationAndPublication(t *testing.T) {
	ctx := context.Background()
	resources := []domain.Resource{
		{Kind: domain.ResourceTool, Reference: weatherToolName, Enabled: true},
		{Kind: domain.ResourceTool, Reference: webSearchToolName, Enabled: true},
	}
	registry := NewToolRegistry(nil)
	if got, err := registry.Resolve(ctx, resources); err != nil || len(got) != 0 {
		t.Fatalf("unconfigured tools exposed: %v %v", got, err)
	}
	registry.WithNetworkTools(true, networkTestKey)
	entries, err := registry.Resolve(ctx, resources)
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
	for _, entry := range entries {
		if _, ok := entry.(tool.InvokableTool); !ok {
			t.Fatal("expected invokable native Eino tool")
		}
	}
	resources[0].Enabled, resources[1].Enabled = false, false
	if got, err := registry.Resolve(ctx, resources); err != nil || len(got) != 0 {
		t.Fatalf("unpublished tools exposed: %v %v", got, err)
	}
}
