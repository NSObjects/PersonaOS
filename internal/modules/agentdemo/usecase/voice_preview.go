package usecase

import (
	"context"
	"strings"

	"github.com/NSObjects/go-speech/audio"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

const maxVoicePreviewBytes = 600

// VoiceResourceReader reads a saved reusable resource without applying it.
type VoiceResourceReader interface {
	Find(context.Context, int64) (domain.Resource, error)
}

// WithVoiceResources enables independent preview of saved voice resources.
func (u *VoiceUsecase) WithVoiceResources(resources VoiceResourceReader) *VoiceUsecase {
	u.resources = resources
	return u
}

// PreviewVoice uses the saved provider and voice ID. Browser-supplied
// endpoints, credentials, or unpublished form values are never used.
func (u *VoiceUsecase) PreviewVoice(ctx context.Context, id int64, text string, preferOpus bool) (audio.Buffer, error) {
	text = strings.TrimSpace(text)
	if ctx == nil || id <= 0 || text == "" || len(text) > maxVoicePreviewBytes {
		return audio.Buffer{}, apperr.NewBadRequest("invalid voice preview request")
	}
	if u == nil || u.resources == nil || u.newSynth == nil {
		return audio.Buffer{}, apperr.NewBadRequest("voice preview is not configured")
	}
	voice, err := u.resources.Find(ctx, id)
	if err != nil {
		return audio.Buffer{}, err
	}
	if voice.Kind != domain.ResourceVoice || !voice.Enabled {
		return audio.Buffer{}, apperr.NewBadRequest("select an enabled voice resource")
	}
	select {
	case u.previewSlots <- struct{}{}:
		defer func() { <-u.previewSlots }()
	default:
		return audio.Buffer{}, apperr.NewConflict("voice preview is busy")
	}
	return u.synthesizeVoice(ctx, voice, text, preferOpus)
}
