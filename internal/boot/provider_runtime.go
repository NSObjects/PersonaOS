package boot

import (
	"context"

	"github.com/NSObjects/go-speech/asr"
	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"

	agentdemousecase "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/voiceconfig"
	"github.com/NSObjects/PersonaOS/internal/modules/providerconfig"
)

func speechRuntime(store *providerconfig.Store) func(context.Context) (asr.StreamRecognizer, agentdemousecase.VoiceSynthFactory, audio.Format, error) {
	return func(ctx context.Context) (asr.StreamRecognizer, agentdemousecase.VoiceSynthFactory, audio.Format, error) {
		snapshot, err := store.Snapshot(ctx)
		if err != nil {
			return nil, nil, audio.Format{}, err
		}
		return speechFromSnapshot(snapshot.Get)
	}
}

// A missing or invalid ASR configuration must not stop text dialogue or
// independent TTS previews. Runtime status reports ASR as unavailable.
func speechFromSnapshot(get func(string) string) (asr.StreamRecognizer, agentdemousecase.VoiceSynthFactory, audio.Format, error) {
	recognizer, err := voiceconfig.NewASR(get)
	if err != nil {
		recognizer = nil
	}
	factory := func(provider string) (tts.Client, error) {
		return voiceconfig.NewTTS(get, provider)
	}
	return recognizer, factory, voiceconfig.InputFormat(get), nil
}
