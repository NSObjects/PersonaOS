package boot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/NSObjects/PersonaOS/internal/modules/providerconfig"
)

func TestConfiguredChatModelSendsVendorOptions(t *testing.T) {
	const modelName = "test-model"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body["enable_thinking"] != false || body["stream"] != true || body["model"] != modelName {
			t.Errorf("unexpected model request: %v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := w.Write([]byte("data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n")); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	env := map[string]string{
		"PERSONAOS_LLM_API_KEY":          "test-key",
		"PERSONAOS_LLM_MODEL":            modelName,
		"PERSONAOS_LLM_BASE_URL":         server.URL,
		providerconfig.EnvLLMExtraFields: `{"enable_thinking":false}`,
	}
	ctx := context.Background()
	model, err := configuredChatModel(ctx, func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	stream, err := model.Stream(ctx, []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	msg, err := stream.Recv()
	if err != nil || msg == nil || msg.Content != "hello" {
		t.Fatalf("stream message=%v err=%v", msg, err)
	}
}
