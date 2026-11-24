package redisrag

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/embedding/ark"
	"github.com/cloudwego/eino/components/embedding"
)

// Model identifies the fixed embedding space used by the Redis index.
const Model = "doubao-embedding-vision-251215"

// NewArkEmbedder creates the official Ark text-to-multimodal embedding adapter.
func NewArkEmbedder(ctx context.Context, key string) (embedding.Embedder, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("PERSONAOS_EMBEDDING_API_KEY is required")
	}
	apiType := ark.APITypeMultiModal
	dimensions := Dimensions
	timeout := 10 * time.Second
	retries := 0
	return ark.NewEmbedder(ctx, &ark.EmbeddingConfig{
		APIKey: key, Model: Model, APIType: &apiType,
		Dimensions: &dimensions, Timeout: &timeout, RetryTimes: &retries,
	})
}
