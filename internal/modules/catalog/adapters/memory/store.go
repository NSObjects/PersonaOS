// Package memory provides deterministic in-memory catalog persistence for tests.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// Store is a concurrency-safe test double for a catalog database.
type Store struct {
	mu            sync.RWMutex
	nextCharacter int64
	nextDevice    int64
	characters    map[int64]domain.Character
	devices       map[int64]domain.DeviceType
}

// New creates an empty in-memory catalog.
func New() *Store {
	return &Store{characters: map[int64]domain.Character{}, devices: map[int64]domain.DeviceType{}}
}

// ListCharacters returns all persisted characters.
func (s *Store) ListCharacters(ctx context.Context) ([]domain.Character, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Character, 0, len(s.characters))
	for _, v := range s.characters {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// FindCharacter looks up one character by ID.
func (s *Store) FindCharacter(ctx context.Context, id int64) (domain.Character, error) {
	if err := ctx.Err(); err != nil {
		return domain.Character{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.characters[id]
	if !ok {
		return domain.Character{}, apperr.NewNotFound("character")
	}
	return v, nil
}

// CreateCharacter saves a new character with a unique code.
func (s *Store) CreateCharacter(ctx context.Context, v domain.Character) (domain.Character, error) {
	if err := ctx.Err(); err != nil {
		return domain.Character{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.characters {
		if existing.Code == v.Code {
			return domain.Character{}, apperr.NewConflict("character code already exists")
		}
	}
	s.nextCharacter++
	v.ID = s.nextCharacter
	v.CreatedAt = time.Now().UTC()
	v.UpdatedAt = v.CreatedAt
	s.characters[v.ID] = v
	return v, nil
}

// UpdateCharacter updates editable character fields.
func (s *Store) UpdateCharacter(ctx context.Context, v domain.Character) (domain.Character, error) {
	if err := ctx.Err(); err != nil {
		return domain.Character{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.characters[v.ID]
	if !ok {
		return domain.Character{}, apperr.NewNotFound("character")
	}
	v.Code = old.Code
	v.CreatedAt = old.CreatedAt
	v.UpdatedAt = time.Now().UTC()
	s.characters[v.ID] = v
	return v, nil
}

// DeleteCharacter removes a character only when it is not referenced by any device type.
func (s *Store) DeleteCharacter(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.characters[id]; !ok {
		return apperr.NewNotFound("character")
	}
	for _, device := range s.devices {
		if device.CharacterID == id {
			return apperr.NewConflict("character is in use by device types")
		}
	}
	delete(s.characters, id)
	return nil
}

// ListDeviceTypes returns all configured device types.
func (s *Store) ListDeviceTypes(ctx context.Context) ([]domain.DeviceType, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.DeviceType, 0, len(s.devices))
	for _, v := range s.devices {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// FindDeviceType looks up one device type by ID.
func (s *Store) FindDeviceType(ctx context.Context, id int64) (domain.DeviceType, error) {
	if err := ctx.Err(); err != nil {
		return domain.DeviceType{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.devices[id]
	if !ok {
		return domain.DeviceType{}, apperr.NewNotFound("device type")
	}
	return v, nil
}

// CreateDeviceType saves a device type that references an existing character.
func (s *Store) CreateDeviceType(ctx context.Context, v domain.DeviceType) (domain.DeviceType, error) {
	if err := ctx.Err(); err != nil {
		return domain.DeviceType{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.characters[v.CharacterID]; !ok {
		return domain.DeviceType{}, apperr.NewNotFound("character")
	}
	for _, existing := range s.devices {
		if existing.Code == v.Code {
			return domain.DeviceType{}, apperr.NewConflict("device type code already exists")
		}
	}
	s.nextDevice++
	v.ID = s.nextDevice
	v.CreatedAt = time.Now().UTC()
	v.UpdatedAt = v.CreatedAt
	s.devices[v.ID] = v
	return v, nil
}

// UpdateDeviceType updates a device type while preserving its code.
func (s *Store) UpdateDeviceType(ctx context.Context, v domain.DeviceType) (domain.DeviceType, error) {
	if err := ctx.Err(); err != nil {
		return domain.DeviceType{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.devices[v.ID]
	if !ok {
		return domain.DeviceType{}, apperr.NewNotFound("device type")
	}
	if _, ok := s.characters[v.CharacterID]; !ok {
		return domain.DeviceType{}, apperr.NewNotFound("character")
	}
	v.Code = old.Code
	v.CreatedAt = old.CreatedAt
	v.UpdatedAt = time.Now().UTC()
	s.devices[v.ID] = v
	return v, nil
}

// DeleteDeviceType removes a device type without affecting its referenced character.
func (s *Store) DeleteDeviceType(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[id]; !ok {
		return apperr.NewNotFound("device type")
	}
	delete(s.devices, id)
	return nil
}
