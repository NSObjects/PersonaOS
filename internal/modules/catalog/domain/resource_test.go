package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewResource(t *testing.T) {
	tests := []struct {
		name      string
		kind      ResourceKind
		code      string
		title     string
		provider  string
		reference string
		wantErr   bool
	}{
		{name: "voice", kind: ResourceVoice, code: "Voice_A", title: "Voice A", provider: "aliyun", reference: "voice-a"},
		{name: "tool", kind: ResourceTool, code: "observe_scene", title: "Observe Scene", reference: "tools.observe_scene"},
		{name: "skill", kind: ResourceSkill, code: "guide", title: "Guide", reference: "skills.guide"},
		{name: "knowledge base", kind: ResourceKnowledgeBase, code: "game_guide", title: "Game Guide", reference: "kb.game_guide"},
		{name: "unknown kind", kind: "camera", code: "camera", title: "Camera", reference: "camera", wantErr: true},
		{name: "empty code", kind: ResourceTool, title: "Tool", reference: "handler", wantErr: true},
		{name: "bad code", kind: ResourceTool, code: "bad code", title: "Tool", reference: "handler", wantErr: true},
		{name: "empty name", kind: ResourceTool, code: "test", reference: "handler", wantErr: true},
		{name: "empty reference", kind: ResourceTool, code: "test", title: "Tool", wantErr: true},
		{name: "voice requires provider", kind: ResourceVoice, code: "test", title: "Voice", reference: "voice", wantErr: true},
		{name: "reference newline", kind: ResourceTool, code: "test", title: "Tool", reference: "a\nb", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewResource(0, tt.kind, tt.code, tt.title, "", tt.provider, tt.reference, true, time.Time{}, time.Time{})
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewResource() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidResource) {
					t.Fatalf("NewResource() error = %v, want ErrInvalidResource", err)
				}
				return
			}
			if got.Kind != tt.kind || got.Reference != tt.reference {
				t.Fatalf("NewResource() = %+v, want kind %s, reference %s", got, tt.kind, tt.reference)
			}
		})
	}
}

func TestNewResourceNormalizesCode(t *testing.T) {
	got, err := NewResource(1, ResourceVoice, " VOICE_A ", " Demo ", "", " aliyun ", " voice-a ", false, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("NewResource() error = %v, want nil", err)
	}
	if got.Code != "voice_a" || got.Name != "Demo" || got.Provider != "aliyun" || got.Reference != "voice-a" {
		t.Fatalf("NewResource() = %+v, want normalized fields", got)
	}
}
