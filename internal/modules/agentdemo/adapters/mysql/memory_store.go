package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// Store persists the Demo's opt-in conversation turns and preference facts.
type Store struct{ db *gorm.DB }

// NewStore creates only the two minimal memory tables.
func NewStore(ctx context.Context, db *gorm.DB) (*Store, error) {
	if ctx == nil || db == nil {
		return nil, errors.New("create memory store: context and database required")
	}
	if err := db.WithContext(ctx).AutoMigrate(&conversationMessageModel{}, &memoryFactModel{}); err != nil {
		return nil, apperr.WrapDatabase(err, "migrate demo memory")
	}
	return &Store{db: db}, nil
}

type conversationMessageModel struct {
	ID          int64     `gorm:"primaryKey"`
	OwnerID     int64     `gorm:"not null;index:idx_conversation_scope,priority:1"`
	CharacterID int64     `gorm:"not null;index:idx_conversation_scope,priority:2"`
	SessionID   string    `gorm:"type:char(36);not null;index:idx_conversation_scope,priority:3"`
	Role        string    `gorm:"type:varchar(16);not null"`
	Content     string    `gorm:"type:text;not null"`
	CreatedAt   time.Time `gorm:"not null"`
}

func (conversationMessageModel) TableName() string { return "conversation_messages" }

type memoryFactModel struct {
	ID          int64     `gorm:"primaryKey"`
	OwnerID     int64     `gorm:"not null;uniqueIndex:idx_memory_scope_key,priority:1"`
	CharacterID int64     `gorm:"not null;uniqueIndex:idx_memory_scope_key,priority:2"`
	Key         string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_memory_scope_key,priority:3"`
	Value       string    `gorm:"type:varchar(256);not null"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null;index:idx_memory_updated"`
}

func (memoryFactModel) TableName() string { return "memory_facts" }

// ListFacts reads only one authenticated operator's memories for one character.
func (s *Store) ListFacts(ctx context.Context, scope memory.Scope, limit int) ([]memory.Fact, error) {
	if ctx == nil || scope.OwnerID <= 0 || scope.CharacterID <= 0 || limit < 1 || limit > 16 {
		return nil, apperr.NewBadRequest("invalid memory lookup scope")
	}
	var rows []memoryFactModel
	err := s.db.WithContext(ctx).
		Where("owner_id = ? AND character_id = ?", scope.OwnerID, scope.CharacterID).
		Order("updated_at DESC, id DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, apperr.WrapDatabase(err, "list demo memory facts")
	}
	facts := make([]memory.Fact, 0, len(rows))
	for _, row := range rows {
		facts = append(facts, memory.Fact{
			Key: row.Key, Value: row.Value, UpdatedAt: row.UpdatedAt,
		})
	}
	return facts, nil
}

// SaveExchange writes the completed dialogue turn after the reply has been
// produced. The caller is responsible for queueing it off the reply path.
func (s *Store) SaveExchange(ctx context.Context, turn memory.Exchange) error {
	if ctx == nil || turn.Scope.OwnerID <= 0 || turn.Scope.CharacterID <= 0 ||
		turn.SessionID == "" || turn.User == "" || turn.Assistant == "" {
		return apperr.NewBadRequest("invalid memory exchange")
	}
	now := time.Now().UTC()
	rows := []conversationMessageModel{
		{
			OwnerID: turn.Scope.OwnerID, CharacterID: turn.Scope.CharacterID,
			SessionID: turn.SessionID, Role: "user", Content: turn.User, CreatedAt: now,
		},
		{
			OwnerID: turn.Scope.OwnerID, CharacterID: turn.Scope.CharacterID,
			SessionID: turn.SessionID, Role: "assistant", Content: turn.Assistant, CreatedAt: now,
		},
	}
	if err := s.db.WithContext(ctx).Create(&rows).Error; err != nil {
		return apperr.WrapDatabase(err, "save demo conversation")
	}
	return nil
}

// UpsertFacts replaces only explicitly extracted non-sensitive preference
// keys within the same owner+character scope.
func (s *Store) UpsertFacts(ctx context.Context, scope memory.Scope, facts []memory.Fact) error {
	if ctx == nil || scope.OwnerID <= 0 || scope.CharacterID <= 0 {
		return apperr.NewBadRequest("invalid memory fact scope")
	}
	if len(facts) == 0 {
		return nil
	}
	now := time.Now().UTC()
	rows := make([]memoryFactModel, 0, len(facts))
	for _, fact := range facts {
		if fact.Key == "" || fact.Value == "" || len(fact.Key) > 64 || len(fact.Value) > 256 {
			return apperr.NewBadRequest("invalid memory fact")
		}
		rows = append(rows, memoryFactModel{
			OwnerID: scope.OwnerID, CharacterID: scope.CharacterID,
			Key: fact.Key, Value: fact.Value, CreatedAt: now, UpdatedAt: now,
		})
	}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "owner_id"}, {Name: "character_id"}, {Name: "key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&rows).Error
	if err != nil {
		return apperr.WrapDatabase(err, "upsert demo memory facts")
	}
	return nil
}
