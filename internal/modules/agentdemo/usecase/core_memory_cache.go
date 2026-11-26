package usecase

import (
	"context"
	"sync"
	"time"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
)

const (
	coreCacheCapacity = 256
	coreCacheTTL      = time.Minute
)

// CoreMemoryStore combines summary reads with durable memory operations.
type CoreMemoryStore interface {
	MemorySchedulerStore
	memory.CoreReader
}

type coreCacheEntry struct {
	value     memory.CoreMemory
	expiresAt time.Time
}

// CachedMemoryStore shares a bounded, on-demand cache between recall and the
// scheduler. Pending batches still read their authoritative database state.
type CachedMemoryStore struct {
	CoreMemoryStore

	mu         sync.Mutex
	entries    map[memory.Scope]coreCacheEntry
	generation uint64
}

// NewCachedMemoryStore requires one shared instance per application.
func NewCachedMemoryStore(store CoreMemoryStore) *CachedMemoryStore {
	return &CachedMemoryStore{CoreMemoryStore: store, entries: make(map[memory.Scope]coreCacheEntry)}
}

// ReadCore caches even empty summaries but never caches a failed read.
func (s *CachedMemoryStore) ReadCore(ctx context.Context, scope memory.Scope) (memory.CoreMemory, error) {
	now := time.Now()
	s.mu.Lock()
	entry, ok := s.entries[scope]
	if ok && now.Before(entry.expiresAt) {
		s.mu.Unlock()
		return entry.value, nil
	}
	generation := s.generation
	s.mu.Unlock()

	value, err := s.CoreMemoryStore.ReadCore(ctx, scope)
	if err != nil {
		return memory.CoreMemory{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// A read that overlaps publication or deletion must not refill the cache
	// with its pre-invalidation snapshot.
	if generation == s.generation {
		s.putLocked(scope, value, now)
	}
	return value, nil
}

func (s *CachedMemoryStore) putLocked(scope memory.Scope, value memory.CoreMemory, now time.Time) {
	if _, ok := s.entries[scope]; !ok && len(s.entries) >= coreCacheCapacity {
		// No periodic maintenance or additional data structure is necessary.
		// Prefer an expired entry; otherwise evict one entry.
		var victim memory.Scope
		for key, entry := range s.entries {
			victim = key
			if !now.Before(entry.expiresAt) {
				break
			}
		}
		delete(s.entries, victim)
	}
	s.entries[scope] = coreCacheEntry{value: value, expiresAt: now.Add(coreCacheTTL)}
}

// Invalidate removes a cached summary after a successful external mutation.
func (s *CachedMemoryStore) Invalidate(scope memory.Scope) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generation++
	delete(s.entries, scope)
}

// PublishCore invalidates recall only when a changed summary was published.
func (s *CachedMemoryStore) PublishCore(ctx context.Context, scope memory.Scope, version uint64, next memory.CoreMemory) error {
	if err := s.CoreMemoryStore.PublishCore(ctx, scope, version, next); err != nil {
		return err
	}
	s.Invalidate(scope)
	return nil
}
