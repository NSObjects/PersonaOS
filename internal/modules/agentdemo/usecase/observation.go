package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ExecutionEvent describes one lifecycle change in a single demo turn.
type ExecutionEvent struct {
	ID     uint64             `json:"id"`
	Kind   string             `json:"kind"`
	Name   string             `json:"name"`
	Phase  string             `json:"phase"`
	TimeMS int64              `json:"time_ms"`
	Input  string             `json:"input,omitempty"`
	Output string             `json:"output,omitempty"`
	Usage  *schema.TokenUsage `json:"usage,omitempty"`
	Error  string             `json:"error,omitempty"`
	FinishReason string       `json:"finish_reason,omitempty"`
}

const (
	executionModel = "model"
	executionTool  = "tool"
	executionStart = "start"
	executionEnd   = "end"
	executionFirst = "first_output"
)

type observerKey struct{}

type executionObserver struct {
	mu   sync.Mutex
	next uint64
	emit func(ExecutionEvent)
	modelOptions string
}

// WithExecutionObserver attaches a request-local, nonblocking event sink.
func WithExecutionObserver(ctx context.Context, emit func(ExecutionEvent)) context.Context {
	return context.WithValue(ctx, observerKey{}, &executionObserver{emit: emit})
}

func observerFrom(ctx context.Context) *executionObserver {
	o, ok := ctx.Value(observerKey{}).(*executionObserver)
	if !ok {
		return nil
	}
	return o
}

// ObserveConfiguredModelOptions records only explicitly allowlisted vendor options.
// It is configuration metadata, not an HTTP capture or proof of provider behavior.
func ObserveConfiguredModelOptions(ctx context.Context, extra map[string]any) {
	o := observerFrom(ctx)
	if o == nil {
		return
	}
	thinking := "not set"
	if value, exists := extra["enable_thinking"]; exists {
		thinking = "unrecorded value"
		if enabled, ok := value.(bool); ok {
			thinking = fmt.Sprint(enabled)
		}
	}
	effort := "not set"
	if value, exists := extra["reasoning_effort"]; exists {
		effort = "unrecorded value"
		if level, ok := value.(string); ok {
			switch level {
			case "none", "minimal", "low", "medium", "high", "xhigh":
				effort = level
			}
		}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.modelOptions = fmt.Sprintf("Configured enable_thinking: %s\nConfigured reasoning_effort: %s", thinking, effort)
}

func (o *executionObserver) start(kind, name, input string) ExecutionEvent {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.next++
	if kind == executionModel && o.modelOptions != "" {
		input += "\n" + o.modelOptions
	}
	event := ExecutionEvent{ID: o.next, Kind: kind, Name: name, Phase: executionStart, TimeMS: time.Now().UnixMilli(), Input: boundedDetail(input)}
	o.emit(event)
	return event
}

func (o *executionObserver) finish(event ExecutionEvent, output string, usage *schema.TokenUsage, err error) {
	event.Phase = executionEnd
	event.TimeMS = time.Now().UnixMilli()
	event.Input = ""
	event.Output = boundedDetail(output)
	event.Usage = usage
	if err != nil {
		event.Error = "execution failed"
		if errors.Is(err, context.Canceled) {
			event.Error = "canceled"
		} else if errors.Is(err, context.DeadlineExceeded) {
			event.Error = "timeout"
		}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emit(event)
}

// firstOutput records the first usable streamed result, not a provider-reported token timestamp.
func (o *executionObserver) firstOutput(event ExecutionEvent) {
	event.Phase = executionFirst
	event.TimeMS = time.Now().UnixMilli()
	event.Input = ""
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emit(event)
}

func boundedDetail(text string) string {
	const limit = 4096
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}

func observeStage(ctx context.Context, kind, name string) func(error) {
	stage := startExecutionStage(ctx, kind, name, "")
	return func(err error) { stage.finish("", err) }
}

type executionStage struct {
	observer *executionObserver
	event    ExecutionEvent
}

func startExecutionStage(ctx context.Context, kind, name, input string) executionStage {
	o := observerFrom(ctx)
	if o == nil {
		return executionStage{}
	}
	return executionStage{observer: o, event: o.start(kind, name, input)}
}

func (s executionStage) firstOutput() {
	if s.observer != nil {
		s.observer.firstOutput(s.event)
	}
}

func (s executionStage) finish(output string, err error) {
	if s.observer != nil {
		s.observer.finish(s.event, output, nil, err)
	}
}

type componentEventKey struct{}

type executionCallbacks struct {
	observer *executionObserver
	streams  sync.WaitGroup
}

func (h *executionCallbacks) Needed(_ context.Context, info *callbacks.RunInfo, timing callbacks.CallbackTiming) bool {
	return info != nil && (info.Component == components.ComponentOfChatModel || info.Component == components.ComponentOfTool) &&
		timing != callbacks.TimingOnStartWithStreamInput
}

func (h *executionCallbacks) OnStart(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
	kind, name, detail := executionModel, info.Type, ""
	if info.Component == components.ComponentOfTool {
		kind, name = executionTool, info.Name
		if in := tool.ConvCallbackInput(input); in != nil {
			detail = in.ArgumentsInJSON
		}
	} else if in := model.ConvCallbackInput(input); in != nil {
		detail = modelInputDetail(in)
		if in.Config != nil && in.Config.Model != "" {
			name = in.Config.Model
		}
	}
	return context.WithValue(ctx, componentEventKey{}, h.observer.start(kind, name, detail))
}

func modelInputDetail(in *model.CallbackInput) string {
	var detail strings.Builder
	fmt.Fprintf(&detail, "%d messages, %d tools", len(in.Messages), len(in.Tools))
	choice := "unspecified"
	if in.ToolChoice != nil {
		choice = string(*in.ToolChoice)
	}
	fmt.Fprintf(&detail, "\nEino tool_choice: %s", choice)
	names := make([]string, 0, len(in.Tools))
	for _, info := range in.Tools {
		if info != nil {
			names = append(names, info.Name)
		}
	}
	fmt.Fprintf(&detail, "\nTools: %s", strings.Join(names, ", "))
	for _, info := range in.Tools {
		if info != nil {
			fmt.Fprintf(&detail, "\n- %s: %s", info.Name, boundedDetail(info.Desc))
		}
	}
	return detail.String()
}

func (h *executionCallbacks) OnEnd(ctx context.Context, _ *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
	h.complete(ctx, output, nil)
	return ctx
}

func (h *executionCallbacks) OnError(ctx context.Context, _ *callbacks.RunInfo, err error) context.Context {
	h.complete(ctx, nil, err)
	return ctx
}

func (h *executionCallbacks) OnStartWithStreamInput(ctx context.Context, _ *callbacks.RunInfo, input *schema.StreamReader[callbacks.CallbackInput]) context.Context {
	input.Close()
	return ctx
}

func (h *executionCallbacks) OnEndWithStreamOutput(ctx context.Context, _ *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
	// Eino provides a separate reader. Consume it asynchronously so the
	// original stream continues to feed the agent and speech synthesis.
	h.streams.Go(func() {
		defer output.Close()
		var detail strings.Builder
		var usage *schema.TokenUsage
		var finishReason string
		first := false
		for {
			value, err := output.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = nil
				}
				if event, ok := ctx.Value(componentEventKey{}).(ExecutionEvent); ok {
					event.FinishReason = finishReason
					h.observer.finish(event, detail.String(), usage, err)
				}
				return
			}
			part, current, reason := componentOutput(value)
			if reason != "" {
				finishReason = reason
			}
			if !first && strings.TrimSpace(part) != "" {
				first = true
				if event, ok := ctx.Value(componentEventKey{}).(ExecutionEvent); ok {
					h.observer.firstOutput(event)
				}
			}
			if detail.Len() < 16384 {
				detail.WriteString(boundedDetail(part))
			}
			if current != nil {
				usage = current
			}
		}
	})
	return ctx
}

