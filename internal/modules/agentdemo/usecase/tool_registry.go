package usecase

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const builtinToolProvider = "builtin"

// ToolRegistry resolves published metadata into native Eino tools. It owns
// neither model calls nor tool dispatch. All entries are immutable after setup.
type ToolRegistry struct {
	builtins  map[string]tool.BaseTool
	endpoints map[string]string
	client    *http.Client
}

// NewToolRegistry installs builtins and deployment-approved HTTP endpoint keys.
func NewToolRegistry(endpoints map[string]string) *ToolRegistry {
	return &ToolRegistry{
		builtins:  map[string]tool.BaseTool{currentTimeToolName: currentTimeTool{}},
		endpoints: maps.Clone(endpoints),
		client: &http.Client{
			Timeout: 3 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Resolve exposes only enabled tools in the confirmed device snapshot.
func (r *ToolRegistry) Resolve(ctx context.Context, resources []domain.Resource) ([]tool.BaseTool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tools := make([]tool.BaseTool, 0, len(resources))
	seen := make(map[string]bool)
	for _, resource := range resources {
		if !resource.Enabled || resource.Kind != domain.ResourceTool {
			continue
		}
		entry, err := r.resolve(resource)
		if err != nil {
			return nil, err
		}
		if entry == nil {
			continue
		}
		info, err := entry.Info(ctx)
		if err != nil {
			return nil, err
		}
		if seen[info.Name] {
			continue
		}
		seen[info.Name] = true
		tools = append(tools, entry)
	}
	return tools, nil
}

// WithNetworkTools registers explicitly enabled external query capabilities at startup.
func (r *ToolRegistry) WithNetworkTools(weatherEnabled bool, searchKey string) *ToolRegistry {
	if weatherEnabled {
		r.builtins[weatherToolName] = &weatherTool{
			client: r.client, geocodeURL: "https://geocoding-api.open-meteo.com/v1/search",
			forecastURL: "https://api.open-meteo.com/v1/forecast",
		}
	}
	if strings.TrimSpace(searchKey) != "" {
		r.builtins[webSearchToolName] = &webSearchTool{
			client: r.client, endpoint: deepSeekSearchURL, key: searchKey,
		}
	}
	return r
}

func (r *ToolRegistry) resolve(resource domain.Resource) (tool.BaseTool, error) {
	switch resource.Provider {
	case "", builtinToolProvider:
		return r.builtins[resource.Reference], nil
	case domain.HTTPToolProvider:
		if resource.Code == currentTimeToolName || resource.Code == knowledgeToolName || resource.Code == "skill" ||
			resource.Code == weatherToolName || resource.Code == webSearchToolName {
			return nil, fmt.Errorf("HTTP tool name conflicts with an installed tool")
		}
		return newHTTPTool(resource, r.endpoints[resource.Reference], r.client)
	default:
		return nil, nil
	}
}
