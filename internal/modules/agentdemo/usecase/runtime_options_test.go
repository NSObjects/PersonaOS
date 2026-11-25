package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimeResourceOptionsReflectRegistry(t *testing.T) {
	const privateEndpoint = "https://private.example.test/query"
	registry := NewToolRegistry(map[string]string{"approved": privateEndpoint}).WithNetworkTools(true, "")
	u := NewChatUsecase(nil, nil).WithToolRegistry(registry)
	tools, skills, err := u.ResourceOptions(context.Background())
	if err != nil || len(tools) != 3 || len(skills) != 1 {
		t.Fatalf("tools=%+v skills=%+v err=%v", tools, skills, err)
	}
	for _, item := range tools {
		if item.Reference == webSearchToolName {
			t.Fatal("unconfigured network tool is selectable")
		}
	}
	if skills[0].Reference != storytellingSkillName {
		t.Fatalf("skill=%+v", skills[0])
	}
	data, err := json.Marshal(tools)
	if err != nil || strings.Contains(string(data), privateEndpoint) {
		t.Fatalf("options leaked endpoint: %s err=%v", data, err)
	}
}

func TestRuntimeStatusReportsConfiguredDependenciesOnly(t *testing.T) {
	const provider = "unknown"
	u := NewChatUsecase(nil, nil)
	status := u.ConfigurationStatus(nil, []string{provider})
	if status.LLM || status.ASR || status.Knowledge || status.TTS[provider] {
		t.Fatalf("unexpected initialized dependencies: %+v", status)
	}
	u = NewChatUsecase(nil, &fakeChatModel{})
	if !u.ConfigurationStatus(nil, nil).LLM {
		t.Fatal("initialized model not reported")
	}
}
