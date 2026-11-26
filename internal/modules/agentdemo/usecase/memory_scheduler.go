package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/memory"
)

const (
	memoryPollInterval = 15 * time.Second
	memoryScanTimeout  = 2 * time.Second
	memoryScopePage    = 16
)

// MemoryModelResolver obtains the currently configured model for a ready batch.
type MemoryModelResolver func(context.Context) (model.BaseChatModel, error)

// MemorySchedulerStore combines durable queue discovery with consolidation.
type MemorySchedulerStore interface {
	memory.ConsolidationStore
	memory.PendingScopeReader
}

// MemoryScheduler scans durable pending work independently of active sessions.
// Run is intended to be owned by the application lifecycle, once per process.
type MemoryScheduler struct {
	reader       memory.PendingScopeReader
	consolidator *MemoryConsolidator
	resolve      MemoryModelResolver
	after        memory.Scope
}

// NewMemoryScheduler uses the database as the durable queue.
func NewMemoryScheduler(store MemorySchedulerStore, resolve MemoryModelResolver) *MemoryScheduler {
	return &MemoryScheduler{
		reader: store, consolidator: NewMemoryConsolidator(store), resolve: resolve,
	}
}

// Run polls immediately and then at a bounded interval until ctx is canceled.
// Each claimed batch is attempted once; later scans discover new work only.
func (s *MemoryScheduler) Run(ctx context.Context) error {
	if s == nil || s.reader == nil || s.resolve == nil || ctx == nil {
		return errors.New("memory scheduler is not configured")
	}
	for ctx.Err() == nil {
		if err := s.scan(ctx, time.Now()); err != nil && ctx.Err() == nil {
			log.Warn().Err(err).Msg("memory pending scope scan failed")
		}
		timer := time.NewTimer(memoryPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return ctx.Err()
}

func (s *MemoryScheduler) scan(ctx context.Context, now time.Time) error {
	readCtx, cancel := context.WithTimeout(ctx, memoryScanTimeout)
	scopes, err := s.reader.ListReadyScopes(readCtx, s.after, now, memoryScopePage)
	cancel()
	if err != nil {
		return err
	}
	for _, item := range scopes {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.processScope(ctx, item)
		s.after = item.Scope
	}
	if len(scopes) < memoryScopePage {
		s.after = memory.Scope{}
	}
	return nil
}

func (s *MemoryScheduler) processScope(ctx context.Context, item memory.PendingScope) {
	jobCtx, cancel := context.WithTimeout(ctx, memoryJobTimeout)
	defer cancel()
	// Discovery already admitted the scope using the entire pending backlog.
	// Its first bounded page may contain fewer characters than the threshold.
	if _, err := s.consolidator.consolidate(jobCtx, item.Scope, s.resolve, true); err != nil && ctx.Err() == nil {
		log.Warn().Err(err).Msg("memory consolidation ended without updating memory")
	}
}
