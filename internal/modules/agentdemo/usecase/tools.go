package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/rs/zerolog/log"
)

const currentTimeToolName = "get_current_time"

type currentTimeTool struct{}

func (currentTimeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        currentTimeToolName,
		Desc:        "查询当前真实 UTC 日期和时间。询问现在几点、今天日期、星期等当前时间信息时使用。结果为 UTC；用户指定或上下文明确时区时按该时区换算，不猜测用户所在地。无需参数，传入 {}。",
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
