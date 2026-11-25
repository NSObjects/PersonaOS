package providerconfig

import (
	"context"
	"errors"
)

// Snapshot is a server-only view of the effective configuration keyed by
// the existing deployment variable names. Each request owns its own copy.
type Snapshot struct {
	values map[string]string
}

// Get returns an allowlisted effective value. Unknown keys are empty.
func (s Snapshot) Get(key string) string { return s.values[key] }

// Snapshot reads all managed groups in one SQL statement so a runtime never
// mixes fields from different versions of a saved service.
func (s *Store) Snapshot(ctx context.Context) (Snapshot, error) {
	var rows []storedService
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return Snapshot{}, errors.New("cannot load provider runtime configuration")
	}
	stored := make(map[string][]byte, len(rows))
	for _, row := range rows {
		stored[row.Service] = row.Ciphertext
	}
	snapshot := Snapshot{values: make(map[string]string)}
	for _, service := range Services() {
		values := service.Environment(s.getenv)
		if data, exists := stored[service.ID]; exists {
			var err error
			values, err = s.cipher.Open(service.ID, data)
			if err != nil {
				return Snapshot{}, err
			}
		}
		for _, field := range service.Fields {
			snapshot.values[field.Env] = values[field.Name]
		}
	}
	return snapshot, nil
}
