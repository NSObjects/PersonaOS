package redisrag

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// TestArkRedisLive exercises the paid embedding endpoint and real Redis Search.
func TestArkRedisLive(t *testing.T) {
	if os.Getenv("PERSONAOS_TEST_ARK_LIVE") != "1" {
		t.Skip("requires explicit live test opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	embedder, err := NewArkEmbedder(ctx, os.Getenv("PERSONAOS_EMBEDDING_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	vectors, err := embedder.EmbedStrings(ctx, []string{"森林里的小动物"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 1 || len(vectors[0]) != Dimensions {
		t.Fatalf("unexpected embedding shape: %d vectors, expected one with %d dimensions", len(vectors), Dimensions)
	}
	t.Logf("Ark embedding: %d dimensions, %s", len(vectors[0]), time.Since(start))
	client := redis.NewClient(&redis.Options{
		Addr: os.Getenv("PERSONAOS_TEST_REDIS_ADDR"), Protocol: 2,
	})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	store, err := New(ctx, client, embedder)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	verifyArkRetrieval(t, ctx, store)
}

func verifyArkRetrieval(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	allowed, denied := uuid.NewString(), uuid.NewString()
	const fact = "EM-5831"
	const privateFact = "PRIVATE-002"
	text := "星光船的船长是一位叫阿木的探险家。船长的专属通行证编号为 " + fact +
		"。\n\n小兔子喜欢吃胡萝卜。\n\n森林深处有一条河。\n\n春天的花园开满鲜花。"
	start := time.Now()
	if err := store.IngestText(ctx, allowed, text); err != nil {
		t.Fatal(err)
	}
	t.Logf("Ark vector ingestion (4 chunks): %s", time.Since(start))
	if err := store.IngestText(ctx, denied, "星光船船长的通行证编号为 "+privateFact+"。"); err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	docs, err := store.Retrieve(ctx, []string{allowed}, "阿木的身份凭证号码是多少？")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) == 0 || !strings.Contains(docs[0].Content, fact) {
		t.Fatalf("semantic retrieval did not rank the target first: %+v", docs)
	}
	for _, doc := range docs {
		if strings.Contains(doc.Content, privateFact) {
			t.Fatal("unauthorized corpus leaked")
		}
	}
	t.Logf("Ark query embedding + Redis retrieval: %s; correct top-1 and corpus isolation", time.Since(start))
}
