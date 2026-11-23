package domain

import (
	"errors"
	"slices"
)

// ErrInvalidCapabilityConfig indicates an invalid device-type capability selection.
var ErrInvalidCapabilityConfig = errors.New("invalid device-type capability configuration")

// DeviceTypeCapabilities is a draft selection of resources for one product model.
// It does not grant execution permissions or represent a published configuration.
// Character selection remains owned by DeviceType; hardware capabilities remain
// independent of the Tool and Skill IDs declared here.
type DeviceTypeCapabilities struct {
	DeviceTypeID     int64   `json:"device_type_id"`
	VoiceID          int64   `json:"voice_id"`
	ToolIDs          []int64 `json:"tool_ids"`
	SkillIDs         []int64 `json:"skill_ids"`
	KnowledgeBaseIDs []int64 `json:"knowledge_base_ids"`
}

// NewDeviceTypeCapabilities validates and defensively copies draft references.
// VoiceID == 0 means no voice override. Resource IDs must be positive, unique
// within their own category, and reference validation belongs to the store.
func NewDeviceTypeCapabilities(deviceTypeID, voiceID int64, toolIDs, skillIDs, knowledgeBaseIDs []int64) (DeviceTypeCapabilities, error) {
	if deviceTypeID <= 0 || voiceID < 0 {
		return DeviceTypeCapabilities{}, ErrInvalidCapabilityConfig
	}
	tools, err := normalizeResourceIDs(toolIDs)
	if err != nil {
		return DeviceTypeCapabilities{}, err
	}
	skills, err := normalizeResourceIDs(skillIDs)
	if err != nil {
		return DeviceTypeCapabilities{}, err
	}
	knowledgeBases, err := normalizeResourceIDs(knowledgeBaseIDs)
	if err != nil {
		return DeviceTypeCapabilities{}, err
	}
	return DeviceTypeCapabilities{
		DeviceTypeID: deviceTypeID, VoiceID: voiceID,
		ToolIDs: tools, SkillIDs: skills, KnowledgeBaseIDs: knowledgeBases,
	}, nil
}

// Clone returns an independent copy for readers and in-memory persistence.
func (c DeviceTypeCapabilities) Clone() DeviceTypeCapabilities {
	c.ToolIDs = slices.Clone(c.ToolIDs)
	c.SkillIDs = slices.Clone(c.SkillIDs)
	c.KnowledgeBaseIDs = slices.Clone(c.KnowledgeBaseIDs)
	return c
}

func normalizeResourceIDs(ids []int64) ([]int64, error) {
	copied := slices.Clone(ids)
	slices.Sort(copied)
	for i, id := range copied {
		if id <= 0 || (i > 0 && id == copied[i-1]) {
			return nil, ErrInvalidCapabilityConfig
		}
	}
	return copied, nil
}
