package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

const (
	knowledgeUnconfigured = "unconfigured"
	knowledgeUnavailable  = "unavailable"
	knowledgeNotIndexed   = "not_indexed"
	knowledgeIndexed      = "indexed"
)

// KnowledgeInspector reads index metadata and retrieves scoped fragments.
type KnowledgeInspector interface {
	VersionChunkCount(context.Context, string) (int64, error)
	PreviewVersion(context.Context, string, string) ([]domain.KnowledgeHit, error)
}

// WithKnowledgeInspector enables read-only diagnostics for registered resources.
func (u *ResourceUsecase) WithKnowledgeInspector(inspector KnowledgeInspector) *ResourceUsecase {
	u.inspector = inspector
	return u
}

// KnowledgeStatus reports actual index contents for a resource's current version.
type KnowledgeStatus struct {
	Version string `json:"version"`
	State   string `json:"state"`
	Chunks  int64  `json:"chunks"`
}

// KnowledgePreview contains retrieved fragments without invoking an LLM.
type KnowledgePreview struct {
	Version string                `json:"version"`
	Hits    []domain.KnowledgeHit `json:"hits"`
}

func (u *ResourceUsecase) knowledgeResource(ctx context.Context, id int64) (domain.Resource, error) {
	resource, err := u.Find(ctx, id)
	if err != nil {
		return domain.Resource{}, err
	}
	if resource.Kind != domain.ResourceKnowledgeBase || resource.Provider != knowledgeRedisProvider {
		return domain.Resource{}, apperr.NewBadRequest("unsupported knowledge resource")
	}
	return resource, nil
}

// InspectKnowledge does not invoke the embedding service or mutate data.
func (u *ResourceUsecase) InspectKnowledge(ctx context.Context, id int64) (KnowledgeStatus, error) {
	resource, err := u.knowledgeResource(ctx, id)
	if err != nil {
		return KnowledgeStatus{}, err
	}
	status := KnowledgeStatus{Version: resource.Reference, State: knowledgeUnconfigured}
	if u.inspector == nil {
		return status, nil
	}
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	count, err := u.inspector.VersionChunkCount(readCtx, resource.Reference)
	switch {
	case err != nil:
		status.State = knowledgeUnavailable
	case count == 0:
		status.State = knowledgeNotIndexed
	default:
		status.State = knowledgeIndexed
		status.Chunks = count
	}
	return status, nil
}

// SearchKnowledge accepts a resource ID, never a client-provided Redis version.
// Testing a draft resource does not grant that resource to any Agent.
func (u *ResourceUsecase) SearchKnowledge(ctx context.Context, id int64, query string) (KnowledgePreview, error) {
	resource, err := u.knowledgeResource(ctx, id)
	if err != nil {
		return KnowledgePreview{}, err
	}
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 2000 {
		return KnowledgePreview{}, apperr.NewBadRequest("请输入不超过 2000 字节的检索问题")
	}
	if u.inspector == nil {
		return KnowledgePreview{}, apperr.NewBadRequest("知识库向量服务未配置")
	}
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	hits, err := u.inspector.PreviewVersion(runCtx, resource.Reference, query)
	if err != nil {
		return KnowledgePreview{}, apperr.New(apperr.ErrInternalServer, "knowledge retrieval test failed")
	}
	return KnowledgePreview{Version: resource.Reference, Hits: hits}, nil
}
