package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/adapters/redisrag"
	"github.com/NSObjects/PersonaOS/internal/platform/configs"
	redisinfra "github.com/NSObjects/PersonaOS/internal/platform/infrastructure/redis"
)

var knowledgeIndexCmd = &cobra.Command{
	Use:   "knowledge-index VERSION FILE",
	Short: "Index an immutable UTF-8 knowledge corpus into Redis Search",
	Args:  cobra.ExactArgs(2),
	RunE:  runKnowledgeIndex,
}

func runKnowledgeIndex(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cfg, err := configs.Load(cfgFile)
	if err != nil {
		return err
	}
	resource, err := redisinfra.Open(ctx, cfg.Redis)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resource.Close(); closeErr != nil {
			cmd.PrintErrln(closeErr)
		}
	}()
	embedder, err := redisrag.NewArkEmbedder(ctx, os.Getenv("PERSONAOS_EMBEDDING_API_KEY"))
	if err != nil {
		return err
	}
	store, err := redisrag.New(ctx, resource.Client(), embedder)
	if err != nil {
		return err
	}
	file, err := os.Open(args[1])
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			cmd.PrintErrln(closeErr)
		}
	}()
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return err
	}
	if err := store.EnsureIndex(ctx); err != nil {
		return err
	}
	start := time.Now()
	if err := store.IngestText(ctx, args[0], string(data)); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "indexed %s in %s\n", args[0], time.Since(start))
	return err
}