func (h *executionCallbacks) complete(ctx context.Context, output callbacks.CallbackOutput, err error) {
	event, ok := ctx.Value(componentEventKey{}).(ExecutionEvent)
	if !ok {
		return
	}
	detail, usage, reason := componentOutput(output)
	event.FinishReason = reason
	h.observer.finish(event, detail, usage, err)
}

func componentOutput(value callbacks.CallbackOutput) (string, *schema.TokenUsage, string) {
	if out := model.ConvCallbackOutput(value); out != nil {
		var usage *schema.TokenUsage
		if out.TokenUsage != nil {
			usage = &schema.TokenUsage{
				PromptTokens: out.TokenUsage.PromptTokens, CompletionTokens: out.TokenUsage.CompletionTokens, TotalTokens: out.TokenUsage.TotalTokens,
				PromptTokenDetails: schema.PromptTokenDetails{CachedTokens: out.TokenUsage.PromptTokenDetails.CachedTokens, CacheWriteTokens: out.TokenUsage.PromptTokenDetails.CacheWriteTokens},
				CompletionTokensDetails: schema.CompletionTokensDetails{ReasoningTokens: out.TokenUsage.CompletionTokensDetails.ReasoningTokens},
			}
		}
		if out.Message == nil {
			return "", usage, ""
		}
		var reason string
		if out.Message.ResponseMeta != nil {
			reason = out.Message.ResponseMeta.FinishReason
			if reported := out.Message.ResponseMeta.Usage; usage == nil && reported != nil {
				copy := *reported
				usage = &copy
			}
		}
		detail := out.Message.Content
		for _, call := range out.Message.ToolCalls {
			if call.Function.Name != "" {
				detail += "\nTool: " + call.Function.Name
			}
		}
		return detail, usage, reason
	}
	if out := tool.ConvCallbackOutput(value); out != nil {
		return out.Response, nil, ""
	}
	return "", nil, ""
}
