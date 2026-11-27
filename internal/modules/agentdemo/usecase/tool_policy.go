package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

const (
	maxTurnToolExecutions = 8
	toolBudgetFeedback    = `{"error":"tool_budget_exhausted","message":"本轮工具调用额度已用完，请根据已有结果回答。"}`
	unknownToolFeedback   = `{"error":"unknown_tool","message":"该工具不存在，请仅使用本轮提供的工具。"}`
)

type toolRequestKey struct {
	name string
	args string
}

type turnToolResult struct {
	done   chan struct{}
	result string
	err    error
}

// turnToolPolicy is owned by one Agent run, not a shared cache. Eino owns
// dispatch, concurrency and the association between results and call IDs.
type turnToolPolicy struct {
	mu       sync.Mutex
	executed int
	readonly map[string]bool
	results  map[toolRequestKey]*turnToolResult
}

func newDemoToolsConfig(tools []tool.BaseTool) compose.ToolsNodeConfig {
	policy := &turnToolPolicy{
		readonly: map[string]bool{
			currentTimeToolName: true, weatherToolName: true,
			webSearchToolName: true, knowledgeToolName: true,
		},
		results: make(map[toolRequestKey]*turnToolResult),
	}
	for _, entry := range tools {
		if httpEntry, ok := entry.(*httpTool); ok && httpEntry.config.Method == http.MethodGet {
			policy.readonly[httpEntry.info.Name] = true
		}
	}
	return compose.ToolsNodeConfig{
		Tools: tools,
		UnknownToolsHandler: func(ctx context.Context, _, _ string) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			return unknownToolFeedback, nil
		},
		ToolCallMiddlewares: []compose.ToolMiddleware{{Invokable: policy.wrap}},
	}
}

func toolKey(input *compose.ToolInput) toolRequestKey {
	// Canonicalize object key order and insignificant JSON whitespace, without
	// rewriting argument values before the tool's own validation.
	args := strings.TrimSpace(input.Arguments)
	if json.Valid([]byte(args)) {
		var object map[string]json.RawMessage
		if json.Unmarshal([]byte(args), &object) == nil && object != nil {
			if canonical, err := json.Marshal(object); err == nil {
				args = string(canonical)
			}
		}
	}
	return toolRequestKey{name: input.Name, args: args}
}

func (p *turnToolPolicy) wrap(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
	return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := toolKey(input)
		p.mu.Lock()
		if previous := p.results[key]; previous != nil {
			p.mu.Unlock()
			return reuseTurnToolResult(ctx, input.Name, previous)
		}
		if p.executed >= maxTurnToolExecutions {
			p.mu.Unlock()
			stage := startExecutionStage(ctx, "tool_policy", input.Name, "execution budget exhausted")
			stage.finish(toolBudgetFeedback, nil)
			return &compose.ToolOutput{Result: toolBudgetFeedback}, nil
		}
		p.executed++
		var pending *turnToolResult
		if p.readonly[input.Name] {
			pending = &turnToolResult{done: make(chan struct{})}
			p.results[key] = pending
		}
		p.mu.Unlock()
		if pending == nil {
			return next(ctx, input)
		}
		return executeTurnTool(ctx, next, input, pending)
	}
}

func executeTurnTool(ctx context.Context, next compose.InvokableToolEndpoint, input *compose.ToolInput, pending *turnToolResult) (*compose.ToolOutput, error) {
	// Always release duplicate waiters, even if the underlying tool panics.
	pending.err = errors.New("tool execution did not complete")
	defer close(pending.done)
	output, err := next(ctx, input)
	pending.err = err
	if output != nil {
		pending.result = output.Result
	} else if err == nil {
		pending.err = errors.New("tool returned no result")
		return nil, pending.err
	}
	return output, err
}

func reuseTurnToolResult(ctx context.Context, name string, previous *turnToolResult) (*compose.ToolOutput, error) {
	stage := startExecutionStage(ctx, "tool_policy", name, "same-turn result reuse")
	select {
	case <-ctx.Done():
		stage.finish("", ctx.Err())
		return nil, ctx.Err()
	case <-previous.done:
		if err := ctx.Err(); err != nil {
			stage.finish("", err)
			return nil, err
		}
		stage.finish("reused", previous.err)
		if previous.err != nil {
			return nil, previous.err
		}
		return &compose.ToolOutput{Result: previous.result}, nil
	}
}
