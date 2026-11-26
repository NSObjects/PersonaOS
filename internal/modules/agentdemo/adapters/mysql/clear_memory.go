package mysql

import (
	"context"

	"gorm.io/gorm"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// ClearMemory removes the scoped summary, old preference facts and pending
// references. The version barrier prevents an in-flight consolidation from
// publishing memory from before the clear. Archived conversations remain.
func (s *Store) ClearMemory(ctx context.Context, scope memory.Scope) error {
	if ctx == nil || scope.OwnerID <= 0 || scope.CharacterID <= 0 {
		return apperr.NewBadRequest("invalid memory clear scope")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, err := lockCoreState(tx, scope)
		if err != nil {
			return err
		}
		if err := tx.Model(&state).Updates(map[string]any{
			"stable": "", "dynamic": "", coreVersionColumn: state.Version + 1,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where(scopePredicate, scope.OwnerID, scope.CharacterID).
			Delete(&pendingExchangeModel{}).Error; err != nil {
			return err
		}
		return tx.Where(scopePredicate, scope.OwnerID, scope.CharacterID).
			Delete(&memoryFactModel{}).Error
	})
	if err != nil {
		return apperr.WrapDatabase(err, "clear scoped memory")
	}
	return nil
}
