package boot

import (
	"context"
	"errors"
	"io"
	"maps"
	"os"
	"strings"
	"sync"

	"github.com/NSObjects/go-speech/asr"
	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"
	"github.com/rs/zerolog/log"

	agentdemo "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/voiceconfig"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

// speechClientRuntime owns SDK clients, not WebSocket connections. A retired
// configuration remains usable until all requests holding it release it.
type speechClientRuntime struct {
	mu          sync.Mutex
	load        func(context.Context) (func(string) string, error)
	current     *speechClients
	generations map[*speechClients]struct{}
	closed      bool
}

type speechClients struct {
	settings   map[string]string
	recognizer asr.StreamRecognizer
	format     audio.Format
	synths     map[string]speechSynth
	refs       int
}

type speechSynth struct {
	client tts.Client
	err    error
}

func newSpeechClientRuntime(load func(context.Context) (func(string) string, error)) *speechClientRuntime {
	return &speechClientRuntime{load: load, generations: make(map[*speechClients]struct{})}
}

func speechSettings(get func(string) string) map[string]string {
	settings := make(map[string]string)
	for _, key := range []string{
		"PERSONAOS_ASR_PROVIDER", "DASHSCOPE_API_KEY",
		"VOLCENGINE_API_KEY", "VOLCENGINE_ASR_RESOURCE_ID", "VOLCENGINE_TTS_MODEL_ID",
		"TENCENT_APP_ID", "TENCENT_SECRET_ID", "TENCENT_SECRET_KEY",
	} {
		settings[key] = get(key)
	}
	// The endpoint remains a deployment-only setting, outside the database.
	settings["DASHSCOPE_WS_ENDPOINT"] = os.Getenv("DASHSCOPE_WS_ENDPOINT")
	return settings
}

func newSpeechClients(settings map[string]string) *speechClients {
	get := func(key string) string { return settings[key] }
	recognizer, err := voiceconfig.NewASR(get)
	if err != nil {
		recognizer = nil // Invalid ASR must not disable independent TTS.
	}
	generation := &speechClients{
		settings: settings, recognizer: recognizer, format: voiceconfig.InputFormat(get),
		synths: make(map[string]speechSynth),
	}
	// SDK constructors validate configuration without opening sockets.
	for _, provider := range voiceconfig.TTSProviders() {
		client, err := voiceconfig.NewTTS(get, provider)
		generation.synths[provider] = speechSynth{client: client, err: err}
	}
	return generation
}

func (g *speechClients) synth(provider string) (tts.Client, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == domain.Seed3CloneProvider {
		provider = "volcengine"
	}
	entry, ok := g.synths[provider]
	if !ok {
		return nil, errors.New("unsupported configured voice provider")
	}
	return entry.client, entry.err
}

func (r *speechClientRuntime) resolve(ctx context.Context) (agentdemo.VoiceRuntime, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return agentdemo.VoiceRuntime{}, errors.New("speech runtime is closed")
	}
	// Serialize the configuration read and rotation so a slow old snapshot
	// cannot replace a newer generation. No provider network calls occur here.
	get, err := r.load(ctx)
	if err != nil {
		return agentdemo.VoiceRuntime{}, err
	}
	settings := speechSettings(get)
	if r.current == nil || !maps.Equal(r.current.settings, settings) {
		previous := r.current
		r.current = newSpeechClients(settings)
		r.generations[r.current] = struct{}{}
		r.retire(previous)
	}
	generation := r.current
	generation.refs++
	return agentdemo.VoiceRuntime{
		Recognizer: generation.recognizer, Factory: generation.synth, Format: generation.format,
		Release: sync.OnceFunc(func() { r.release(generation) }),
	}, nil
}

func (r *speechClientRuntime) release(generation *speechClients) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.generations[generation]; !ok {
		return
	}
	generation.refs--
	r.retire(generation)
}

// retire is called with the runtime lock held.
func (r *speechClientRuntime) retire(generation *speechClients) {
	if generation == nil || generation == r.current || generation.refs != 0 {
		return
	}
	delete(r.generations, generation)
	if err := generation.close(); err != nil {
		log.Warn().Err(err).Msg("close retired speech clients")
	}
}

func (g *speechClients) close() error {
	err := closeSpeechClient(g.recognizer)
	for _, entry := range g.synths {
		err = errors.Join(err, closeSpeechClient(entry.client))
	}
	return err
}

func closeSpeechClient(client any) error {
	if closer, ok := client.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Shutdown is called by the application dependency injector.
func (r *speechClientRuntime) Shutdown(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	var err error
	for generation := range r.generations {
		err = errors.Join(err, generation.close())
		delete(r.generations, generation)
	}
	r.current = nil
	return err
}
