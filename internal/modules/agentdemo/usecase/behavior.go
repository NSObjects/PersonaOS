package usecase

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	actionToolName     = "perform_action"
	expressionToolName = "set_expression"
	actionKind         = "action"
	expressionKind     = "expression"
	previewStatus      = "preview_queued"
	maxBehaviorEvents  = 4
)

// BehaviorEvent is a requested browser preview, not a hardware acknowledgement.
type BehaviorEvent struct {
	Kind string `json:"kind"`
	Code string `json:"code"`
}

type behaviorBuffer struct {
	mu     sync.Mutex
	events []BehaviorEvent
}

func (b *behaviorBuffer) add(event BehaviorEvent) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.events) >= maxBehaviorEvents {
		return false
	}
	b.events = append(b.events, event)
	return true
}

func (b *behaviorBuffer) snapshot() []BehaviorEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]BehaviorEvent(nil), b.events...)
}

type behaviorTool struct {
	name   string
	kind   string
	desc   string
	codes  []string
	buffer *behaviorBuffer
}

func newBehaviorTool(name string, buffer *behaviorBuffer) tool.BaseTool {
	if name == actionToolName {
		return behaviorTool{
			name: name, kind: actionKind, buffer: buffer,
			desc: "为角色请求动作预览，例如挥手、点头或摇头。只有网页预览，没有真实硬件执行回执。",
			codes: []string{"wave", "nod", "shake_head"},
		}
	}
	return behaviorTool{
		name: name, kind: expressionKind, buffer: buffer,
		desc: "为角色请求表情预览，例如微笑、难过、惊讶或恢复自然表情。只有网页预览，没有真实硬件执行回执。",
		codes: []string{"smile", "sad", "surprised", "neutral"},
	}
}

func (t behaviorTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.name, Desc: t.desc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"code": {Type: schema.String, Enum: t.codes, Required: true},
		}),
	}, nil
}

func (t behaviorTool) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var values map[string]string
	if len(args) > 256 || json.Unmarshal([]byte(args), &values) != nil || len(values) != 1 {
		return httpToolArgumentFeedback, nil
	}
	code := values["code"]
	valid := false
	for _, allowed := range t.codes {
		if allowed == code {
			valid = true
			break
		}
	}
	if !valid {
		return httpToolArgumentFeedback, nil
	}
	if !t.buffer.add(BehaviorEvent{Kind: t.kind, Code: code}) {
		return `{"error":"preview_limit","message":"本轮预览事件已达到上限，不再请求动作或表情。"}`, nil
	}
	result, err := json.Marshal(struct {
		Status string `json:"status"`
		Kind   string `json:"kind"`
		Code   string `json:"code"`
	}{Status: previewStatus, Kind: t.kind, Code: code})
	return string(result), err
}
