package usecase

import (
	"context"
	"time"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
)

// MemorySnapshot is the current operator's recallable preferences, not the
// underlying conversation archive.
type MemorySnapshot struct {
	CharacterID   int64         `json:"character_id"`
	CharacterName string        `json:"character_name"`
	Facts         []memory.Fact `json:"facts"`
	Extracting    bool          `json:"extracting"`
}

func (s *MemoryService) reserveExtraction(scope memory.Scope) (previous <-chan struct{}, done chan struct{}, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending[scope] < 0 {
		return nil, nil, false
	}
	previous = s.tails[scope]
	done = make(chan struct{})
	s.tails[scope] = done
	s.pending[scope]++
	return previous, done, true
}

func (s *MemoryService) releaseExtraction(scope memory.Scope, done chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[scope]--
	if s.pending[scope] == 0 {
		delete(s.pending, scope)
		delete(s.tails, scope)
	}
	close(done)
}

func (s *MemoryService) extracting(scope memory.Scope) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending[scope] > 0
}

func (u *ChatUsecase) memorySnapshotScope(ctx context.Context, deviceTypeID int64) (memory.Scope, string, error) {
	if deviceTypeID <= 0 {
		return memory.Scope{}, "", apperr.NewBadRequest("invalid device type")
	}
	owner, err := requestctx.RequireUserID(ctx)
	if err != nil {
		return memory.Scope{}, "", err
	}
	if u == nil || u.configs == nil || u.memory == nil || u.memory.store == nil {
		return memory.Scope{}, "", apperr.NewBadRequest("memory is not configured")
	}
	config, err := u.configs.Get(ctx, deviceTypeID)
	if err != nil {
		return memory.Scope{}, "", err
	}
	return memory.Scope{OwnerID: owner, CharacterID: config.Character.ID}, config.Character.Name, nil
}

// InspectMemory derives both identity keys on the server.
func (u *ChatUsecase) InspectMemory(ctx context.Context, deviceTypeID int64) (MemorySnapshot, error) {
	scope, name, err := u.memorySnapshotScope(ctx, deviceTypeID)
	if err != nil {
		return MemorySnapshot{}, err
	}
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	facts, err := u.memory.store.ListFacts(readCtx, scope, maxRecalledFacts)
	if err != nil {
		return MemorySnapshot{}, err
	}
	if facts == nil {
		facts = []memory.Fact{}
	}
	return MemorySnapshot{CharacterID: scope.CharacterID, CharacterName: name, Facts: facts, Extracting: u.memory.extracting(scope)}, nil
}

// DeleteMemoryFact deletes a preference and refuses to race queued extraction.
// New opted-in turns can learn the same preference again.
func (u *ChatUsecase) DeleteMemoryFact(ctx context.Context, deviceTypeID int64, key string) error {
	scope, _, err := u.memorySnapshotScope(ctx, deviceTypeID)
	if err != nil {
		return err
	}
	if !allowedMemoryKeys[key] {
		return apperr.NewBadRequest("invalid preference key")
	}
	return u.memory.deleteFact(ctx, scope, key)
}

func (s *MemoryService) deleteFact(ctx context.Context, scope memory.Scope, key string) error {
	s.mu.Lock()
	if s.pending[scope] != 0 {
		s.mu.Unlock()
		return apperr.NewConflict("memory extraction or deletion is in progress")
	}
	// Negative reservation prevents a new extraction from racing this write.
	s.pending[scope] = -1
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, scope)
		s.mu.Unlock()
	}()
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.store.DeleteFact(writeCtx, scope, key)
}
