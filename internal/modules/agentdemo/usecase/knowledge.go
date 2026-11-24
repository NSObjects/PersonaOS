package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const (
	knowledgeToolName   = "search_knowledge"
	knowledgeMaxBytes   = 64 << 10
	knowledgeMaxDocs    = 16
	knowledgeTopK       = 3
	knowledgeChunkRunes = 600
)

var knowledgeKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type localKnowledge struct {
	dir  string
	keys []string
}

// newKnowledgeTool fixes the search scope at the published snapshot. Model
// arguments can select a query, never a file, directory or knowledge base ID.
func newKnowledgeTool(dir string, resources []domain.Resource) (tool.BaseTool, error) {
	var keys []string
	seen := make(map[string]bool)
	for _, resource := range resources {
		if !resource.Enabled || resource.Kind != domain.ResourceKnowledgeBase {
			continue
		}
		if resource.Provider != "local" || !knowledgeKeyPattern.MatchString(resource.Reference) {
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
	if strings.TrimSpace(dir) == "" || len(keys) > knowledgeMaxDocs {
		return nil, fmt.Errorf("invalid local knowledge configuration")
	}
	return knowledgeSearchTool{retriever: localKnowledge{dir: dir, keys: keys}}, nil
}

type knowledgeSearchTool struct {
	retriever retriever.Retriever
}

func (knowledgeSearchTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: knowledgeToolName,
		Desc: "检索当前角色和设备已授权的专属知识资料。涉及专属设定、故事世界、产品资料等事实时使用。输入简短关键词。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {Type: schema.String, Desc: "待检索的问题或关键词", Required: true},
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
	docs, err := t.retriever.Retrieve(ctx, input.Query, retriever.WithTopK(knowledgeTopK))
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

type rankedKnowledge struct {
	doc   *schema.Document
	score int
}

func (r localKnowledge) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(r.dir)
	if err != nil {
		return nil, fmt.Errorf("open knowledge directory: %w", err)
	}
	defer func() {
		if closeErr := root.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("close knowledge directory")
		}
	}()
	terms := knowledgeTerms(query)
	var ranked []rankedKnowledge
	for _, key := range r.keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		text, err := readKnowledgeFile(root, key)
		if err != nil {
			return nil, err
		}
		ranked = append(ranked, rankKnowledge(key, text, terms)...)
	}
	slices.SortStableFunc(ranked, func(a, b rankedKnowledge) int { return b.score - a.score })
	options := retriever.GetCommonOptions(nil, opts...)
	top := knowledgeTopK
	if options.TopK != nil && *options.TopK > 0 {
		top = min(top, *options.TopK)
	}
	docs := make([]*schema.Document, 0, min(top, len(ranked)))
	for _, hit := range ranked[:min(top, len(ranked))] {
		docs = append(docs, hit.doc)
	}
	return docs, nil
}

func readKnowledgeFile(root *os.Root, key string) (string, error) {
	file, err := root.Open(key + ".txt")
	if err != nil {
		return "", fmt.Errorf("open knowledge corpus %q: %w", key, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("close knowledge corpus")
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat knowledge corpus: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > knowledgeMaxBytes {
		return "", fmt.Errorf("invalid knowledge corpus size or type")
	}
	data, err := io.ReadAll(io.LimitReader(file, knowledgeMaxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read knowledge corpus: %w", err)
	}
	if len(data) > knowledgeMaxBytes || !utf8.Valid(data) || strings.TrimSpace(string(data)) == "" {
		return "", fmt.Errorf("invalid UTF-8 knowledge corpus")
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n"), nil
}

// Match Unicode bigrams for Chinese and whole words for Latin text. This is a
// bounded lexical retriever, not a semantic vector search.
func knowledgeTerms(query string) map[string]bool {
	terms := make(map[string]bool)
	var word strings.Builder
	var previous rune
	for _, ch := range strings.ToLower(query) {
		if unicode.Is(unicode.Han, ch) {
			if word.Len() > 0 {
				terms[word.String()] = true
				word.Reset()
			}
			if previous != 0 {
				terms[string([]rune{previous, ch})] = true
			}
			previous = ch
			continue
		}
		previous = 0
		if unicode.IsLetter(ch) || unicode.IsNumber(ch) {
			word.WriteRune(ch)
		} else if word.Len() > 0 {
			terms[word.String()] = true
			word.Reset()
		}
	}
	if word.Len() > 0 {
		terms[word.String()] = true
	}
	return terms
}

func rankKnowledge(key, text string, terms map[string]bool) []rankedKnowledge {
	var ranked []rankedKnowledge
	for i, paragraph := range strings.Split(text, "\n\n") {
		runes := []rune(strings.TrimSpace(paragraph))
		for offset := 0; offset < len(runes); offset += knowledgeChunkRunes {
			part := string(runes[offset:min(offset+knowledgeChunkRunes, len(runes))])
			score := 0
			lower := strings.ToLower(part)
			for term := range terms {
				if strings.Contains(lower, term) {
					score++
				}
			}
			if score > 0 {
				ranked = append(ranked, rankedKnowledge{
					doc:   &schema.Document{ID: fmt.Sprintf("%s:%d:%d", key, i+1, offset), Content: part},
					score: score,
				})
			}
		}
	}
	return ranked
}
