package usecase

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
)

const (
	memoryReadTimeout = 250 * time.Millisecond
	memoryJobTimeout  = 15 * time.Second
)

var (
	memoryDigits = regexp.MustCompile(`\d{4,}`)
	memoryEmail  = regexp.MustCompile(`(?i)\S+@\S+`)
)

// MemoryService queues completed turns for batched consolidation.
type MemoryService struct {
	core    *CachedMemoryStore
	slots   chan struct{}
	mu      sync.Mutex
	pending map[memory.Scope]int
	tails   map[memory.Scope]chan struct{}
}

// NewMemoryService shares the core cache with the consolidation scheduler.
func NewMemoryService(core *CachedMemoryStore) *MemoryService {
	return &MemoryService{
		core: core, slots: make(chan struct{}, 4),
		pending: make(map[memory.Scope]int),
		tails:   make(map[memory.Scope]chan struct{}),
	}
}

// Remember queues one completed turn. Queue saturation never delays replies;
// a false return lets callers record a dropped best-effort Demo job.
func (s *MemoryService) Remember(turn memory.Exchange) bool {
	if s == nil || s.core == nil {
		return false
	}
	select {
	case s.slots <- struct{}{}:
		previous, done, ok := s.reserveWrite(turn.Scope)
		if !ok {
			<-s.slots
			return false
		}
		go func() {
			defer func() {
				s.releaseWrite(turn.Scope, done)
				<-s.slots
			}()
			// Serialize writes within a scope, not across unrelated roles.
			// Waiting jobs count against the existing global capacity.
			if previous != nil {
				<-previous
			}
			if err := s.persistPending(turn); err != nil {
				log.Warn().Err(err).Msg("demo memory background job failed")
			}
		}()
		return true
	default:
		return false
	}
}

func (s *MemoryService) persistPending(turn memory.Exchange) error {
	ctx, cancel := context.WithTimeout(context.Background(), memoryJobTimeout)
	defer cancel()
	if err := s.core.AppendPending(ctx, turn); err != nil {
		return fmt.Errorf("persist pending memory: %w", err)
	}
	return nil
}

func unwrapMemoryJSON(raw string) string {
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```json\n") && strings.HasSuffix(text, "```") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "```json\n"), "```"))
	}
	return text
}
