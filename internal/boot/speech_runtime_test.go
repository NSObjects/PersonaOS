package boot

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NSObjects/go-speech/asr"
	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"
	"github.com/gorilla/websocket"

	agentdemo "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const (
	poolTestAliyun   = "aliyun"
	poolTestAPIKey   = "DASHSCOPE_API_KEY"
	poolTestEndpoint = "DASHSCOPE_WS_ENDPOINT"
)

func testSpeechRuntime(t *testing.T) (*speechClientRuntime, map[string]string) {
	t.Helper()
	t.Setenv(poolTestEndpoint, "")
	config := map[string]string{
		"PERSONAOS_ASR_PROVIDER": poolTestAliyun,
		poolTestAPIKey:           "fixture-key",
		"VOLCENGINE_API_KEY":     "fixture-volc-key",
	}
	runtime := newSpeechClientRuntime(func(context.Context) (func(string) string, error) {
		return func(key string) string { return config[key] }, nil
	})
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return runtime, config
}

func pinSpeechRuntime(t *testing.T, runtime *speechClientRuntime) agentdemo.VoiceRuntime {
	t.Helper()
	snapshot, err := runtime.resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(snapshot.Release)
	return snapshot
}

func speechTestTTS(t *testing.T, snapshot agentdemo.VoiceRuntime, provider string) tts.Client {
	t.Helper()
	client, err := snapshot.Factory(provider)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestSpeechRuntimeSharesClientsAcrossSnapshots(t *testing.T) {
	runtime, config := testSpeechRuntime(t)
	first := pinSpeechRuntime(t, runtime)
	client := speechTestTTS(t, first, poolTestAliyun)
	if !client.(tts.CapabilityProvider).Capabilities().Pool.Enabled ||
		!first.Recognizer.(asr.CapabilityProvider).Capabilities().Pool.Enabled {
		t.Fatal("Aliyun pools are not enabled")
	}
	volc := speechTestTTS(t, first, "volcengine")
	if volc != speechTestTTS(t, first, domain.Seed3CloneProvider) ||
		!volc.(tts.CapabilityProvider).Capabilities().Pool.Enabled {
		t.Fatal("official and cloned Seed-TTS voices must share the pooled client")
	}
	first.Release()
	config["PERSONAOS_LLM_MODEL"] = "unrelated-change"
	second := pinSpeechRuntime(t, runtime)
	if first.Recognizer != second.Recognizer || client != speechTestTTS(t, second, poolTestAliyun) {
		t.Fatal("unchanged speech settings recreated the SDK clients")
	}
}

func TestSpeechRuntimeRetiresOnlyAfterLastRequest(t *testing.T) {
	runtime, config := testSpeechRuntime(t)
	first := pinSpeechRuntime(t, runtime)
	second := pinSpeechRuntime(t, runtime)
	oldClient := speechTestTTS(t, first, poolTestAliyun)
	oldPool := oldClient.(tts.CapabilityProvider)
	config[poolTestAPIKey] = "rotated-key"
	third := pinSpeechRuntime(t, runtime)
	newClient := speechTestTTS(t, third, poolTestAliyun)
	if newClient == oldClient || third.Recognizer == first.Recognizer || !oldPool.Capabilities().Pool.Enabled {
		t.Fatal("rotation did not preserve the pinned old request")
	}
	first.Release()
	first.Release()
	if !oldPool.Capabilities().Pool.Enabled {
		t.Fatal("old pool closed before last request released")
	}
	second.Release()
	if oldPool.Capabilities().Pool.Enabled {
		t.Fatal("retired pool still enabled")
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if newClient.(tts.CapabilityProvider).Capabilities().Pool.Enabled {
		t.Fatal("shutdown did not close current pool")
	}
	if _, err := runtime.resolve(context.Background()); err == nil {
		t.Fatal("shutdown permitted another snapshot")
	}
}

type poolTestCommand struct {
	Header struct {
		Action string `json:"action"`
		TaskID string `json:"task_id"`
	} `json:"header"`
}

func readPoolCommand(ws *websocket.Conn, action, taskID string) (string, error) {
	var command poolTestCommand
	if err := ws.ReadJSON(&command); err != nil {
		return "", err
	}
	if command.Header.Action != action || command.Header.TaskID == "" ||
		(taskID != "" && taskID != command.Header.TaskID) {
		return "", errors.New("unexpected TTS command")
	}
	return command.Header.TaskID, nil
}

func poolTestEvent(ws *websocket.Conn, event, id string) error {
	return ws.WriteJSON(map[string]any{
		"header":  map[string]string{"event": event, "task_id": id},
		"payload": map[string]any{},
	})
}

func servePoolTask(ws *websocket.Conn) error {
	id, err := readPoolCommand(ws, "run-task", "")
	if err != nil {
		return err
	}
	if err := poolTestEvent(ws, "task-started", id); err != nil {
		return err
	}
	if _, err := readPoolCommand(ws, "continue-task", id); err != nil {
		return err
	}
	if _, err := readPoolCommand(ws, "finish-task", id); err != nil {
		return err
	}
	if err := ws.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
		return err
	}
	return poolTestEvent(ws, "task-finished", id)
}

func testPooledDuplex(t *testing.T, runtime *speechClientRuntime) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snapshot, err := runtime.resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Release()
	client := speechTestTTS(t, snapshot, poolTestAliyun)
	stream, err := client.(tts.DuplexSynthesizer).OpenDuplex(ctx, tts.DuplexRequest{
		Voice: "longanyang", Format: audio.Format{Encoding: audio.EncodingPCM16LE, SampleRate: 24000, Channels: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if err := stream.SendText(ctx, "你好"); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(ctx); err != nil {
		t.Fatal(err)
	}
	chunk, err := stream.Recv(ctx)
	if err != nil || len(chunk.Data) != 2 {
		t.Fatalf("audio=%v err=%v", chunk.Data, err)
	}
	if _, err := stream.Recv(ctx); !errors.Is(err, io.EOF) {
		t.Fatalf("expected successful terminal event, got %v", err)
	}
}

func TestSpeechRuntimeReusesSocketAcrossRequests(t *testing.T) {
	var sockets atomic.Int32
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer ws.Close()
		sockets.Add(1)
		for {
			if err := servePoolTask(ws); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	runtime, _ := testSpeechRuntime(t)
	t.Setenv(poolTestEndpoint, "ws"+strings.TrimPrefix(server.URL, "http")+"/api-ws/v1/inference")
	for range 2 {
		testPooledDuplex(t, runtime)
	}
	if sockets.Load() != 1 {
		t.Fatalf("expected a single physical WebSocket, got %d", sockets.Load())
	}
}
