package domain

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

const publishTestTime = "2026-11-23T10:00:00Z"

func publishFixture(t *testing.T) PublishSnapshotInput {
	t.Helper()
	publishedAt, err := time.Parse(time.RFC3339, publishTestTime)
	if err != nil {
		t.Fatal(err)
	}
	return PublishSnapshotInput{
		Version: 1, PublishedBy: 10, PublishedAt: publishedAt,
		DeviceType: DeviceType{ID: 3, Code: "A3", Name: "Base", CharacterID: 2, HasSpeaker: true},
		Character:  Character{ID: 2, Code: "hero", Name: "Hero", Persona: "Helpful"},
		Draft: DeviceTypeCapabilities{
			DeviceTypeID: 3, VoiceID: 11,
			ToolIDs: []int64{8, 4}, SkillIDs: []int64{6}, KnowledgeBaseIDs: []int64{9},
		},
		Voice: &Resource{ID: 11, Kind: ResourceVoice, Code: "voice_a",
			Name: "Voice A", Provider: "aliyun", Reference: "voice-a", Enabled: true},
		Tools: []Resource{
			{ID: 8, Kind: ResourceTool, Code: "tool_b", Name: "Tool B", Reference: "handler-b", Enabled: true},
			{ID: 4, Kind: ResourceTool, Code: "tool_a", Name: "Tool A", Reference: "handler-a", Enabled: true},
		},
		Skills: []Resource{
			{ID: 6, Kind: ResourceSkill, Code: "skill_a", Name: "Skill A", Reference: "skill-a", Enabled: true},
		},
		KnowledgeBases: []Resource{
			{ID: 9, Kind: ResourceKnowledgeBase, Code: "kb_a", Name: "KB A", Reference: "index-a", Enabled: true},
		},
	}
}

func TestPublishedDeviceConfigCapturesResolvedVersion(t *testing.T) {
	in := publishFixture(t)
	published, err := NewPublishedDeviceConfig(in)
	if err != nil {
		t.Fatalf("NewPublishedDeviceConfig() = %v", err)
	}
	if published.Version != 1 || published.PublishedBy != 10 ||
		published.DeviceType.ID != 3 || published.Character.ID != 2 ||
		published.Voice.ID != 11 {
		t.Fatalf("published metadata = %+v", published)
	}
	if !reflect.DeepEqual(published.Capabilities.ToolIDs, []int64{4, 8}) ||
		published.Tools[0].ID != 4 || published.Tools[1].ID != 8 {
		t.Fatalf("published resources should be canonical: %+v", published)
	}
	if !published.PublishedAt.Equal(in.PublishedAt) ||
		published.PublishedAt.Location() != time.UTC {
		t.Fatalf("published timestamp = %v", published.PublishedAt)
	}
}

func TestPublishedDeviceConfigCopiesReferences(t *testing.T) {
	in := publishFixture(t)
	published, err := NewPublishedDeviceConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Draft.ToolIDs[0] = 999
	in.Voice.Reference = "mutated"
	in.Tools[0].Reference = "mutated"
	in.Skills[0].Name = "mutated"
	in.KnowledgeBases[0].Reference = "mutated"
	if published.Capabilities.ToolIDs[1] != 8 || published.Voice.Reference != "voice-a" ||
		published.Tools[1].Reference != "handler-b" ||
		published.Skills[0].Name != "Skill A" ||
		published.KnowledgeBases[0].Reference != "index-a" {
		t.Fatalf("published snapshot aliases original: %+v", published)
	}
	clone := published.Clone()
	clone.Capabilities.ToolIDs[0] = 99
	clone.Voice.Reference = "changed"
	clone.Tools[0].Reference = "changed"
	clone.Skills[0].Reference = "changed"
	clone.KnowledgeBases[0].Reference = "changed"
	if published.Capabilities.ToolIDs[0] != 4 || published.Voice.Reference != "voice-a" ||
		published.Tools[0].Reference != "handler-a" ||
		published.Skills[0].Reference != "skill-a" ||
		published.KnowledgeBases[0].Reference != "index-a" {
		t.Fatalf("Clone() mutated published snapshot: %+v", published)
	}
}

