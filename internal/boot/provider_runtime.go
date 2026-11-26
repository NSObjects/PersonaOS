package boot

import (
	"context"
	"os"
	"time"

	"github.com/NSObjects/go-speech/asr"
	"github.com/NSObjects/go-speech/audio"
	"github.com/NSObjects/go-speech/tts"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/adapters/redisrag"
	agentdemousecase "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/voiceconfig"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/adapters/volcclone"
	catalogusecase "github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
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

// A missing or invalid ASR configuration must not stop text dialog or
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

func chatRuntime(store *providerconfig.Store, bundle *Resources, endpoints map[string]string) agentdemousecase.ChatRuntimeResolver {
	return func(ctx context.Context) (model.BaseChatModel, agentdemousecase.KnowledgeRetriever, *agentdemousecase.ToolRegistry, error) {
		snapshot, err := store.Snapshot(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		registry := agentdemousecase.NewToolRegistry(endpoints).WithNetworkTools(
			os.Getenv("PERSONAOS_OPEN_METEO_DEMO") == "1", snapshot.Get("DEEPSEEK_API_KEY"),
		)
		chatModel, err := configuredChatModel(ctx, snapshot.Get)
		if err != nil {
			return nil, nil, nil, err
		}
		var knowledge agentdemousecase.KnowledgeRetriever
		if backend := runtimeKnowledge(ctx, bundle, snapshot.Get("PERSONAOS_EMBEDDING_API_KEY")); backend != nil {
			knowledge = backend
		}
		return chatModel, knowledge, registry, nil
	}
}

func configuredChatModel(ctx context.Context, get func(string) string) (model.BaseChatModel, error) {
	key, name := get("PERSONAOS_LLM_API_KEY"), get("PERSONAOS_LLM_MODEL")
	if key == "" || name == "" {
		return nil, nil
	}
	return openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey: key, Model: name, BaseURL: get("PERSONAOS_LLM_BASE_URL"),
		Timeout: 40 * time.Second,
	})
}

func memoryModelRuntime(store *providerconfig.Store) agentdemousecase.MemoryModelResolver {
	return func(ctx context.Context) (model.BaseChatModel, error) {
		snapshot, err := store.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		return configuredChatModel(ctx, snapshot.Get)
	}
}

func resourceRuntime(store *providerconfig.Store, bundle *Resources) catalogusecase.ResourceRuntimeResolver {
	return func(ctx context.Context) (catalogusecase.VoiceCloner, catalogusecase.KnowledgeIndexer, catalogusecase.KnowledgeInspector, error) {
		snapshot, err := store.Snapshot(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		var cloner catalogusecase.VoiceCloner
		if key := snapshot.Get("VOLCENGINE_API_KEY"); key != "" {
			cloner = volcclone.New(key)
		}
		backend := runtimeKnowledge(ctx, bundle, snapshot.Get("PERSONAOS_EMBEDDING_API_KEY"))
		if backend == nil {
			return cloner, nil, nil, nil
		}
		return cloner, backend, backend, nil
	}
}

// Missing Redis or embedding configuration disables knowledge, not chat.
func runtimeKnowledge(ctx context.Context, bundle *Resources, key string) *redisrag.Store {
	if key == "" {
		return nil
	}
	client, ok := bundle.Redis()
	if !ok {
		return nil
	}
	embedder, err := redisrag.NewArkEmbedder(ctx, key)
	if err != nil {
		return nil
	}
	backend, err := redisrag.New(ctx, client, embedder)
	if err != nil {
		return nil
	}
	return backend
}
