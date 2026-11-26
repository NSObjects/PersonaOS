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
	UserChars     int64
}

// ListReadyScopes filters pending scopes before loading conversation bodies.
// Only new user text contributes to the trigger; assistant verbosity cannot trigger it.
func (s *Store) ListReadyScopes(ctx context.Context, after memory.Scope, now time.Time, limit int) ([]memory.PendingScope, error) {
	if ctx == nil || after.OwnerID < 0 || after.CharacterID < 0 || now.IsZero() || limit < 1 || limit > memory.MaxPendingBatchSize {
		return nil, apperr.NewBadRequest("invalid pending scope scan")
	}
	var rows []pendingScopeRow
	err := s.db.WithContext(ctx).Table("memory_pending_exchanges AS p").
		Select("p.owner_id, p.character_id, MAX(u.created_at) AS last_pending_at, SUM(CHAR_LENGTH(u.content)) AS user_chars").
		Joins("JOIN conversation_messages AS u ON u.id = p.user_message_id AND u.owner_id = p.owner_id AND u.character_id = p.character_id").
		Where("(p.owner_id > ? OR (p.owner_id = ? AND p.character_id > ?))", after.OwnerID, after.OwnerID, after.CharacterID).
		Group("p.owner_id, p.character_id, c.version, c.retry_count").
		Having("SUM(CHAR_LENGTH(u.content)) >= ? OR (SUM(CHAR_LENGTH(u.content)) >= ? AND MAX(u.created_at) <= ?)",
			memory.ConsolidationThreshold, memory.ConsolidationIdleMinimum, now.Add(-memory.ConsolidationIdleAge).UTC()).
		Order("p.owner_id ASC, p.character_id ASC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, apperr.WrapDatabase(err, "list pending memory scopes")
	}
	scopes := make([]memory.PendingScope, 0, len(rows))
	for _, row := range rows {
		scopes = append(scopes, memory.PendingScope{
			Scope:         memory.Scope{OwnerID: row.OwnerID, CharacterID: row.CharacterID},
			LastPendingAt: row.LastPendingAt,
			UserChars:     row.UserChars,
		})
	}
	return scopes, nil
}

