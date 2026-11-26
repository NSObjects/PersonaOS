package mysql

import (
	"context"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

var _ memory.CoreReader = (*Store)(nil)

// ReadCore loads only the last published stable and dynamic summaries.
func (s *Store) ReadCore(ctx context.Context, scope memory.Scope) (memory.CoreMemory, error) {
	if ctx == nil || scope.OwnerID <= 0 || scope.CharacterID <= 0 {
		return memory.CoreMemory{}, apperr.NewBadRequest("invalid core memory lookup")
	}
	var core memory.CoreMemory
	err := s.db.WithContext(ctx).Model(&coreMemoryModel{}).
		Select("stable", "dynamic").Where(scopePredicate, scope.OwnerID, scope.CharacterID).
		Find(&core).Error
	if err != nil {
		return memory.CoreMemory{}, apperr.WrapDatabase(err, "read published core memory")
	}
	return core, nil
}
