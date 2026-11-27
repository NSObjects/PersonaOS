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
	for _, field := range []string{"latitude", "longitude", "apparent_temperature", "wind_speed_10m", "temperature_2m_min", "precipitation_probability_max"} {
		if strings.Contains(got, field) {
			t.Errorf("unexpected absent or internal field %q in %s", field, got)
		}
	}
	if !strings.Contains(got, `"weather_code":0`) {
		t.Fatalf("valid zero weather code was lost: %s", got)
	}
}

func TestWebSearchReturnsBoundedSourceResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query       string `json:"query"`
			Count       int    `json:"count"`
			NeedContent *bool  `json:"need_content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+networkTestKey ||
			request.Query != networkTestQuery || request.Count != searchTopK || request.NeedContent == nil || *request.NeedContent {
			t.Errorf("unexpected search request")
		}
		if _, err := w.Write([]byte(`{"results":[{"title":"Go","url":"https://go.dev/doc/","summary":"release notes","content":"not requested"},{"title":"invalid","url":"file:///tmp/private","summary":"ignored"},{"title":"Go Blog","url":"https://go.dev/blog/","snippet":"fallback snippet"}]}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	entry := &webSearchTool{client: server.Client(), endpoint: server.URL, key: networkTestKey}
	got, err := entry.InvokableRun(context.Background(), `{"query":"Go release notes"}`)
	var hits []webSearchHit
	if err != nil || json.Unmarshal([]byte(got), &hits) != nil || len(hits) != 2 || hits[0].URL != "https://go.dev/doc/" || hits[0].Content != "release notes" || hits[1].Content != "fallback snippet" {
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
	got, err = entry.InvokableRun(context.Background(), `{"query":"`+strings.Repeat("中", 129)+`"}`)
	if err != nil || got != httpToolArgumentFeedback {
		t.Fatalf("oversized search query=%q err=%v", got, err)
	}
	got, err = entry.InvokableRun(context.Background(), `{"query":"Go"}`)
	if err != nil || got != webSearchFailure {
		t.Fatalf("unavailable provider=%q err=%v", got, err)
	}
}

func TestNetworkRegistryUsesPublicationAndSearchConfiguration(t *testing.T) {
	ctx := context.Background()
	resources := []domain.Resource{
		{Kind: domain.ResourceTool, Reference: weatherToolName, Enabled: true},
		{Kind: domain.ResourceTool, Reference: webSearchToolName, Enabled: true},
	}
	registry := NewToolRegistry(nil)
	weatherOnly, err := registry.Resolve(ctx, resources)
	if err != nil || len(weatherOnly) != 1 {
		t.Fatalf("expected weather only: %v %v", weatherOnly, err)
	}
	info, err := weatherOnly[0].Info(ctx)
	if err != nil || info.Name != weatherToolName {
		t.Fatalf("unexpected tool: %v %v", info, err)
	}
	if got, err := registry.Resolve(ctx, nil); err != nil || len(got) != 0 {
		t.Fatalf("unbound tools exposed: %v %v", got, err)
	}
	registry.WithWebSearch(networkTestKey)
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

func TestCompactWebSearchHitsPreservesDistinctEvidence(t *testing.T) {
	first := webSearchResult{Title: " Release notes ", URL: "https://example.org/release", Summary: " Original fact "}
	duplicate := first
	duplicate.Title = strings.TrimSpace(first.Title)
	duplicate.Summary = strings.TrimSpace(first.Summary)
	sameSource := first
	sameSource.Summary = "Updated fact"
	otherSource := first
	otherSource.URL = "https://example.org/other"
	hits := compactWebSearchHits([]webSearchResult{first, duplicate, sameSource, otherSource})
	if len(hits) != searchTopK {
		t.Fatalf("unexpected hits: %+v", hits)
	}
	if hits[0].Title != duplicate.Title || hits[0].Content != duplicate.Summary || hits[1].Content != sameSource.Summary || hits[2].URL != otherSource.URL {
		t.Fatalf("distinct evidence lost: %+v", hits)
	}
}
