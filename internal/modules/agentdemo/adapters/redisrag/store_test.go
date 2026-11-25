package redisrag

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type fixtureEmbedder struct{}

func (fixtureEmbedder) EmbedStrings(_ context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	vectors := make([][]float64, len(texts))
	for i, text := range texts {
		vectors[i] = make([]float64, Dimensions)
		if strings.Contains(text, "徽章") {
			vectors[i][0] = 1
		} else {
			vectors[i][1] = 1
		}
	}
	return vectors, nil
}

func TestSplitText(t *testing.T) {
	docs, err := splitText("generation", strings.Repeat("字", 601)+"\n\n第二段")
	if err != nil || len(docs) != 3 {
		t.Fatalf("docs=%v err=%v, want 3 chunks", docs, err)
	}
	if len([]rune(docs[0].Content)) != chunkRunes || docs[1].Content != "字" || docs[2].Content != "第二段" {
		t.Fatalf("unexpected split: %+v", docs)
	}
}

func TestRedisVectorKnowledge(t *testing.T) {
	addr := os.Getenv("PERSONAOS_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("requires Redis 8 Search service")
	}
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: addr, Protocol: 2})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	store, err := New(ctx, client, fixtureEmbedder{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	allowed, denied := uuid.NewString(), uuid.NewString()
	if err := store.IngestText(ctx, allowed, "月光徽章编号为 LX-7429。"); err != nil {
		t.Fatal(err)
	}
	if err := store.IngestText(ctx, denied, "月光徽章编号为 SECRET-002。"); err != nil {
		t.Fatal(err)
	}
	if count, err := store.VersionChunkCount(ctx, allowed); err != nil || count != 1 {
		t.Fatalf("indexed count=%d err=%v", count, err)
	}
	missing := uuid.NewString()
	if count, err := store.VersionChunkCount(ctx, missing); err != nil || count != 0 {
		t.Fatalf("missing count=%d err=%v", count, err)
	}
	// A manifest without documents must never be advertised as indexed.
	generation := strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := client.Set(ctx, manifestPrefix+missing, generation, 0).Err(); err != nil {
		t.Fatal(err)
	}
	if count, err := store.VersionChunkCount(ctx, missing); err != nil || count != 0 {
		t.Fatalf("empty manifest count=%d err=%v", count, err)
	}
	hits, err := store.PreviewVersion(ctx, allowed, "徽章")
	if err != nil || len(hits) != 1 || !strings.Contains(hits[0].Content, "LX-7429") {
		t.Fatalf("preview=%v err=%v", hits, err)
	}
	docs, err := store.Retrieve(ctx, []string{allowed}, "徽章编号是什么")
	if err != nil || len(docs) != 1 || !strings.Contains(docs[0].Content, "LX-7429") {
		t.Fatalf("authorized docs=%v err=%v", docs, err)
	}
	if _, err := store.Retrieve(ctx, []string{"unknown"}, "徽章"); err == nil {
		t.Fatal("unindexed version accepted")
	}
	if err := store.IngestText(ctx, allowed, "replacement"); err == nil {
		t.Fatal("immutable version overwritten")
	}
}
