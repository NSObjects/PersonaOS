package usecase

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

const (
	maxKnowledgeUploadBytes = 64 << 10
	knowledgeRedisProvider  = "redis"
)

// KnowledgeSourceStore archives original text independently of the vector index.
type KnowledgeSourceStore interface {
	SaveKnowledgeSource(ctx context.Context, version, filename, text string) error
}

// KnowledgeIndexer indexes immutable versions using the configured vector backend.
type KnowledgeIndexer interface {
	EnsureIndex(context.Context) error
	IngestText(ctx context.Context, version, text string) error
}

// WithKnowledgeImport enables synchronous knowledge document ingestion.
func (u *ResourceUsecase) WithKnowledgeImport(sources KnowledgeSourceStore, indexer KnowledgeIndexer) *ResourceUsecase {
	u.sources, u.indexer = sources, indexer
	return u
}

type knowledgeSource struct {
	version  string
	filename string
	text     string
}

func (u *ResourceUsecase) prepareKnowledge(in ResourceInput, kind domain.ResourceKind) (ResourceInput, *knowledgeSource, error) {
	if in.KnowledgeFilename == "" && in.KnowledgeText == "" {
		return in, nil, nil
	}
	filename := strings.TrimSpace(in.KnowledgeFilename)
	ext := strings.ToLower(filepath.Ext(filename))
	if kind != domain.ResourceKnowledgeBase || len(filename) > 255 || strings.ContainsAny(filename, "/\\\r\n\x00") ||
		(ext != ".txt" && ext != ".md" && ext != ".markdown") || len(in.KnowledgeText) > maxKnowledgeUploadBytes ||
		!utf8.ValidString(in.KnowledgeText) || strings.TrimSpace(in.KnowledgeText) == "" {
		return in, nil, apperr.NewBadRequest("仅支持非空 UTF-8 TXT 或 Markdown 文档，大小不超过 64 KiB")
	}
	if u.sources == nil || u.indexer == nil {
		return in, nil, apperr.New(apperr.ErrInternalServer, "知识库向量服务未配置")
	}
	source := &knowledgeSource{version: "kb-" + uuid.NewString(), filename: filename, text: in.KnowledgeText}
	in.Reference = source.version
	in.Provider = knowledgeRedisProvider
	return in, source, nil
}

func (u *ResourceUsecase) indexKnowledge(ctx context.Context, source *knowledgeSource) error {
	if source == nil {
		return nil
	}
	// Archive first, so the original survives Redis loss or a failed build.
	// The resource reference is not changed until the index is complete.
	if err := u.sources.SaveKnowledgeSource(ctx, source.version, source.filename, source.text); err != nil {
		return fmt.Errorf("archive knowledge source: %w", err)
	}
	if err := u.indexer.EnsureIndex(ctx); err != nil {
		return fmt.Errorf("ensure knowledge index: %w", err)
	}
	if err := u.indexer.IngestText(ctx, source.version, source.text); err != nil {
		return fmt.Errorf("index knowledge source: %w", err)
	}
	return nil
}
