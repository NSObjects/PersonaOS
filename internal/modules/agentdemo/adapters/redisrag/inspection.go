package redisrag

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/redis/go-redis/v9"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

// VersionChunkCount checks both the manifest and the actual indexed documents.
// It never creates an index or embeds a query.
func (s *Store) VersionChunkCount(ctx context.Context, version string) (int64, error) {
	if !versionPattern.MatchString(version) {
		return 0, fmt.Errorf("invalid knowledge version")
	}
	generation, err := s.client.Get(ctx, manifestPrefix+version).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !generationPattern.MatchString(generation) {
		return 0, fmt.Errorf("invalid knowledge manifest")
	}
	names, err := s.client.Do(ctx, "FT._LIST").StringSlice()
	if err != nil {
		return 0, err
	}
	if !slices.Contains(names, indexName) {
		return 0, nil
	}
	result, err := s.client.Do(ctx, "FT.SEARCH", indexName, "@"+corpusField+":{"+generation+"}",
		"LIMIT", 0, 0, "DIALECT", 2).Slice()
	if err != nil {
		return 0, err
	}
	if len(result) == 0 {
		return 0, fmt.Errorf("invalid search count response")
	}
	count, ok := result[0].(int64)
	if !ok || count < 0 {
		return 0, fmt.Errorf("invalid search count")
	}
	return count, nil
}

// PreviewVersion reuses the same scoped Eino retriever as the Agent.
func (s *Store) PreviewVersion(ctx context.Context, version, query string) ([]domain.KnowledgeHit, error) {
	docs, err := s.Retrieve(ctx, []string{version}, query)
	if err != nil {
		return nil, err
	}
	hits := make([]domain.KnowledgeHit, 0, len(docs))
	for _, doc := range docs {
		hits = append(hits, domain.KnowledgeHit{Source: doc.ID, Content: doc.Content})
	}
	return hits, nil
}
