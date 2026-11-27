package usecase

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func TestReadAgentReplySuppressesKnownToolText(t *testing.T) {
	call := schema.ToolCall{
		ID: "weather-call", Type: testToolCallType,
		Function: schema.FunctionCall{Name: weatherToolName, Arguments: `{"city":"上海"}`},
	}
	message := schema.AssistantMessage("我查查。", []schema.ToolCall{call})
	for _, streaming := range []bool{false, true} {
		output := &adk.MessageVariant{Role: schema.Assistant, IsStreaming: streaming, Message: message}
		if streaming {
			output.Message = nil
			output.MessageStream = schema.StreamReaderFromArray([]*schema.Message{
				message, schema.AssistantMessage("等一下。", nil),
			})
		}
		reply, err := readAgentReply(output, func(delta string) error {
			t.Errorf("tool-call text reached output: %q", delta)
			return nil
		})
		if err != nil || reply != "" {
			t.Fatalf("streaming=%t reply=%q err=%v", streaming, reply, err)
		}
	}
}

func TestReadAgentStreamPreservesAlreadyEmittedText(t *testing.T) {
	call := schema.ToolCall{
		ID: "late-call", Type: testToolCallType,
		Function: schema.FunctionCall{Name: weatherToolName, Arguments: `{"city":"北京"}`},
	}
	output := &adk.MessageVariant{
		Role: schema.Assistant, IsStreaming: true,
		MessageStream: schema.StreamReaderFromArray([]*schema.Message{
			schema.AssistantMessage("先看看。", nil),
			schema.AssistantMessage("", []schema.ToolCall{call}),
			schema.AssistantMessage("不应继续朗读。", nil),
		}),
	}
	var emitted strings.Builder
	reply, err := readAgentReply(output, func(delta string) error {
		emitted.WriteString(delta)
		return nil
	})
	if err != nil || reply != "先看看。" || reply != emitted.String() {
		t.Fatalf("reply=%q emitted=%q err=%v", reply, emitted.String(), err)
	}
}

func TestReadAgentStreamEmitsNormalChunksSeparately(t *testing.T) {
	output := &adk.MessageVariant{
		Role: schema.Assistant, IsStreaming: true,
		MessageStream: schema.StreamReaderFromArray([]*schema.Message{
			schema.AssistantMessage(streamTestMessage, nil),
			schema.AssistantMessage("呀！", nil),
		}),
	}
	var chunks []string
	reply, err := readAgentReply(output, func(delta string) error {
		chunks = append(chunks, delta)
		return nil
	})
	if err != nil || reply != streamTestMessage+"呀！" || len(chunks) != 2 || chunks[0] != streamTestMessage || chunks[1] != "呀！" {
		t.Fatalf("reply=%q chunks=%q err=%v", reply, chunks, err)
	}
}
