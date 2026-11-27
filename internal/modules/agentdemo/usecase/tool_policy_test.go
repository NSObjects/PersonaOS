package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const policyEmptyArgs = "{}"

func policyCall(id, args string) schema.ToolCall {
	return schema.ToolCall{
		ID: id, Type: testToolCallType,
		Function: schema.FunctionCall{Name: httpFixtureToolName, Arguments: args},
	}
}

func TestTurnToolPolicyWithEino(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			verifyTurnToolPolicy(t, status)
		})
	}
}

func verifyTurnToolPolicy(t *testing.T, status int) {
	t.Helper()
	ctx := context.Background()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(status)
		if _, err := fmt.Fprint(w, httpFixtureReply); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	tools := []tool.BaseTool{testHTTPFailureTool(t, server.URL, server.Client())}
	node, err := compose.NewToolNode(ctx, ptrToolsConfig(newDemoToolsConfig(tools)))
	if err != nil {
		t.Fatal(err)
	}
	first := policyCall("first", `{"city":"上海"}`)
	second := policyCall("second", `{ "city" : "上海" }`)
	msg := schema.AssistantMessage("", []schema.ToolCall{first, second})
	results, err := node.Invoke(ctx, msg)
	expected := httpFixtureReply
	if status != http.StatusOK {
		expected = networkFailure
	}
	if err != nil || len(results) != 2 || calls.Load() != 1 {
		t.Fatalf("results=%v calls=%d err=%v", results, calls.Load(), err)
	}
	for i, result := range results {
		if result.Content != expected || result.ToolCallID != msg.ToolCalls[i].ID {
			t.Fatalf("unexpected tool result: %+v", result)
		}
	}
	verifyPolicyStreamReuse(t, node, first, expected, &calls)

	_, err = node.Invoke(ctx, schema.AssistantMessage("", []schema.ToolCall{policyCall("other", `{"city":"北京"}`)}))
	if err != nil || calls.Load() != 2 {
		t.Fatalf("distinct query calls=%d err=%v", calls.Load(), err)
	}
	fresh, err := compose.NewToolNode(ctx, ptrToolsConfig(newDemoToolsConfig(tools)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = fresh.Invoke(ctx, schema.AssistantMessage("", []schema.ToolCall{first}))
	if err != nil || calls.Load() != 3 {
		t.Fatalf("new turn calls=%d err=%v", calls.Load(), err)
	}
}

func verifyPolicyStreamReuse(t *testing.T, node *compose.ToolsNode, first schema.ToolCall, expected string, calls *atomic.Int32) {
	t.Helper()
	ctx := context.Background()
	stream, err := node.Stream(ctx, schema.AssistantMessage("", []schema.ToolCall{policyCall("third", first.Function.Arguments)}))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	chunk, err := stream.Recv()
	if err != nil || len(chunk) != 1 || chunk[0].Content != expected || chunk[0].ToolCallID != "third" || calls.Load() != 1 {
		t.Fatalf("stream=%v calls=%d err=%v", chunk, calls.Load(), err)
	}
}

func ptrToolsConfig(cfg compose.ToolsNodeConfig) *compose.ToolsNodeConfig {
	return &cfg
}

func TestTurnToolPolicyBudgetAndCancellation(t *testing.T) {
	ctx := context.Background()
	cfg := newDemoToolsConfig(nil)
	var calls int
	run := cfg.ToolCallMiddlewares[0].Invokable(func(_ context.Context, _ *compose.ToolInput) (*compose.ToolOutput, error) {
		calls++
		return &compose.ToolOutput{Result: httpFixtureReply}, nil
	})
	// Non-read-only tools must not reuse successful responses.
	input := &compose.ToolInput{Name: "write_tool", Arguments: policyEmptyArgs}
	for range maxTurnToolExecutions {
		got, err := run(ctx, input)
		if err != nil || got.Result != httpFixtureReply {
			t.Fatalf("result=%v err=%v", got, err)
		}
	}
	got, err := run(ctx, input)
	if err != nil || got.Result != toolBudgetFeedback || calls != maxTurnToolExecutions {
		t.Fatalf("budget result=%v calls=%d err=%v", got, calls, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = run(canceled, input)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error=%v", err)
	}
	feedback, err := cfg.UnknownToolsHandler(ctx, "missing", policyEmptyArgs)
	if err != nil || feedback != unknownToolFeedback {
		t.Fatalf("unknown tool result=%q err=%v", feedback, err)
	}
}
