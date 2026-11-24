package usecase

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"time"

	"github.com/cloudwego/eino/components/tool"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

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
func (r *ToolRegistry) Resolve(ctx context.Context, resources []domain.Resource, behaviors *behaviorBuffer) ([]tool.BaseTool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tools := make([]tool.BaseTool, 0, len(resources))
	seen := make(map[string]bool)
	for _, resource := range resources {
		if !resource.Enabled || resource.Kind != domain.ResourceTool {
			continue
		}
		entry, err := r.resolve(resource, behaviors)
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

func (r *ToolRegistry) resolve(resource domain.Resource, behaviors *behaviorBuffer) (tool.BaseTool, error) {
	switch resource.Provider {
	case "", "builtin":
		if resource.Reference == actionToolName || resource.Reference == expressionToolName {
			return newBehaviorTool(resource.Reference, behaviors), nil
		}
		return r.builtins[resource.Reference], nil
	case domain.HTTPToolProvider:
		if resource.Code == currentTimeToolName || resource.Code == knowledgeToolName || resource.Code == "skill" ||
			resource.Code == actionToolName || resource.Code == expressionToolName {
			return nil, fmt.Errorf("HTTP tool name conflicts with an installed tool")
		}
		return newHTTPTool(resource, r.endpoints[resource.Reference], r.client)
	default:
		return nil, nil
	}
}
