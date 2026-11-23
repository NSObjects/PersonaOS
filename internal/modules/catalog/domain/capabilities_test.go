package domain

import (
	"errors"
	"reflect"
	"testing"
)

func TestNewDeviceTypeCapabilities(t *testing.T) {
	tests := []struct {
		name         string
		deviceTypeID int64
		voiceID      int64
		tools        []int64
		skills       []int64
		knowledge    []int64
		wantErr      bool
	}{
		{name: "empty selection is valid", deviceTypeID: 1},
		{name: "voice and all resources", deviceTypeID: 2, voiceID: 3, tools: []int64{9, 2}, skills: []int64{6}, knowledge: []int64{5, 4}},
		{name: "device type missing", voiceID: 3, wantErr: true},
		{name: "negative voice", deviceTypeID: 1, voiceID: -1, wantErr: true},
		{name: "zero tool", deviceTypeID: 1, tools: []int64{0}, wantErr: true},
		{name: "duplicate tools", deviceTypeID: 1, tools: []int64{4, 4}, wantErr: true},
		{name: "duplicate skills", deviceTypeID: 1, skills: []int64{2, 2}, wantErr: true},
		{name: "negative skill", deviceTypeID: 1, skills: []int64{-2}, wantErr: true},
		{name: "duplicate knowledge", deviceTypeID: 1, knowledge: []int64{5, 5}, wantErr: true},
		{name: "zero knowledge", deviceTypeID: 1, knowledge: []int64{0}, wantErr: true},
		{name: "same ID across categories is valid", deviceTypeID: 1, voiceID: 2, tools: []int64{2}, skills: []int64{2}, knowledge: []int64{2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewDeviceTypeCapabilities(tt.deviceTypeID, tt.voiceID, tt.tools, tt.skills, tt.knowledge)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error: got %v, want error=%v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidCapabilityConfig) {
					t.Fatalf("error: got %v, want %v", err, ErrInvalidCapabilityConfig)
				}
				return
			}
			if got.DeviceTypeID != tt.deviceTypeID || got.VoiceID != tt.voiceID {
				t.Fatalf("resource identity: got %+v, want device=%d voice=%d", got, tt.deviceTypeID, tt.voiceID)
			}
		})
	}
}

func TestDeviceTypeCapabilitiesDefensiveCopies(t *testing.T) {
	tools := []int64{3, 1}
	skills := []int64{6}
	knowledge := []int64{8}
	got, err := NewDeviceTypeCapabilities(1, 0, tools, skills, knowledge)
	if err != nil {
		t.Fatalf("new capability config: got error %v, want nil", err)
	}
	if want := []int64{1, 3}; !reflect.DeepEqual(got.ToolIDs, want) {
		t.Fatalf("sorted tool IDs: got %v, want %v", got.ToolIDs, want)
	}
	tools[0] = 99
	skills[0] = 99
	knowledge[0] = 99
	if got.ToolIDs[1] != 3 || got.SkillIDs[0] != 6 || got.KnowledgeBaseIDs[0] != 8 {
		t.Fatalf("input aliased: got %+v, want original IDs", got)
	}
	cloned := got.Clone()
	cloned.ToolIDs[0] = 99
	cloned.SkillIDs[0] = 99
	cloned.KnowledgeBaseIDs[0] = 99
	if got.ToolIDs[0] != 1 || got.SkillIDs[0] != 6 || got.KnowledgeBaseIDs[0] != 8 {
		t.Fatalf("clone aliased original: got %+v, want original IDs", got)
	}
}
