package mysql

import (
	"context"
	"slices"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

type coreMemoryModel struct {
	OwnerID     int64  `gorm:"primaryKey;autoIncrement:false"`
	CharacterID int64  `gorm:"primaryKey;autoIncrement:false"`
	Stable      string `gorm:"type:varchar(600);not null"`
	Dynamic     string `gorm:"type:varchar(600);not null"`
	Version     uint64 `gorm:"not null"`
	ThroughID   int64  `gorm:"not null"`
}

func (coreMemoryModel) TableName() string { return "memory_core_states" }

type pendingExchangeModel struct {
	UserMessageID      int64 `gorm:"primaryKey;autoIncrement:false;index:idx_pending_scope,priority:3"`
	AssistantMessageID int64 `gorm:"not null"`
	OwnerID            int64 `gorm:"not null;index:idx_pending_scope,priority:1"`
	CharacterID        int64 `gorm:"not null;index:idx_pending_scope,priority:2"`
}

func (pendingExchangeModel) TableName() string { return "memory_pending_exchanges" }

const scopePredicate = "owner_id = ? AND character_id = ?"

var _ memory.ConsolidationStore = (*Store)(nil)

func lockCoreState(tx *gorm.DB, scope memory.Scope) (coreMemoryModel, error) {
	state := coreMemoryModel{OwnerID: scope.OwnerID, CharacterID: scope.CharacterID}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return state, err
	}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(scopePredicate, scope.OwnerID, scope.CharacterID).Take(&state).Error
	return state, err
}

// AppendPending archives a completed turn and queues its message IDs atomically.
// The scope lock orders admission against consolidation commits.
func (s *Store) AppendPending(ctx context.Context, turn memory.Exchange) error {
	if ctx == nil || !validExchange(turn) {
		return apperr.NewBadRequest("invalid memory exchange")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockCoreState(tx, turn.Scope); err != nil {
			return err
		}
		rows := exchangeRows(turn)
		if err := tx.Create(&rows).Error; err != nil {
			return err
		}
		return tx.Create(&pendingExchangeModel{
			UserMessageID: rows[0].ID, AssistantMessageID: rows[1].ID,
			OwnerID: turn.Scope.OwnerID, CharacterID: turn.Scope.CharacterID,
		}).Error
	})
	if err != nil {
		return apperr.WrapDatabase(err, "append pending memory exchange")
	}
	return nil
}

type pendingExchangeRow struct {
	ID        int64
	SessionID string
	User      string
	Assistant string
}

// ReadPending loads a bounded ordered batch without modifying the queue.
// The snapshot can become stale; CommitPending validates it before writing.
func (s *Store) ReadPending(ctx context.Context, scope memory.Scope, limit int) (memory.PendingBatch, error) {
	if ctx == nil || scope.OwnerID <= 0 || scope.CharacterID <= 0 || limit < 1 || limit > memory.MaxPendingBatchSize {
		return memory.PendingBatch{}, apperr.NewBadRequest("invalid pending memory lookup")
	}
	var state coreMemoryModel
	err := s.db.WithContext(ctx).Where(scopePredicate, scope.OwnerID, scope.CharacterID).Find(&state).Error
	if err != nil {
		return memory.PendingBatch{}, apperr.WrapDatabase(err, "read core memory")
	}
	var rows []pendingExchangeRow
	err = s.db.WithContext(ctx).Table("memory_pending_exchanges AS p").
		Select("p.user_message_id AS id, u.session_id, u.content AS user, a.content AS assistant").
		Joins("JOIN conversation_messages AS u ON u.id = p.user_message_id AND u.owner_id = p.owner_id AND u.character_id = p.character_id").
		Joins("JOIN conversation_messages AS a ON a.id = p.assistant_message_id AND a.owner_id = p.owner_id AND a.character_id = p.character_id").
		Where("p.owner_id = ? AND p.character_id = ?", scope.OwnerID, scope.CharacterID).
		Order("p.user_message_id ASC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return memory.PendingBatch{}, apperr.WrapDatabase(err, "read pending memory")
	}
	batch := memory.PendingBatch{
		Core: memory.CoreState{
			CoreMemory: memory.CoreMemory{Stable: state.Stable, Dynamic: state.Dynamic},
			Version: state.Version, ThroughID: state.ThroughID,
		},
		Exchanges: make([]memory.PendingExchange, 0, len(rows)),
	}
	for _, row := range rows {
		batch.Exchanges = append(batch.Exchanges, memory.PendingExchange{
			ID: row.ID, Exchange: memory.Exchange{Scope: scope, SessionID: row.SessionID, User: row.User, Assistant: row.Assistant},
		})
	}
	return batch, nil
}

// CommitPending atomically publishes a revision and removes precisely the
// acknowledged queue prefix. A noop consolidation passes the unchanged memory.
func (s *Store) CommitPending(ctx context.Context, scope memory.Scope, version uint64, ids []int64, next memory.CoreMemory) error {
	if ctx == nil || scope.OwnerID <= 0 || scope.CharacterID <= 0 || len(ids) == 0 || len(ids) > memory.MaxPendingBatchSize ||
		!utf8.ValidString(next.Stable) || !utf8.ValidString(next.Dynamic) ||
		utf8.RuneCountInString(next.Stable) > memory.MaxCoreMemoryLength || utf8.RuneCountInString(next.Dynamic) > memory.MaxCoreMemoryLength {
		return apperr.NewBadRequest("invalid memory consolidation")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, err := lockCoreState(tx, scope)
		if err != nil {
			return apperr.WrapDatabase(err, "lock core memory")
		}
		if state.Version != version {
			return apperr.NewConflict("stale core memory revision")
		}
		return commitMemoryPrefix(tx, scope, state, ids, next)
	})
}

func commitMemoryPrefix(tx *gorm.DB, scope memory.Scope, state coreMemoryModel, ids []int64, next memory.CoreMemory) error {
	var pending []int64
	err := tx.Model(&pendingExchangeModel{}).Where(scopePredicate, scope.OwnerID, scope.CharacterID).
		Order("user_message_id ASC").Limit(len(ids)).Pluck("user_message_id", &pending).Error
	if err != nil {
		return apperr.WrapDatabase(err, "verify pending memory prefix")
	}
	if !slices.Equal(pending, ids) {
		return apperr.NewConflict("stale pending memory batch")
	}
	err = tx.Model(&state).Updates(map[string]any{
		"stable": next.Stable, "dynamic": next.Dynamic, "version": state.Version + 1, "through_id": ids[len(ids)-1],
	}).Error
	if err != nil {
		return apperr.WrapDatabase(err, "commit core memory")
	}
	err = tx.Where(scopePredicate, scope.OwnerID, scope.CharacterID).
		Where("user_message_id IN ?", ids).Delete(&pendingExchangeModel{}).Error
	if err != nil {
		return apperr.WrapDatabase(err, "acknowledge pending memory")
	}
	return nil
}
