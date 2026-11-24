package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const currentTimeToolName = "get_current_time"

// A published reference selects an installed handler, not arbitrary executable
// code. Never use catalog text as a command, URL or tool implementation.
func resolveDemoTools(resources []domain.Resource) []tool.BaseTool {
	var tools []tool.BaseTool
	seen := make(map[string]bool)
	for _, resource := range resources {
		if !resource.Enabled || resource.Kind != domain.ResourceTool || seen[resource.Reference] {
			continue
		}
		if resource.Reference == currentTimeToolName {
			tools = append(tools, currentTimeTool{})
			seen[resource.Reference] = true
		}
	}
	return tools
}

type currentTimeTool struct{}

func (currentTimeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        currentTimeToolName,
		Desc:        "查询当前真实 UTC 日期和时间；需要知道当前时间时调用，不需要参数。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

func (currentTimeTool) InvokableRun(ctx context.Context, _ string, _ ...tool.Option) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(struct {
		UTC string `json:"utc"`
	}{UTC: now})
	if err != nil {
		return "", err
	}
	log.Info().Str("tool", currentTimeToolName).Msg("demo agent tool executed")
	return string(data), nil
}
