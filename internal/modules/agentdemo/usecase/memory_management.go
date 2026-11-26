package usecase

import (
	"context"
	"time"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
)

// MemorySnapshot is the current operator's recallable memory, not the archive.
type MemorySnapshot struct {
	CharacterID   int64  `json:"character_id"`
	CharacterName string `json:"character_name"`
	Extracting    bool   `json:"extracting"`
	Stable        string `json:"stable,omitempty"`
	Dynamic       string `json:"dynamic,omitempty"`
}

func (s *MemoryService) reserveWrite(scope memory.Scope) (previous <-chan struct{}, done chan struct{}, ok bool) {
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

func (s *MemoryService) releaseWrite(scope memory.Scope, done chan struct{}) {
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
	if u == nil || u.configs == nil || u.memory == nil || u.memory.core == nil {
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
	core, err := u.memory.core.ReadCore(readCtx, scope)
	if err != nil {
		return MemorySnapshot{}, err
	}
	return MemorySnapshot{
		CharacterID: scope.CharacterID, CharacterName: name,
		Stable: core.Stable, Dynamic: core.Dynamic, Extracting: u.memory.extracting(scope),
	}, nil
}

// DeleteMemoryFact clears scoped memory and refuses to race queued writes.
// New opted-in turns can learn the same preference again.
func (u *ChatUsecase) DeleteMemoryFact(ctx context.Context, deviceTypeID int64, key string) error {
	scope, _, err := u.memorySnapshotScope(ctx, deviceTypeID)
	if err != nil {
		return err
	}
	if key != "all" {
		return apperr.NewBadRequest("core memory requires a full clear")
	}
	return u.memory.mutateMemory(ctx, scope, func(writeCtx context.Context) error {
		return u.memory.core.ClearMemory(writeCtx, scope)
	})
}

// The local admission reservation prevents queued writes from restoring
// memory after a clear. The store version rejects in-flight model results.
func (s *MemoryService) mutateMemory(ctx context.Context, scope memory.Scope, mutate func(context.Context) error) error {
	s.mu.Lock()
	if s.pending[scope] != 0 {
		s.mu.Unlock()
		return apperr.NewConflict("memory write or clear is in progress")
	}
	// Negative reservation prevents a new write from racing this write.
	s.pending[scope] = -1
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, scope)
		s.mu.Unlock()
	}()
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return mutate(writeCtx)
}
