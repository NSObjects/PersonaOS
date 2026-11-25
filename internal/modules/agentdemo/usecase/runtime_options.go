package usecase

import (
	"context"
	"maps"
	"slices"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

// ResourceOption is an installed executor or an approved HTTP endpoint key.
// It deliberately excludes endpoint URLs and credentials.
type ResourceOption struct {
	Provider    string `json:"provider"`
	Reference   string `json:"reference"`
	Description string `json:"description,omitempty"`
}

// ResourceOptions returns the actual registry entries, not guessed tool names.
func (u *ChatUsecase) ResourceOptions(ctx context.Context) ([]ResourceOption, []ResourceOption, error) {
	r := u.toolRegistry
	tools := make([]ResourceOption, 0, len(r.builtins)+len(r.endpoints))
	for _, key := range slices.Sorted(maps.Keys(r.builtins)) {
		info, err := r.builtins[key].Info(ctx)
		if err != nil {
			return nil, nil, err
		}
		tools = append(tools, ResourceOption{Provider: builtinToolProvider, Reference: key, Description: info.Desc})
	}
	for _, key := range slices.Sorted(maps.Keys(r.endpoints)) {
		tools = append(tools, ResourceOption{Provider: domain.HTTPToolProvider, Reference: key})
	}
	skills := []ResourceOption{{Reference: storytellingSkillName, Description: storytellingDescription}}
	return tools, skills, nil
}

// RuntimeStatus reports initialized dependencies only. It is not a paid
// provider health probe and does not promise that credentials are accepted.
type RuntimeStatus struct {
	LLM       bool            `json:"llm"`
	ASR       bool            `json:"asr"`
	Knowledge bool            `json:"knowledge"`
	TTS       map[string]bool `json:"tts"`
}

// ConfigurationStatus reports which dependencies are configured without calls
// to external providers. The caller supplies the supported provider names.
func (u *ChatUsecase) ConfigurationStatus(voice *VoiceUsecase, providers []string) RuntimeStatus {
	status := RuntimeStatus{
		LLM:       u != nil && u.model != nil,
		ASR:       voice != nil && voice.recognizer != nil,
		Knowledge: u != nil && u.knowledge != nil,
		TTS:       make(map[string]bool, len(providers)),
	}
	for _, provider := range providers {
		status.TTS[provider] = voice.ttsConfigured(provider)
	}
	return status
}

func (u *VoiceUsecase) ttsConfigured(provider string) bool {
	if u == nil || u.newSynth == nil {
		return false
	}
	client, err := u.newSynth(provider)
	return err == nil && client != nil
}
