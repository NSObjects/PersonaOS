package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const (
	knowledgeToolName = "search_knowledge"
	knowledgeProvider = "redis"
	knowledgeMaxDocs  = 16
)

var knowledgeKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// KnowledgeRetriever retrieves only the explicitly authorized corpus versions.
type KnowledgeRetriever interface {
	Retrieve(ctx context.Context, versions []string, query string) ([]*schema.Document, error)
}

// newKnowledgeTool fixes the scope at the published snapshot. The model can
// supply a query, but cannot choose a corpus, Redis key or index.
func newKnowledgeTool(search KnowledgeRetriever, resources []domain.Resource) (tool.BaseTool, error) {
	keys := make([]string, 0, len(resources))
	seen := make(map[string]bool)
	for _, resource := range resources {
		if !resource.Enabled || resource.Kind != domain.ResourceKnowledgeBase {
			continue
		}
		if resource.Provider != knowledgeProvider || !knowledgeKeyPattern.MatchString(resource.Reference) {
			return nil, fmt.Errorf("unsupported knowledge base reference")
		}
		if !seen[resource.Reference] {
			keys = append(keys, resource.Reference)
			seen[resource.Reference] = true
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	if search == nil || len(keys) > knowledgeMaxDocs {
		return nil, fmt.Errorf("invalid Redis knowledge configuration")
	}
	return knowledgeSearchTool{retriever: search, versions: keys}, nil
}

type knowledgeSearchTool struct {
	retriever KnowledgeRetriever
	versions  []string
}

func (knowledgeSearchTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: knowledgeToolName,
		Desc: "检索当前角色和设备已授权的专属知识资料。涉及专属设定、故事世界、产品资料等事实时使用。输入简短自然语言问题。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {Type: schema.String, Desc: "待检索的问题", Required: true},
		}),
	}, nil
}

type knowledgeHit struct {
	Source  string `json:"source"`
	Content string `json:"content"`
}

func (t knowledgeSearchTool) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", fmt.Errorf("decode knowledge query: %w", err)
	}
	if strings.TrimSpace(input.Query) == "" || len(input.Query) > maxDemoMessageLength {
		return "", fmt.Errorf("invalid knowledge query")
	}
	docs, err := t.retriever.Retrieve(ctx, t.versions, input.Query)
	if err != nil {
		return "", err
	}
	hits := make([]knowledgeHit, 0, len(docs))
	for _, doc := range docs {
		hits = append(hits, knowledgeHit{Source: doc.ID, Content: doc.Content})
	}
	data, err := json.Marshal(hits)
	if err != nil {
		return "", err
	}
	log.Info().Str("tool", knowledgeToolName).Int("hits", len(hits)).Msg("demo agent knowledge retrieved")
	return string(data), nil
}
