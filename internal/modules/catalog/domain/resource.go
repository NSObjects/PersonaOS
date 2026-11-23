package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// ErrInvalidResource indicates an invalid reusable capability definition.
var ErrInvalidResource = errors.New("invalid capability resource")

// ResourceKind identifies a reusable, independently managed capability.
type ResourceKind string

const (
	// ResourceVoice identifies a TTS voice entry.
	ResourceVoice         ResourceKind = "voice"
	// ResourceTool identifies an approved agent tool handler.
	ResourceTool          ResourceKind = "tool"
	// ResourceSkill identifies an installed agent skill.
	ResourceSkill         ResourceKind = "skill"
	// ResourceKnowledgeBase identifies a retrieval knowledge base.
	ResourceKnowledgeBase ResourceKind = "knowledge_base"
)

var resourceCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

// Resource represents one catalog entry, not a published device configuration.
// Reference is an adapter-owned lookup key, never a secret or credential.
// For voice it identifies the provider voice; for tool, an approved handler;
// for skill, an installed skill key; for knowledge_base, a retrieval index key.
type Resource struct {
	ID          int64        `json:"id"`
	Kind        ResourceKind `json:"kind"`
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Provider    string       `json:"provider"`
	Reference   string       `json:"reference"`
	Enabled     bool         `json:"enabled"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// NewResource validates metadata without binding or executing an integration.
// No tokens, API keys, passwords or executable scripts belong in Reference.
func NewResource(id int64, kind ResourceKind, code, name, description, provider, reference string, enabled bool, createdAt, updatedAt time.Time) (Resource, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	provider = strings.TrimSpace(provider)
	reference = strings.TrimSpace(reference)
	switch kind {
	case ResourceVoice, ResourceTool, ResourceSkill, ResourceKnowledgeBase:
	default:
		return Resource{}, ErrInvalidResource
	}
	if id < 0 || !resourceCodePattern.MatchString(code) ||
		name == "" || len(name) > 160 || len(description) > 2000 ||
		reference == "" || len(reference) > 512 || len(provider) > 120 ||
		strings.ContainsAny(reference, "\r\n\x00") || strings.ContainsAny(provider, "\r\n\x00") {
		return Resource{}, ErrInvalidResource
	}
	if kind == ResourceVoice && provider == "" {
		return Resource{}, ErrInvalidResource
	}
	return Resource{
		ID: id, Kind: kind, Code: code, Name: name,
		Description: description, Provider: provider, Reference: reference,
		Enabled: enabled, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}, nil
}
