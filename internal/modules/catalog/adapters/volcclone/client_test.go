package volcclone

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

const (
	testKey     = "fixture-key"
	testSpeaker = "PersonaOS_fixture"
	testDemo    = "https://x.bytespeech.com/fixture"
)

func TestCloneProviderContract(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if !checkCloneRequest(t, req) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"speaker_id":"PersonaOS_fixture","status":2,"available_training_times":14,"speaker_status":[{"model_type":5,"demo_audio":"https://x.bytespeech.com/fixture"}]}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := New(testKey)
	client.base = server.URL
	ctx := context.Background()
	trained, err := client.Train(ctx, testSpeaker, "wav", "你好", []byte("fixture-audio"))
	if err != nil || !trained.ModelReady || trained.DemoAudio != testDemo || trained.TrainingTimes != 14 {
		t.Fatalf("train status=%+v err=%v", trained, err)
	}
	status, err := client.Query(ctx, testSpeaker)
	if err != nil || status != trained || calls.Load() != 2 {
		t.Fatalf("query status=%+v calls=%d err=%v", status, calls.Load(), err)
	}
}

func TestUnsafeDemoURLRejected(t *testing.T) {
	for _, raw := range []string{"http://x.bytespeech.com/a", "https://evil.test/a", "https://x.bytespeech.com.evil.test/a"} {
		if got := safeDemoURL(raw); got != "" {
			t.Fatalf("accepted unsafe audition URL: %s", got)
		}
	}
}

func checkCloneRequest(t *testing.T, req *http.Request) bool {
	t.Helper()
	if req.Method != http.MethodPost || req.Header.Get("X-Api-Key") != testKey {
		t.Error("incorrect request method or API key")
	}
	if _, err := uuid.Parse(req.Header.Get("X-Api-Request-Id")); err != nil {
		t.Errorf("invalid request ID: %v", err)
	}
	var body requestBody
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Error(err)
		return false
	}
	if body.SpeakerID != customSpeaker || body.CustomSpeakerID != testSpeaker {
		t.Error("wrong speaker identity")
	}
	switch req.URL.Path {
	case clonePath:
		if body.Audio == nil || string(body.Audio.Data) != "fixture-audio" || body.Audio.Format != "wav" ||
			body.Language == nil || *body.Language != 0 || body.Extra == nil || body.Extra.PromptText != "你好" {
			t.Error("incorrect training sample")
		}
	case queryPath:
		if body.Audio != nil {
			t.Error("query uploaded audio")
		}
	default:
		t.Error("unexpected path")
	}
	return true
}
