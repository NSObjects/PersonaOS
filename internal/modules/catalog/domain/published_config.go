package domain

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// ErrInvalidPublishedConfig indicates a snapshot whose references or metadata
// cannot be safely published.
var ErrInvalidPublishedConfig = errors.New("invalid published device configuration")

// PublishSnapshotInput contains a fully resolved, untrusted candidate for
// one version of one device type. The publisher must load these rows within
// the same database transaction that assigns the version.
type PublishSnapshotInput struct {
	Version        int64
	PublishedBy    int64
	PublishedAt    time.Time
	DeviceType     DeviceType
	Character      Character
	Draft          DeviceTypeCapabilities
	Voice          *Resource
	Tools          []Resource
	Skills         []Resource
	KnowledgeBases []Resource
}

// PublishedDeviceConfig captures the configuration at publish time. Read
// boundaries must use Clone; stored versions must never be updated in place.
// A snapshot is data, not authorization to execute Tool or Skill handlers.
type PublishedDeviceConfig struct {
	Version        int64                  `json:"version"`
	PublishedBy    int64                  `json:"published_by"`
	PublishedAt    time.Time              `json:"published_at"`
	DeviceType     DeviceType             `json:"device_type"`
	Character      Character              `json:"character"`
	Capabilities   DeviceTypeCapabilities `json:"capabilities"`
	Voice          *Resource              `json:"voice,omitempty"`
	Tools          []Resource             `json:"tools"`
	Skills         []Resource             `json:"skills"`
	KnowledgeBases []Resource             `json:"knowledge_bases"`
}

// NewPublishedDeviceConfig validates all references and copies the resolved
// metadata so future catalog edits cannot mutate an existing release.
func NewPublishedDeviceConfig(in PublishSnapshotInput) (PublishedDeviceConfig, error) {
	if in.Version <= 0 || in.PublishedBy <= 0 || in.PublishedAt.IsZero() {
		return PublishedDeviceConfig{}, fmt.Errorf("%w: missing version, publisher or timestamp", ErrInvalidPublishedConfig)
	}
	device, character, draft, err := validatePublishIdentity(in)
	if err != nil {
		return PublishedDeviceConfig{}, err
	}
	voice, err := validatePublishedVoice(draft.VoiceID, in.Voice)
	if err != nil {
		return PublishedDeviceConfig{}, err
	}
	tools, err := validatePublishedResources(draft.ToolIDs, in.Tools, ResourceTool)
	if err != nil {
		return PublishedDeviceConfig{}, err
	}
	skills, err := validatePublishedResources(draft.SkillIDs, in.Skills, ResourceSkill)
	if err != nil {
		return PublishedDeviceConfig{}, err
	}
	knowledge, err := validatePublishedResources(draft.KnowledgeBaseIDs, in.KnowledgeBases, ResourceKnowledgeBase)
	if err != nil {
		return PublishedDeviceConfig{}, err
	}
	return PublishedDeviceConfig{
		Version: in.Version, PublishedBy: in.PublishedBy, PublishedAt: in.PublishedAt.UTC(),
		DeviceType: device, Character: character, Capabilities: draft,
		Voice: voice, Tools: tools, Skills: skills, KnowledgeBases: knowledge,
	}, nil
}

func validatePublishIdentity(in PublishSnapshotInput) (DeviceType, Character, DeviceTypeCapabilities, error) {
	if in.DeviceType.ID <= 0 || in.Character.ID <= 0 || in.DeviceType.CharacterID != in.Character.ID {
		return DeviceType{}, Character{}, DeviceTypeCapabilities{}, fmt.Errorf("%w: device and character mismatch", ErrInvalidPublishedConfig)
	}
	device, err := NewDeviceType(in.DeviceType.ID, in.DeviceType.Code, in.DeviceType.Name,
		in.DeviceType.Description, in.DeviceType.CharacterID, in.DeviceType.HasCamera,
		in.DeviceType.HasMicrophone, in.DeviceType.HasSpeaker, in.DeviceType.CreatedAt, in.DeviceType.UpdatedAt)
	if err != nil {
		return DeviceType{}, Character{}, DeviceTypeCapabilities{}, fmt.Errorf("%w: invalid device type", ErrInvalidPublishedConfig)
	}
	character, err := NewCharacter(in.Character.ID, in.Character.Code, in.Character.Name,
		in.Character.Persona, in.Character.Description, in.Character.CreatedAt, in.Character.UpdatedAt)
	if err != nil {
		return DeviceType{}, Character{}, DeviceTypeCapabilities{}, fmt.Errorf("%w: invalid character", ErrInvalidPublishedConfig)
	}
	draft, err := NewDeviceTypeCapabilities(in.Draft.DeviceTypeID, in.Draft.VoiceID,
		in.Draft.ToolIDs, in.Draft.SkillIDs, in.Draft.KnowledgeBaseIDs)
	if err != nil || draft.DeviceTypeID != device.ID {
		return DeviceType{}, Character{}, DeviceTypeCapabilities{}, fmt.Errorf("%w: invalid device draft", ErrInvalidPublishedConfig)
	}
	return device, character, draft, nil
}

func validatePublishedVoice(id int64, candidate *Resource) (*Resource, error) {
	if id == 0 && candidate == nil {
		return nil, nil
	}
	if candidate == nil || candidate.ID != id {
		return nil, fmt.Errorf("%w: voice reference mismatch", ErrInvalidPublishedConfig)
	}
	normalized, err := validatePublishResource(*candidate, ResourceVoice)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func validatePublishedResources(ids []int64, candidates []Resource, kind ResourceKind) ([]Resource, error) {
	if len(ids) != len(candidates) {
		return nil, fmt.Errorf("%w: incomplete %s references", ErrInvalidPublishedConfig, kind)
	}
	resolved := make([]Resource, 0, len(candidates))
	for _, candidate := range candidates {
		normalized, err := validatePublishResource(candidate, kind)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, normalized)
	}
	slices.SortFunc(resolved, func(a, b Resource) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	for i, id := range ids {
		if resolved[i].ID != id {
			return nil, fmt.Errorf("%w: mismatched or repeated %s reference", ErrInvalidPublishedConfig, kind)
		}
	}
	return resolved, nil
}

func validatePublishResource(candidate Resource, kind ResourceKind) (Resource, error) {
	if candidate.ID <= 0 || candidate.Kind != kind || !candidate.Enabled {
		return Resource{}, fmt.Errorf("%w: unavailable %s resource", ErrInvalidPublishedConfig, kind)
	}
	resource, err := NewResource(candidate.ID, candidate.Kind, candidate.Code, candidate.Name,
		candidate.Description, candidate.Provider, candidate.Reference, candidate.Enabled,
		candidate.CreatedAt, candidate.UpdatedAt)
	if err != nil {
		return Resource{}, fmt.Errorf("%w: invalid %s resource", ErrInvalidPublishedConfig, kind)
	}
	return resource, nil
}

// Clone separates every mutable slice and optional pointer from the source
// snapshot. Version history should return a clone to callers.
func (p PublishedDeviceConfig) Clone() PublishedDeviceConfig {
	p.Capabilities = p.Capabilities.Clone()
	if p.Voice != nil {
		voice := *p.Voice
		p.Voice = &voice
	}
	p.Tools = slices.Clone(p.Tools)
	p.Skills = slices.Clone(p.Skills)
	p.KnowledgeBases = slices.Clone(p.KnowledgeBases)
	return p
}
