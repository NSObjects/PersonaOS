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
}

const (
	executionModel = "model"
	executionTool  = "tool"
	executionStart = "start"
	executionEnd   = "end"
)

type observerKey struct{}

type executionObserver struct {
	mu   sync.Mutex
	next uint64
	emit func(ExecutionEvent)
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

func (o *executionObserver) start(kind, name, input string) ExecutionEvent {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.next++
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

func boundedDetail(text string) string {
	const limit = 4096
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}

func observeStage(ctx context.Context, kind, name string) func(error) {
	o := observerFrom(ctx)
	if o == nil {
		return func(error) {}
	}
	event := o.start(kind, name, "")
	return func(err error) { o.finish(event, "", nil, err) }
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
		detail = fmt.Sprintf("%d messages, %d tools", len(in.Messages), len(in.Tools))
		if in.Config != nil && in.Config.Model != "" {
			name = in.Config.Model
		}
	}
	return context.WithValue(ctx, componentEventKey{}, h.observer.start(kind, name, detail))
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
		for {
			value, err := output.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = nil
				}
				if event, ok := ctx.Value(componentEventKey{}).(ExecutionEvent); ok {
					h.observer.finish(event, detail.String(), usage, err)
				}
				return
			}
			part, current := componentOutput(value)
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
	detail, usage := componentOutput(output)
	h.observer.finish(event, detail, usage, err)
}

func componentOutput(value callbacks.CallbackOutput) (string, *schema.TokenUsage) {
	if out := model.ConvCallbackOutput(value); out != nil {
		var usage *schema.TokenUsage
		if out.TokenUsage != nil {
			usage = &schema.TokenUsage{PromptTokens: out.TokenUsage.PromptTokens, CompletionTokens: out.TokenUsage.CompletionTokens, TotalTokens: out.TokenUsage.TotalTokens}
		}
		if out.Message == nil {
			return "", usage
		}
		if usage == nil && out.Message.ResponseMeta != nil {
			if reported := out.Message.ResponseMeta.Usage; reported != nil {
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
		return detail, usage
	}
	if out := tool.ConvCallbackOutput(value); out != nil {
		return out.Response, nil
	}
	return "", nil
}