func TestPublishedDeviceConfigKeepsDeviceTypesIndependent(t *testing.T) {
	a3 := publishFixture(t)
	a4 := publishFixture(t)
	a4.DeviceType.ID = 4
	a4.DeviceType.Code = "A4"
	a4.DeviceType.HasCamera = true
	a4.Draft = DeviceTypeCapabilities{DeviceTypeID: 4, ToolIDs: []int64{4}}
	a4.Voice = nil
	a4.Tools = a4.Tools[1:]
	a4.Skills = nil
	a4.KnowledgeBases = nil

	first, err := NewPublishedDeviceConfig(a3)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewPublishedDeviceConfig(a4)
	if err != nil {
		t.Fatal(err)
	}
	if first.Character.ID != second.Character.ID || first.DeviceType.HasCamera ||
		!second.DeviceType.HasCamera || second.Voice != nil || len(second.Tools) != 1 ||
		len(second.Skills) != 0 || first.DeviceType.ID == second.DeviceType.ID {
		t.Fatalf("A3 and A4 configurations not isolated: A3=%+v A4=%+v", first, second)
	}
}

func TestPublishedDeviceConfigRejectsInvalidIdentity(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*PublishSnapshotInput)
	}{
		{"missing version", func(in *PublishSnapshotInput) { in.Version = 0 }},
		{"missing publisher", func(in *PublishSnapshotInput) { in.PublishedBy = 0 }},
		{"missing timestamp", func(in *PublishSnapshotInput) { in.PublishedAt = time.Time{} }},
		{"missing device", func(in *PublishSnapshotInput) { in.DeviceType.ID = 0 }},
		{"character mismatch", func(in *PublishSnapshotInput) { in.Character.ID = 23 }},
		{"invalid character", func(in *PublishSnapshotInput) { in.Character.Persona = "" }},
		{"invalid device", func(in *PublishSnapshotInput) { in.DeviceType.Name = "" }},
		{"draft mismatch", func(in *PublishSnapshotInput) { in.Draft.DeviceTypeID = 4 }},
		{"duplicate draft IDs", func(in *PublishSnapshotInput) { in.Draft.ToolIDs = []int64{8, 8} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := publishFixture(t)
			tc.mutate(&in)
			_, err := NewPublishedDeviceConfig(in)
			if !errors.Is(err, ErrInvalidPublishedConfig) {
				t.Fatalf("NewPublishedDeviceConfig() error = %v, want ErrInvalidPublishedConfig", err)
			}
		})
	}
}

func TestPublishedDeviceConfigRejectsUnavailableResources(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*PublishSnapshotInput)
	}{
		{"missing voice", func(in *PublishSnapshotInput) { in.Voice = nil }},
		{"wrong voice", func(in *PublishSnapshotInput) { in.Voice.Kind = ResourceTool }},
		{"disabled voice", func(in *PublishSnapshotInput) { in.Voice.Enabled = false }},
		{"unused voice", func(in *PublishSnapshotInput) { in.Draft.VoiceID = 0 }},
		{"missing tool", func(in *PublishSnapshotInput) { in.Tools = in.Tools[:1] }},
		{"extra tool", func(in *PublishSnapshotInput) { in.Tools = append(in.Tools, in.Tools[0]) }},
		{"wrong tool kind", func(in *PublishSnapshotInput) { in.Tools[0].Kind = ResourceSkill }},
		{"wrong tool ID", func(in *PublishSnapshotInput) { in.Tools[0].ID = 99 }},
		{"disabled tool", func(in *PublishSnapshotInput) { in.Tools[0].Enabled = false }},
		{"invalid tool reference", func(in *PublishSnapshotInput) { in.Tools[0].Reference = "" }},
		{"wrong skill", func(in *PublishSnapshotInput) { in.Skills[0].ID = 99 }},
		{"disabled knowledge", func(in *PublishSnapshotInput) { in.KnowledgeBases[0].Enabled = false }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := publishFixture(t)
			tc.mutate(&in)
			_, err := NewPublishedDeviceConfig(in)
			if !errors.Is(err, ErrInvalidPublishedConfig) {
				t.Fatalf("NewPublishedDeviceConfig() error = %v, want ErrInvalidPublishedConfig", err)
			}
		})
	}
}

func TestPublishedDeviceConfigAllowsEmptyCapabilities(t *testing.T) {
	in := publishFixture(t)
	in.Draft = DeviceTypeCapabilities{DeviceTypeID: in.DeviceType.ID}
	in.Voice, in.Tools, in.Skills, in.KnowledgeBases = nil, nil, nil, nil
	result, err := NewPublishedDeviceConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	if result.Voice != nil || len(result.Tools) != 0 || len(result.Skills) != 0 || len(result.KnowledgeBases) != 0 {
		t.Fatalf("expected empty published resource selection, got %+v", result)
	}
}
