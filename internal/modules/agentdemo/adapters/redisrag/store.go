// Package redisrag implements versioned knowledge retrieval with Eino Redis components.
package redisrag

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	redisindexer "github.com/cloudwego/eino-ext/components/indexer/redis"
	redisretriever "github.com/cloudwego/eino-ext/components/retriever/redis"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	// Dimensions is the vector size shared by ingestion and retrieval.
	Dimensions = 1024
	indexName = "personaos:rag:vision251215:1024"
	chunkPrefix = indexName + ":chunk:"
	manifestPrefix = indexName + ":version:"
	contentField = "content"
	corpusField = "corpus"
	vectorField = "vector_content"
	maxCorpora = 16
	topK = 3
)

var (
	versionPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	generationPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

// Store composes the official Eino Indexer and Retriever over Redis Search.
type Store struct {
	client *redis.Client
	indexer *redisindexer.Indexer
	retriever *redisretriever.Retriever
}

// New configures the native components using a shared RESP2 Redis client.
func New(ctx context.Context, client *redis.Client, embedder embedding.Embedder) (*Store, error) {
	if client == nil || embedder == nil {
		return nil, fmt.Errorf("Redis and embedding are required for knowledge retrieval")
	}
	if client.Options().Protocol != 2 || !client.Options().UnstableResp3 {
		return nil, fmt.Errorf("Redis Search requires RESP2 and UnstableResp3")
	}
	idx, err := redisindexer.NewIndexer(ctx, &redisindexer.IndexerConfig{
		Client: client, KeyPrefix: chunkPrefix, Embedding: embedder,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize Eino Redis indexer: %w", err)
	}
	search, err := redisretriever.NewRetriever(ctx, &redisretriever.RetrieverConfig{
		Client: client, Index: indexName, VectorField: vectorField,
		TopK: topK, ReturnFields: []string{contentField, corpusField}, Embedding: embedder,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize Eino Redis retriever: %w", err)
	}
	return &Store{client: client, indexer: idx, retriever: search}, nil
}

// EnsureIndex creates the versioned Redis Search schema before ingestion.
func (s *Store) EnsureIndex(ctx context.Context) error {
	names, err := s.client.Do(ctx, "FT._LIST").StringSlice()
	if err != nil {
		return fmt.Errorf("list Redis Search indexes: %w", err)
	}
	for _, name := range names {
		if name == indexName {
			return nil
		}
	}
	if err := s.client.Do(ctx, "FT.CREATE", indexName,
		"ON", "HASH", "PREFIX", 1, chunkPrefix,
		"SCHEMA", corpusField, "TAG",
		contentField, "TEXT",
		vectorField, "VECTOR", "FLAT", 6,
		"TYPE", "FLOAT32", "DIM", Dimensions, "DISTANCE_METRIC", "COSINE").Err(); err != nil {
		return fmt.Errorf("create Redis Search index: %w", err)
	}
	return nil
}

// IngestText indexes an immutable corpus version. A manifest is published only
// after successful indexing; incomplete generations cannot be retrieved.
func (s *Store) IngestText(ctx context.Context, version, text string) error {
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("invalid knowledge version")
	}
	exists, err := s.client.Exists(ctx, manifestPrefix+version).Result()
	if err != nil {
		return err
	}
	if exists != 0 {
		return fmt.Errorf("knowledge version already exists; use a new version")
	}
	generation := strings.ReplaceAll(uuid.NewString(), "-", "")
	docs, err := splitText(generation, text)
	if err != nil {
		return err
	}
	if _, err := s.indexer.Store(ctx, docs); err != nil {
		return fmt.Errorf("index knowledge documents: %w", err)
	}
	published, err := s.client.SetNX(ctx, manifestPrefix+version, generation, 0).Result()
	if err != nil {
		return fmt.Errorf("publish knowledge version: %w", err)
	}
	if !published {
		return fmt.Errorf("knowledge version already exists; use a new version")
	}
	return nil
}

// Retrieve searches only the immutable versions authorized by the caller.
// Model-supplied text is embedded, never interpolated into the Redis filter.
func (s *Store) Retrieve(ctx context.Context, versions []string, query string) ([]*schema.Document, error) {
	if len(versions) == 0 {
		return nil, nil
	}
	if len(versions) > maxCorpora || strings.TrimSpace(query) == "" || len(query) > 2000 {
		return nil, fmt.Errorf("invalid knowledge retrieval request")
	}
	keys := make([]string, 0, len(versions))
	for _, version := range versions {
		if !versionPattern.MatchString(version) {
			return nil, fmt.Errorf("invalid knowledge version")
		}
		keys = append(keys, manifestPrefix+version)
	}
	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("resolve knowledge versions: %w", err)
	}
	generations := make([]string, 0, len(values))
	for _, value := range values {
		generation, ok := value.(string)
		if !ok || !generationPattern.MatchString(generation) {
			return nil, fmt.Errorf("knowledge version not indexed")
		}
		generations = append(generations, generation)
	}
	filter := "@" + corpusField + ":{" + strings.Join(generations, "|") + "}"
	return s.retriever.Retrieve(ctx, query, redisretriever.WithFilterQuery(filter))
}
