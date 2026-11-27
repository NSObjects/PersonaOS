package boot

import (
	"context"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/adapters/redisrag"
	agentdemousecase "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/adapters/volcclone"
	catalogusecase "github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/modules/providerconfig"
)

func chatRuntime(store *providerconfig.Store, bundle *Resources, endpoints map[string]string) agentdemousecase.ChatRuntimeResolver {
	return func(ctx context.Context) (agentdemousecase.ChatRuntime, error) {
		snapshot, err := store.Snapshot(ctx)
		if err != nil {
			return agentdemousecase.ChatRuntime{}, err
		}
		registry := agentdemousecase.NewToolRegistry(endpoints).WithWebSearch(snapshot.Get("DEEPSEEK_API_KEY"))
		chatModel, extra, err := configuredChatModel(ctx, snapshot.Get)
		if err != nil {
			return agentdemousecase.ChatRuntime{}, err
		}
		var knowledge agentdemousecase.KnowledgeRetriever
		if backend := runtimeKnowledge(ctx, bundle, snapshot.Get("PERSONAOS_EMBEDDING_API_KEY")); backend != nil {
			knowledge = backend
		}
		return agentdemousecase.ChatRuntime{Model: chatModel, ModelOptions: extra, Knowledge: knowledge, Tools: registry}, nil
	}
}

func configuredChatModel(ctx context.Context, get func(string) string) (model.BaseChatModel, map[string]any, error) {
	key, name := get("PERSONAOS_LLM_API_KEY"), get("PERSONAOS_LLM_MODEL")
	if key == "" || name == "" {
		return nil, nil, nil
	}
	extra, err := providerconfig.ParseLLMExtraFields(get(providerconfig.EnvLLMExtraFields))
	if err != nil {
		return nil, nil, err
	}
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey: key, Model: name, BaseURL: get("PERSONAOS_LLM_BASE_URL"),
		Timeout: 40 * time.Second, ExtraFields: extra,
	})
	return chatModel, extra, err
}

func memoryModelRuntime(store *providerconfig.Store) agentdemousecase.MemoryModelResolver {
	return func(ctx context.Context) (model.BaseChatModel, error) {
		snapshot, err := store.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		chatModel, _, err := configuredChatModel(ctx, snapshot.Get)
		return chatModel, err
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
