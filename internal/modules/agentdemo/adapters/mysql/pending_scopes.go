package mysql

import (
	"context"
	"time"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

var _ memory.PendingScopeReader = (*Store)(nil)

type pendingScopeRow struct {
	OwnerID       int64
	CharacterID   int64
	LastPendingAt time.Time
}

// ListPendingScopes returns a bounded page of scopes and their latest pending
// archive timestamp. Previously acknowledged exchanges are not scanned.
func (s *Store) ListPendingScopes(ctx context.Context, after memory.Scope, limit int) ([]memory.PendingScope, error) {
	if ctx == nil || after.OwnerID < 0 || after.CharacterID < 0 || limit < 1 || limit > memory.MaxPendingBatchSize {
		return nil, apperr.NewBadRequest("invalid pending scope scan")
	}
	var rows []pendingScopeRow
	err := s.db.WithContext(ctx).Table("memory_pending_exchanges AS p").
		Select("p.owner_id, p.character_id, MAX(u.created_at) AS last_pending_at").
		Joins("JOIN conversation_messages AS u ON u.id = p.user_message_id AND u.owner_id = p.owner_id AND u.character_id = p.character_id").
		Where("(p.owner_id > ? OR (p.owner_id = ? AND p.character_id > ?))", after.OwnerID, after.OwnerID, after.CharacterID).
		Group("p.owner_id, p.character_id").
		Order("p.owner_id ASC, p.character_id ASC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, apperr.WrapDatabase(err, "list pending memory scopes")
	}
	scopes := make([]memory.PendingScope, 0, len(rows))
	for _, row := range rows {
		scopes = append(scopes, memory.PendingScope{
			Scope:         memory.Scope{OwnerID: row.OwnerID, CharacterID: row.CharacterID},
			LastPendingAt: row.LastPendingAt,
		})
	}
	return scopes, nil
}
