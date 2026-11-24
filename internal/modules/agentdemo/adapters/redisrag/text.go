package redisrag

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
)

const (
	maxTextBytes = 1 << 20
	chunkRunes = 600
)

func splitText(generation, text string) ([]*schema.Document, error) {
	if len(text) > maxTextBytes || !utf8.ValidString(text) || strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("knowledge text must be nonempty UTF-8, at most 1 MiB")
	}
	var docs []*schema.Document
	for _, paragraph := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		runes := []rune(strings.TrimSpace(paragraph))
		for start := 0; start < len(runes); start += chunkRunes {
			part := string(runes[start:min(start+chunkRunes, len(runes))])
			docs = append(docs, &schema.Document{
				ID: fmt.Sprintf("%s:%d", generation, len(docs)),
				Content: part,
				MetaData: map[string]any{corpusField: generation},
			})
		}
	}
	return docs, nil
}
