package mysql

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// MigrateLegacyMemory transfers old preferences before enabling the batch
// runtime. Each scope is atomic. A completed migration leaves no legacy facts
// to scan on the next startup. Archived messages are never replayed.
func (s *Store) MigrateLegacyMemory(ctx context.Context) error {
	if ctx == nil {
		return apperr.NewBadRequest("invalid memory migration context")
	}
	for {
		var scope memory.Scope
		result := s.db.WithContext(ctx).Model(&memoryFactModel{}).
			Select("owner_id", "character_id").Order("owner_id ASC, character_id ASC").
			Limit(1).Find(&scope)
		if result.Error != nil {
			return apperr.WrapDatabase(result.Error, "discover legacy memory")
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := s.migrateLegacyScope(ctx, scope); err != nil {
			return err
		}
	}
}

func (s *Store) migrateLegacyScope(ctx context.Context, scope memory.Scope) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, err := lockCoreState(tx, scope)
		if err != nil {
			return apperr.WrapDatabase(err, "lock legacy memory")
		}
		var facts []memoryFactModel
		if err := tx.Where(scopePredicate, scope.OwnerID, scope.CharacterID).
			Order("id ASC").Find(&facts).Error; err != nil {
			return apperr.WrapDatabase(err, "read legacy memory")
		}
		if len(facts) == 0 {
			return nil
		}
		next, err := mergeLegacyFacts(memory.CoreMemory{Stable: state.Stable, Dynamic: state.Dynamic}, facts)
		if err != nil {
			return err
		}
		if err := tx.Model(&state).Select("Stable", "Dynamic", "Version").Updates(coreMemoryModel{
			Stable: next.Stable, Dynamic: next.Dynamic, Version: state.Version + 1,
		}).Error; err != nil {
			return apperr.WrapDatabase(err, "publish migrated memory")
		}
		if err := tx.Where(scopePredicate, scope.OwnerID, scope.CharacterID).Delete(&memoryFactModel{}).Error; err != nil {
			return apperr.WrapDatabase(err, "remove migrated preferences")
		}
		return nil
	})
}

func mergeLegacyFacts(core memory.CoreMemory, facts []memoryFactModel) (memory.CoreMemory, error) {
	for _, fact := range facts {
		line := "旧偏好 " + fact.Key + ": " + fact.Value
		if next, ok := appendLegacyLine(core.Stable, line); ok {
			core.Stable = next
			continue
		}
		if next, ok := appendLegacyLine(core.Dynamic, line); ok {
			core.Dynamic = next
			continue
		}
		return memory.CoreMemory{}, errors.New("legacy memory exceeds core capacity; original facts retained")
	}
	return core, nil
}

func appendLegacyLine(section, line string) (string, bool) {
	next := strings.TrimSpace(section + "\n" + line)
	return next, utf8.ValidString(next) && utf8.RuneCountInString(next) <= memory.MaxCoreMemoryLength
}
