package mysql

import (
	"context"
	"time"

	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

type knowledgeSourceModel struct {
	Version   string `gorm:"type:varchar(64);primaryKey"`
	Filename  string `gorm:"type:varchar(255);not null"`
	Content   string `gorm:"type:mediumtext;not null"`
	CreatedAt time.Time
}

func (knowledgeSourceModel) TableName() string { return "knowledge_sources" }

// SaveKnowledgeSource preserves original documents for immutable index versions.
func (s *ResourceStore) SaveKnowledgeSource(ctx context.Context, version, filename, text string) error {
	row := knowledgeSourceModel{Version: version, Filename: filename, Content: text}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return apperr.WrapDatabase(err, "archive knowledge document")
	}
	return nil
}
